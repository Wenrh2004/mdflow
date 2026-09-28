package mdflow

import (
	"io"
	"iter"
	"sync"
)

// This file is the facade's front door: the package-level convenience functions
// that most callers reach for first. Everything they delegate to lives one layer
// down — the Parser type and its construction in parser.go, and the terminal
// operations in render.go, events.go and extract.go.

// defaultParser is built on first use, not at init.
//
// A package-level `= New()` would construct the complete CommonMark rule set
// and its safe raw-HTML renderer registrations in any binary that imports this
// package, including one that only ever calls NewWith with an explicit profile
// and never touches these three functions.
var defaultParser = sync.OnceValue(func() *Parser { return NewBuilder().Build() })

// HTML renders src with the default parser. Repeated calls reuse one parser, so
// the rule tables and object pools are built exactly once per process.
func HTML(src string) string { return defaultParser().HTML(src) }

// Render streams src's HTML into w using the default parser.
func Render(w io.Writer, src string) error { return defaultParser().Render(w, src) }

// Events returns the default parser's unified event stream for src.
func Events(src string) iter.Seq[Event] { return defaultParser().Events(src) }
