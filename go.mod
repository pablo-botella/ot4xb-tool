module github.com/pablo-botella/ot4xb-tool

go 1.26

require (
	github.com/pablo-botella/linereader v0.5.2
	github.com/yuin/goldmark v1.7.8
	modernc.org/sqlite v1.58.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.75.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// Everything up to v0.0.7 is the tool before the documentation subtree: the
// packages lived under modules/doc* and modules/sitedef, the CLI had its doc
// commands at the top level, and the database carried no configuration of its
// own. Nothing written against those paths compiles against v0.0.8, so they
// are withdrawn instead of left as a trap.
retract [v0.0.0, v0.0.7]
