// Package docdb is the generic core of the documentation database: the
// intermediate .db the doc pipeline compiles sources into (spec 03-srcdoc-spec,
// "Draft 3 - the intermediate database"). It knows sources, topics, segments,
// references, issues and categories - never what a "function" or a "class" is:
// kinds, idents and normalized keys are opaque strings decided by the caller
// (the ot4xb layer today, any other scanner tomorrow).
//
// Principles it enforces:
//   - Scanning is multi-pass and append-only per source. A pass registers its
//     source, deletes everything that source contributed before, and inserts
//     again; it never looks at other sources. Resolution is a separate step.
//   - Every row carries idsrc, so ReplaceSource is a plain delete by idsrc.
//   - Integer ids only where the identity is known at insert: a source (its
//     path) and a topic (its kind+key, created the first time it is seen). A
//     reference target is named by (kind, ident) only: it may not exist yet.
//   - Order is the parse order. A source gets its pos when first registered and
//     keeps it; a segment's pos packs (file pos, position in file) into one
//     integer so ORDER BY pos sorts a topic's segments with no join.
//
// Besides the data, the database carries the part of the configuration that
// describes the documentation itself - books, index and kinds - in the cfg
// table, written when the sources are collected (SetCfg) and read back by
// whoever generates from it (Cfg). That is what lets a repository holding only
// the .db generate without the .doc-tool it was compiled with. The collecting
// side of the configuration - db, src, folders - never travels: it names paths
// of the machine that compiled.
//
// SQLite through modernc.org/sqlite (pure Go, no cgo). One database per project.
package docdb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// SchemaVersion is stored in meta and bumped whenever the schema changes.
const SchemaVersion = "3"

// Segment pos packing in one 64-bit integer: the file position takes the high
// bits (mask 0xFFFFFFFFFFF00000), the position inside the file the low 20
// (mask 0x000FFFFF). If a part ever runs short the split is simply raised here.
const (
	PosShift   = 20                  // bits reserved for the position inside the file
	PosLowMask = (1 << PosShift) - 1 // 0x000FFFFF
)

// PackPos builds a segment pos from the source's pos and the segment's
// position inside its file.
func PackPos(filePos, segPos int64) int64 { return filePos<<PosShift | (segPos & PosLowMask) }

// UnpackPos splits a segment pos back into (file pos, position in file).
func UnpackPos(pos int64) (filePos, segPos int64) { return pos >> PosShift, pos & PosLowMask }

// The table is named refs: "references" is a reserved word in SQL and would
// have to be quoted in every query, including the ad-hoc ones people and
// agents will type against a kept database.
const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cfg (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sources (
  idsrc INTEGER PRIMARY KEY,
  pos   INTEGER NOT NULL,
  src   TEXT NOT NULL COLLATE NOCASE UNIQUE
);
CREATE TABLE IF NOT EXISTS topics (
  idtopic INTEGER PRIMARY KEY,
  kind    TEXT NOT NULL,
  key     TEXT NOT NULL,
  ident   TEXT NOT NULL,
  slug    TEXT NOT NULL DEFAULT '',
  flags   INTEGER NOT NULL DEFAULT 0,
  idtg    INTEGER NOT NULL DEFAULT 0,
  UNIQUE(kind, key)
);
CREATE TABLE IF NOT EXISTS topic_groups (
  idtg  INTEGER PRIMARY KEY,
  idsrc INTEGER NOT NULL,
  pos   INTEGER NOT NULL,
  name  TEXT NOT NULL,
  key   TEXT NOT NULL UNIQUE,
  slug  TEXT NOT NULL DEFAULT '',
  flags INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS fields (
  idsrc      INTEGER NOT NULL,
  idseg      INTEGER NOT NULL,
  seq        INTEGER NOT NULL,
  label      TEXT NOT NULL,
  value      TEXT NOT NULL,
  hide_entry INTEGER NOT NULL DEFAULT 0,
  hide_label INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS fields_seg ON fields(idseg, seq);
CREATE INDEX IF NOT EXISTS fields_src ON fields(idsrc);
CREATE INDEX IF NOT EXISTS fields_label ON fields(label);
CREATE TABLE IF NOT EXISTS segments (
  idseg       INTEGER PRIMARY KEY,
  idtopic     INTEGER NOT NULL,
  idsrc       INTEGER NOT NULL,
  pos         INTEGER NOT NULL,
  line        INTEGER NOT NULL,
  raw         BLOB    NOT NULL,
  resolved    BLOB,
  is_resolved INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS segments_topic ON segments(idtopic, pos);
CREATE INDEX IF NOT EXISTS segments_src   ON segments(idsrc);
CREATE TABLE IF NOT EXISTS refs (
  idseg      INTEGER NOT NULL,
  idsrc      INTEGER NOT NULL,
  idtopic_in INTEGER NOT NULL,
  reftokind  TEXT NOT NULL,
  reftoident TEXT NOT NULL,
  reftype    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS refs_src ON refs(idsrc);
CREATE INDEX IF NOT EXISTS refs_to  ON refs(reftokind, reftoident);
CREATE TABLE IF NOT EXISTS issues (
  idsrc    INTEGER NOT NULL,
  idseg    INTEGER,
  line     INTEGER,
  severity TEXT NOT NULL,
  code     TEXT,
  message  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS issues_src ON issues(idsrc);
CREATE TABLE IF NOT EXISTS topic_category (
  idsrc    INTEGER NOT NULL,
  idtopic  INTEGER NOT NULL,
  category TEXT NOT NULL COLLATE NOCASE
);
CREATE INDEX IF NOT EXISTS topic_category_src ON topic_category(idsrc);
CREATE INDEX IF NOT EXISTS topic_category_cat ON topic_category(category);
CREATE TABLE IF NOT EXISTS topic_book (
  idsrc    INTEGER NOT NULL,
  idtopic  INTEGER NOT NULL,
  book     TEXT NOT NULL COLLATE NOCASE
);
CREATE INDEX IF NOT EXISTS topic_book_src ON topic_book(idsrc);
CREATE INDEX IF NOT EXISTS topic_book_topic ON topic_book(idtopic);
CREATE TABLE IF NOT EXISTS pages (
  file       TEXT NOT NULL PRIMARY KEY,
  title      TEXT NOT NULL,
  kind       TEXT NOT NULL,
  book       TEXT NOT NULL DEFAULT '',
  short      TEXT NOT NULL DEFAULT '',
  keywords   TEXT NOT NULL DEFAULT '',
  categories TEXT NOT NULL DEFAULT '',
  parent     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS pages_parent ON pages(parent);
CREATE TABLE IF NOT EXISTS page_trail (
  file   TEXT NOT NULL,
  pos    INTEGER NOT NULL,
  title  TEXT NOT NULL,
  target TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS page_trail_file ON page_trail(file);
`

// DB is an open documentation database.
type DB struct {
	sql *sql.DB
}

// Open opens (creating if needed) the database at path and makes sure the
// schema exists. ":memory:" gives a throwaway in-memory database.
func Open(path string) (*DB, error) {
	h, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite likes it that way and the pipeline is sequential.
	// RULE for every caller: never Exec while a Query's rows are still open -
	// the Exec would wait for the single connection forever. Collect, close,
	// then write.
	h.SetMaxOpenConns(1)
	// A build artefact, rebuilt from the sources at will: speed over durability.
	if _, err := h.Exec(`PRAGMA journal_mode = MEMORY; PRAGMA synchronous = OFF;`); err != nil {
		h.Close()
		return nil, fmt.Errorf("docdb: pragma: %w", err)
	}
	if _, err := h.Exec(schema); err != nil {
		h.Close()
		return nil, fmt.Errorf("docdb: schema: %w", err)
	}
	if _, err := h.Exec(`INSERT OR IGNORE INTO meta(key, value) VALUES ('schema_version', ?)`, SchemaVersion); err != nil {
		h.Close()
		return nil, err
	}
	var v string
	if err := h.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v); err == nil && v != SchemaVersion {
		h.Close()
		return nil, fmt.Errorf("docdb: %s has schema version %s, this tool writes %s: delete it and compile again", path, v, SchemaVersion)
	}
	return &DB{sql: h}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// Meta sets a meta key (project name, version, extraction data...).
func (d *DB) Meta(key, value string) error {
	_, err := d.sql.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES (?, ?)`, key, value)
	return err
}

// SetCfg stores one configuration block under k. The blocks are the part of
// the .doc-tool that describes the documentation itself - books, index and
// kinds - written when the sources are collected so that whoever generates
// from this database needs no .doc-tool of their own. The collecting side of
// the configuration (db, src, folders) is never stored: it names paths of the
// machine that compiled, meaningless anywhere else.
func (d *DB) SetCfg(k, v string) error {
	_, err := d.sql.Exec(`INSERT OR REPLACE INTO cfg(k, v) VALUES (?, ?)`, k, v)
	return err
}

// Cfg returns the configuration block stored under k, and whether there was
// one: a database compiled before the blocks existed simply has none.
func (d *DB) Cfg(k string) (string, bool, error) {
	var v string
	err := d.sql.QueryRow(`SELECT v FROM cfg WHERE k = ?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// NormalizeSrc turns a source path into its canonical form: separators to
// "/", no leading "./", no trailing "/", no doubled "/". The result must use
// the source alphabet - a-z, 0-9, "-", ".", "_" and "/" (case-insensitively,
// so A-Z pass too) - or an error is returned: anything else is not a path
// this database will accept.
func NormalizeSrc(p string) (string, error) {
	s := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "", errors.New("docdb: empty source path")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '.', c == '_', c == '/':
		default:
			return "", fmt.Errorf("docdb: source path %q: character %q is outside the a-z 0-9 - . _ / alphabet", p, c)
		}
	}
	return s, nil
}

// Source registers a source file (get-or-create by its normalized path,
// case-insensitively). A new source receives the next pos - the parse order
// is the document order - and keeps it on every later call.
func (d *DB) Source(src string) (idsrc, pos int64, created bool, err error) {
	s, err := NormalizeSrc(src)
	if err != nil {
		return 0, 0, false, err
	}
	err = d.sql.QueryRow(`SELECT idsrc, pos FROM sources WHERE src = ?`, s).Scan(&idsrc, &pos)
	if err == nil {
		return idsrc, pos, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, err
	}
	if err = d.sql.QueryRow(`SELECT COALESCE(MAX(pos), 0) + 1 FROM sources`).Scan(&pos); err != nil {
		return 0, 0, false, err
	}
	res, err := d.sql.Exec(`INSERT INTO sources(pos, src) VALUES (?, ?)`, pos, s)
	if err != nil {
		return 0, 0, false, err
	}
	idsrc, err = res.LastInsertId()
	return idsrc, pos, true, err
}

// ReplaceSource deletes everything the source contributed - segments,
// references, issues, category memberships - so a fresh pass can insert it
// again. The source row itself (its idsrc and pos) stays.
func (d *DB) ReplaceSource(idsrc int64) error {
	for _, t := range []string{"segments", "fields", "refs", "issues", "topic_category", "topic_book"} {
		if _, err := d.sql.Exec(`DELETE FROM `+t+` WHERE idsrc = ?`, idsrc); err != nil {
			return fmt.Errorf("docdb: replace source %d: %s: %w", idsrc, t, err)
		}
	}
	// Any change may alter what the other sources resolve to.
	return d.InvalidateResolved()
}

// Topic returns the topic id for (kind, key), creating it the first time it is
// seen; ident is the spelling to display, kept from the first sighting. The
// caller decides kind and the normalized key - the core does not interpret them.
func (d *DB) Topic(kind, key, ident string) (idtopic int64, err error) {
	err = d.sql.QueryRow(`SELECT idtopic FROM topics WHERE kind = ? AND key = ?`, kind, key).Scan(&idtopic)
	if err == nil {
		return idtopic, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := d.sql.Exec(`INSERT INTO topics(kind, key, ident) VALUES (?, ?, ?)`, kind, key, ident)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Topic flag bits.
const (
	FlagSlugExplicit = 1 << 0 // the slug was written (_slug_), not computed
)

// SetTopicSlug applies the slug rule: a computed slug is stored when the topic
// has none; an explicit one replaces a computed one and sets
// FlagSlugExplicit; a second, different explicit slug is refused (the first
// stays) and reported by returning conflict = the slug that stays.
func (d *DB) SetTopicSlug(idtopic int64, slug string, explicit bool) (conflict string, err error) {
	var cur string
	var flags int64
	if err = d.sql.QueryRow(`SELECT slug, flags FROM topics WHERE idtopic = ?`, idtopic).Scan(&cur, &flags); err != nil {
		return "", err
	}
	isExplicit := flags&FlagSlugExplicit != 0
	switch {
	case explicit && isExplicit:
		if !strings.EqualFold(cur, slug) {
			return cur, nil
		}
		return "", nil
	case explicit:
		_, err = d.sql.Exec(`UPDATE topics SET slug = ?, flags = flags | ? WHERE idtopic = ?`, slug, FlagSlugExplicit, idtopic)
		return "", err
	case cur == "":
		_, err = d.sql.Exec(`UPDATE topics SET slug = ? WHERE idtopic = ?`, slug, idtopic)
		return "", err
	}
	return "", nil
}

// Group returns the topic group id for key (get-or-create); name is the
// spelling to display and idsrc/pos say where it was first declared.
func (d *DB) Group(key, name string, idsrc, pos int64) (idtg int64, err error) {
	err = d.sql.QueryRow(`SELECT idtg FROM topic_groups WHERE key = ?`, key).Scan(&idtg)
	if err == nil {
		return idtg, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := d.sql.Exec(`INSERT INTO topic_groups(idsrc, pos, name, key) VALUES (?, ?, ?, ?)`, idsrc, pos, name, key)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetGroupSlug is SetTopicSlug for a group.
func (d *DB) SetGroupSlug(idtg int64, slug string, explicit bool) (conflict string, err error) {
	var cur string
	var flags int64
	if err = d.sql.QueryRow(`SELECT slug, flags FROM topic_groups WHERE idtg = ?`, idtg).Scan(&cur, &flags); err != nil {
		return "", err
	}
	isExplicit := flags&FlagSlugExplicit != 0
	switch {
	case explicit && isExplicit:
		if !strings.EqualFold(cur, slug) {
			return cur, nil
		}
		return "", nil
	case explicit:
		_, err = d.sql.Exec(`UPDATE topic_groups SET slug = ?, flags = flags | ? WHERE idtg = ?`, slug, FlagSlugExplicit, idtg)
		return "", err
	case cur == "":
		_, err = d.sql.Exec(`UPDATE topic_groups SET slug = ? WHERE idtg = ?`, slug, idtg)
		return "", err
	}
	return "", nil
}

// SetTopicGroup puts a topic in a group; a topic already in another group
// keeps it and the other id is returned as conflict.
func (d *DB) SetTopicGroup(idtopic, idtg int64) (conflict int64, err error) {
	var cur int64
	if err = d.sql.QueryRow(`SELECT idtg FROM topics WHERE idtopic = ?`, idtopic).Scan(&cur); err != nil {
		return 0, err
	}
	if cur != 0 && cur != idtg {
		return cur, nil
	}
	if cur == 0 {
		_, err = d.sql.Exec(`UPDATE topics SET idtg = ? WHERE idtopic = ?`, idtg, idtopic)
	}
	return 0, err
}

// AddField stores one "label: value" entry of a segment, in written order
// (seq); label is canonical (visibility underscores stripped) and the two
// hide flags keep the visibility.
func (d *DB) AddField(idsrc, idseg, seq int64, label, value string, hideEntry, hideLabel bool) error {
	_, err := d.sql.Exec(`INSERT INTO fields(idsrc, idseg, seq, label, value, hide_entry, hide_label) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		idsrc, idseg, seq, label, value, b2i(hideEntry), b2i(hideLabel))
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PruneOrphanGroups deletes groups no topic belongs to any more.
func (d *DB) PruneOrphanGroups() (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM topic_groups WHERE idtg NOT IN (SELECT DISTINCT idtg FROM topics WHERE idtg <> 0)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AddSegment stores one contribution of a source to a topic: its packed pos
// (see PackPos), the line it starts on (for citing) and the RAW marker text.
// The resolved blob starts empty: resolution is an a-posteriori operation
// (SetResolved), run once every source is in.
func (d *DB) AddSegment(idtopic, idsrc, pos, line int64, raw []byte) (idseg int64, err error) {
	res, err := d.sql.Exec(`INSERT INTO segments(idtopic, idsrc, pos, line, raw) VALUES (?, ?, ?, ?, ?)`,
		idtopic, idsrc, pos, line, raw)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetResolved stores the a-posteriori resolved content of a segment and marks
// it resolved. What "resolved" holds is the resolver's business; the core only
// keeps the two blobs and the flag.
func (d *DB) SetResolved(idseg int64, resolved []byte) error {
	_, err := d.sql.Exec(`UPDATE segments SET resolved = ?, is_resolved = 1 WHERE idseg = ?`, resolved, idseg)
	return err
}

// InvalidateResolved clears the resolved flag of EVERY segment: references
// cross sources, so a change in any source may change what others resolve to.
// ReplaceSource calls it; resolve recomputes.
func (d *DB) InvalidateResolved() error {
	_, err := d.sql.Exec(`UPDATE segments SET is_resolved = 0 WHERE is_resolved <> 0`)
	return err
}

// AddReference records "who references whom": the segment (and its topic) the
// reference is written in, and the target by (kind, ident) - never by id, the
// target may not exist yet. refType names the semantics (include, parent,
// ilink, see-also, ...); resolution is a later, separate step.
func (d *DB) AddReference(idseg, idsrc, idtopicIn int64, refToKind, refToIdent, refType string) error {
	_, err := d.sql.Exec(`INSERT INTO refs(idseg, idsrc, idtopic_in, reftokind, reftoident, reftype) VALUES (?, ?, ?, ?, ?, ?)`,
		idseg, idsrc, idtopicIn, refToKind, refToIdent, refType)
	return err
}

// AddIssue records a diagnostic in the database - the log is a table. idseg
// and line may be 0 when the issue is not tied to a segment or a line.
func (d *DB) AddIssue(idsrc, idseg, line int64, severity, code, message string) error {
	var seg, ln any
	if idseg != 0 {
		seg = idseg
	}
	if line != 0 {
		ln = line
	}
	_, err := d.sql.Exec(`INSERT INTO issues(idsrc, idseg, line, severity, code, message) VALUES (?, ?, ?, ?, ?, ?)`,
		idsrc, seg, ln, severity, code, message)
	return err
}

// AddCategory records that a topic belongs to a category (N:N). The category
// is a path as text ("winapi/structures"); it exists because it is used.
func (d *DB) AddCategory(idsrc, idtopic int64, category string) error {
	_, err := d.sql.Exec(`INSERT INTO topic_category(idsrc, idtopic, category) VALUES (?, ?, ?)`,
		idsrc, idtopic, strings.ToLower(strings.TrimSpace(category)))
	return err
}

// AddBook records that a topic named a book in its "book:" field (N:N).
func (d *DB) AddBook(idsrc, idtopic int64, book string) error {
	_, err := d.sql.Exec(`INSERT INTO topic_book(idsrc, idtopic, book) VALUES (?, ?, ?)`,
		idsrc, idtopic, strings.ToLower(strings.TrimSpace(book)))
	return err
}

// PruneOrphanTopics deletes topics left without any segment (a rescan can
// remove a topic's last contribution). Returns how many were removed.
func (d *DB) PruneOrphanTopics() (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM topics WHERE idtopic NOT IN (SELECT DISTINCT idtopic FROM segments)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Exec and QueryRow expose the underlying connection for the layers above
// (resolve, generators, ad-hoc queries) without re-exporting database/sql.
func (d *DB) Exec(query string, args ...any) (sql.Result, error) { return d.sql.Exec(query, args...) }

// QueryRow runs a single-row query.
func (d *DB) QueryRow(query string, args ...any) *sql.Row { return d.sql.QueryRow(query, args...) }

// QueryAll runs a multi-row query and returns EVERY row materialized, the
// cursor already closed: one row per slice, one value per column (int64 for
// INTEGER, string for TEXT, []byte for BLOB - a private copy -, nil for NULL).
// This is the only multi-row read the database offers on purpose: the base is
// small, and a live cursor is what can deadlock the single connection. Read
// it all, then act.
func (d *DB) QueryAll(query string, args ...any) ([][]any, error) {
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = append([]byte(nil), b...) // the driver may reuse its buffer
			}
		}
		out = append(out, vals)
	}
	return out, rows.Err()
}

// HasFTS5 reports whether this SQLite build ships the FTS5 module (needed later
// for the doc_fts lookup table).
func (d *DB) HasFTS5() bool {
	_, err := d.sql.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS _fts5_probe USING fts5(x)`)
	if err != nil {
		return false
	}
	_, _ = d.sql.Exec(`DROP TABLE IF EXISTS _fts5_probe`)
	return true
}
