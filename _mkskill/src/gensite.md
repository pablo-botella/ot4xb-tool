---
title: gensite
mkskill:
  pos: 88
---

## `gensite` - the static HTML site

Outside the build, from the same database `gendoc` reads:

```
ot4xb-tool [-q] doc site -site <file.site-def> [-doctool <file>] [-db <file.db>] [-out <dir>] [-title <text>] [-templates <dir>] [-assets <dir>] [-gencss]
ot4xb-tool [-q] doc site [-doctool <file>] -db <file.db> -out <dir> [-title <text>] [-templates <dir>] [-assets <dir>] [-gencss]
ot4xb-tool doc site -export-templates <dir>
```

Every page of the documentation - topics, groups, the indexes and the
category pages - is rendered from its tree (the documentation subset: see
the `.doc-tool` section) into minimal HTML - standard tags, no classes, one
tag per line, links to `x.md` pointing to `x.html` - and poured into a
template. The text is escaped here: identifiers, placeholders and paths
arrive as written and leave visible. goldmark runs on the `{{begin-md}}`
blocks alone, never on a page. A place outside the subset is an error with
its file and line, and nothing is written.
Only the pages are written: no script, no search index, and a style sheet
only when asked for. Everything is relative: the site works opened from disk
as well as served.

### The `.site-def` file

One database can feed any number of sites, so what belongs to a site lives
in its own JSON file, `<name>.site-def`, normally in the folder of the site
(a repository of its own, the one a static host deploys):

```json
{
  "doc_tool": "../ot4xb.doc-tool",
  "db": "../out/ot4xb.db",
  "templates": "./templates",
  "template_page": "doc-page.html",
  "template_index": "doc-index.html",
  "assets": "./assets",
  "out": "./out",
  "title": "ot4xb Reference",
  "gencss": false,
  "clean_urls": false,
  "keywords": ["ot4xb", "xbase"],
  "sitemap": { "base": "https://www.xbwin.com/ot4xb/doc/" }
}
```

The keywords of a page are the names of its topics (an overload without
its parameter list), then its `kw` field, then the site's `keywords`,
without repeats: they go to the `keywords` meta and to `.Keywords`.

`search` (`-search <file>` on the command line) writes a JSON file with
every page but the indexes - `file`, `title`, `kind`, `books`,
`categories`, `keywords`, `short` and `text`, the page body as plain text -
for a search engine that runs in the browser: a static `search.html` of
the site's assets loads it and scores the query; the other pages carry no
script at all. Without the key nothing is written.

Paths are relative to the file. `doc_tool` and `db` locate the
documentation (the flags of the same name override them); `templates` is
the folder whose files replace the built-in templates, and `template_page`
and `template_index` name the files to take from it - without them the names
are `page.html` and `index.html`, which lets a single folder hold the
templates of one site only; a name given here and missing from the folder is
an error, never a silent fall back to the built-in one; `assets` is a folder
copied into `out` as it is, files and subfolders - a style sheet, a favicon,
`robots.txt`, `_redirects` (the `static` folder of other generators);
`out` is where the site goes (required); `title` is the header of every page
(default: the title of the general index); `gencss` writes `style.css` into
`out` - the one of the templates folder when it exists, the built-in sheet
otherwise (default: false); `sitemap` asks for a sitemap (`-sitemap <base>`
on the command line): `base` is the absolute URL the site is served from,
its folder with the trailing slash (required), `file` the output name
(default `sitemap.xml`), `changefreq` and `priority` optional fields of
every `<url>`, `indexes` the priority of the index pages when it differs; `clean_urls`
drops the `.html` from every link, sitemap entry and search index entry
(default: false).
Every flag wins over the file, so the same `.site-def` can be written
somewhere else with `-out`.

`clean_urls` exists because a host that maps `/foo` to `foo.html` answers a
redirect when asked for `/foo.html`, so every internal link pays a round trip
before the page starts to load. The files on disk keep their extension either
way: only what points at them changes, and the general index becomes its own
folder - `./` inside a page, the base URL in the sitemap and the canonical.
It is off by default because a site built with it cannot be read from the
file system, nor served by anything that does not rewrite.

The `.doc-tool` file describes the documentation and knows nothing about
sites: the kinds, books and indexes `gensite` renders come from it (through
`doc_tool` or `-doctool`), the pages from the database.

### Templates

Go `html/template` files for the pages, plain for the style sheet:

| file | used for |
|---|---|
| `page.html` | topic and group pages |
| `index.html` | the composed pages: general index, book indexes, sections of their own, category pages |
| `style.css` | written to the site only with `gencss` |

The built-in ones are embedded in the binary - minimal HTML, standard tags,
no classes, no script, no style sheet: a header linking the general index,
the page, a footer - and `-export-templates <dir>` writes them out (never
overwriting) so they can be edited; any file of the same name in the
templates folder replaces the built-in one. A page template receives
`.Site`, `.Title`, `.Kind`, `.Books`, `.Categories`, `.Keywords`, `.Short`,
`.Source` (`path:line`), `.File`, `.Index` (the general index file),
`.Body` (the HTML), `.Trail` and `.Siblings`; the functions `join` and
`lower` are available.

`.Trail` is the page's logical location, as `resolve` materialized it: the
general index, the book index, the section (a page of its own, or the
heading of the index page - every heading carries an `id`) and, when the
section lists by category, the category page; each step has `.Title` and
`.File`. `.Siblings` are the other pages under the same last step, by
title: the functions of the same category, the classes of the same
section, the category pages of the same section. A site that has not been
resolved cannot be generated: `gensite` says so instead of guessing.
