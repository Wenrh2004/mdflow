// Package rawhtml implements the CommonMark raw-HTML capability shared by the
// default profile and the public extension/rawhtml facade.
//
// It deliberately lives above parser, renderer and token rather than inside
// any of them: those core layers only see custom tags and remain unaware of
// HTML syntax or output policy.
package rawhtml

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// InlineTag and BlockTag are the public event identities used by the facade.
// The additional tags in block.go are parser-only accumulator states and never
// escape into an event stream.
var (
	InlineTag = token.NewAtomicTag("github.com/Wenrh2004/mdflow/internal/rawhtml.raw_html")
	BlockTag  = token.NewTag("github.com/Wenrh2004/mdflow/internal/rawhtml.raw_html_block")
)

// RawHTML recognises CommonMark raw HTML and escapes it for safe HTML output.
var RawHTML = extension.Capability{
	Name:   "raw_html",
	Syntax: Syntax,
	Output: safeOutput,
}

// UnsafeHTML replaces the safe output handlers with verbatim ones. It has no
// syntax half: it is meaningful only after RawHTML has been enabled.
var UnsafeHTML = extension.Capability{
	Name:   "raw_html_unsafe",
	Output: unsafeOutput,
}

// Syntax installs both raw-HTML block syntax and the six inline forms.
func Syntax(p *parser.RuleSet) {
	p.PrependLeafRule(blockRule{})
	p.AddContinuation(continueBlock)
	registerBlockFinalisers(p)
	p.AddInlineRule(inlineRule{})
}

func safeOutput(r renderer.Renderer) {
	renderer.RegisterCustom(r, InlineTag, func(w renderer.Writer, tok token.Inline) {
		writeEscaped(w, tok.Text)
	})
	renderer.RegisterCustomLeaf(r, BlockTag,
		func(w renderer.Writer, leaf token.Leaf, _ []token.Inline, _ renderer.Renderer) {
			writeEscaped(w, leaf.Content)
		})
}

func unsafeOutput(r renderer.Renderer) {
	renderer.RegisterCustom(r, InlineTag, func(w renderer.Writer, tok token.Inline) {
		w.WriteString(tok.Text)
	})
	renderer.RegisterCustomLeaf(r, BlockTag,
		func(w renderer.Writer, leaf token.Leaf, _ []token.Inline, _ renderer.Renderer) {
			w.WriteString(leaf.Content)
		})
}

// writeEscaped is intentionally the same four-character policy as the core
// HTML renderer. html.EscapeString would also rewrite apostrophes, changing
// otherwise harmless source and diverging from CommonMark's HTML output.
func writeEscaped(w renderer.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		var replacement string
		switch s[i] {
		case '&':
			replacement = "&amp;"
		case '<':
			replacement = "&lt;"
		case '>':
			replacement = "&gt;"
		case '"':
			replacement = "&quot;"
		default:
			continue
		}
		w.WriteString(s[start:i])
		w.WriteString(replacement)
		start = i + 1
	}
	w.WriteString(s[start:])
}
