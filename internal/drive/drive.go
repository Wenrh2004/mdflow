// Package drive is the document driver's view of package parser.
//
// Package parser is the extension seam: its public API is what a rule author
// builds on — RuleSet, BlockState's rule-facing methods, InlineState. Running
// the machines is a different job with a different audience. The facade feeds
// lines, closes documents, seals the reference map, and pauses and resumes
// inline scans; none of that belongs in the API an extension author reads, and
// every method of it frozen into parser at v1 would make each internal change
// breaking.
//
// So the driver operations are unexported in parser and reach the facade
// through the interfaces here, which parser implements and registers at
// initialisation. The indirection is one interface call per line and per leaf.
package drive

import "github.com/Wenrh2004/mdflow/token"

// Block is one document's block state machine, plus the inline phase that
// runs against its reference definitions.
type Block interface {
	// FeedLine feeds one line, without its line ending, and returns the block
	// events it completed. The slice is reused: consume it before the next
	// call, then call ReleaseEvents.
	FeedLine(line string) []token.BlockEvent
	// CloseAll closes every open block at end of input.
	CloseAll() []token.BlockEvent
	// ReleaseEvents declares the last returned batch consumed.
	ReleaseEvents()
	// CollectAll parses src to completion and returns every block event; the
	// slice is an internal buffer, valid until the Block is reused.
	CollectAll(src string) []token.BlockEvent
	// SealReferences declares that no later definition can appear.
	SealReferences()
	// ReferencesRefused reports whether the expansion budget refused any
	// reference in this document.
	ReferencesRefused() bool
	// HeldCount is the number of events held for an open list's tightness.
	HeldCount() int
	// ReferenceFingerprint is an order-independent hash of every definition.
	ReferenceFingerprint() uint64
	// AppendInline parses one closed leaf into dst[:0]. It returns a Cursor
	// only when the scan paused on a reference that may still be defined.
	AppendInline(dst []token.Inline, leaf token.Leaf) ([]token.Inline, Cursor)
	// ParseInlineFinal parses a leaf after SealReferences; safe for concurrent
	// use by the fan-out workers.
	ParseInlineFinal(leaf token.Leaf) []token.Inline
	// Total is the number of block events emitted so far.
	Total() int
	// Clone snapshots the machine for a speculative (provisional) parse.
	Clone() Block
	// Reset rewinds the machine for reuse from a pool.
	Reset()
}

// Cursor is an inline scan paused at an unresolved reference.
type Cursor interface {
	// Resume continues the scan; complete is false while the reference is
	// still undefined and the document still open.
	Resume() (tokens []token.Inline, complete bool)
	// ForceLiteral finishes the scan treating undefined references as text.
	ForceLiteral() (tokens []token.Inline, complete bool)
	// PendingLabel reports the normalised label the scan waits on.
	PendingLabel() (label string, ok bool)
	// CloneFor snapshots the cursor against a cloned Block.
	CloneFor(b Block) Cursor
	// Release returns the cursor's scratch to its pool.
	Release()
}

// NewBlock builds a Block for a *parser.RuleSet. Package parser installs it at
// initialisation; the parameter is untyped only because this package cannot
// import parser without an import cycle.
var NewBlock func(rules any) Block

// EachLine splits src on any CommonMark line ending — LF, CRLF, or CR —
// without allocating. fn returning false stops the walk. The parser and every
// driver must agree on where a line ends, so there is exactly one copy.
func EachLine(src string, fn func(line string) bool) {
	start := 0
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			if !fn(src[start:i]) {
				return
			}
			start = i + 1
		case '\r':
			if !fn(src[start:i]) {
				return
			}
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(src) {
		fn(src[start:])
	}
}
