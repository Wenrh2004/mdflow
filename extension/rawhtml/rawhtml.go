// Package rawhtml exposes CommonMark raw HTML as an mdflow extension.
//
// mdflow.New enables this capability with safe escaping as part of its complete
// CommonMark profile. RawHTML remains available for explicit NewWith/WithOnly
// profiles, while WithUnsafeHTML opts trusted input into verbatim output.
package rawhtml

import (
	"github.com/Wenrh2004/mdflow"
	internalrawhtml "github.com/Wenrh2004/mdflow/internal/rawhtml"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// RawHTML recognises all CommonMark inline raw-HTML forms and HTML block types.
// Its HTML output is escaped; use WithUnsafeHTML only for trusted documents.
var RawHTML = internalrawhtml.RawHTML

// Syntax registers raw-HTML syntax independently of a renderer.
func Syntax(p *parser.RuleSet) { internalrawhtml.Syntax(p) }

// IsRawHTML reports whether e is an inline raw-HTML atom or the boundary of a
// raw-HTML block. A block's body is the TextEvent between its two boundaries.
func IsRawHTML(e mdflow.Event) bool {
	return e.Node == token.Custom && e.Tag == internalrawhtml.InlineTag ||
		e.Node == token.CustomLeaf && e.Tag == internalrawhtml.BlockTag
}

// WithUnsafeHTML emits recognised raw HTML verbatim. It only changes output;
// mdflow.New supplies the syntax through its default safe capability.
//
// Enable it only for content you trust. It is intentionally an explicit option
// because raw HTML can execute scripts or inject active attributes.
func WithUnsafeHTML() mdflow.Option {
	return mdflow.WithOutput(internalrawhtml.UnsafeHTML)
}

// WithFilteredHTML emits recognised raw HTML verbatim except for GFM's
// disallowed tags — script, style, iframe, textarea, title, xmp, noembed,
// noframes and plaintext — whose opening '<' is escaped. It is the GFM
// "tagfilter" and suits content that may carry ordinary HTML but should not be
// able to run scripts or swallow the rest of the page. It is not a sanitizer:
// event-handler attributes such as onerror= pass through, so untrusted input
// still belongs on the default escaped output.
func WithFilteredHTML() mdflow.Option {
	return mdflow.WithOutput(internalrawhtml.FilteredHTML)
}
