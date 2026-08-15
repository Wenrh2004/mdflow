// Package text renders parsed structure as plain text.
//
// It exists to prove the parser and the renderer are genuinely independent: the
// same syntax that produces HTML through renderer/html produces prose here, with
// no change anywhere below. A caller swaps New for text.NewRenderer and the
// document arrives as words instead of markup — the seam extension modules plug
// their syntax into is the same one a second output format plugs into.
//
// The renderer is deliberately minimal and depends only on the standard library.
// It implements the [renderer.Renderer] floor and nothing else: it hosts no
// custom-node registrations, so an extension's nodes fall back to their own
// text (an inline `#tag` becomes "tag", a `$x$` becomes "x", a struck run keeps
// its words). That fallback is exactly what reusing only an extension's syntax
// half — not its HTML output — is meant to demonstrate.
//
// Blocks are separated by a blank line; inline emphasis, links and images
// contribute their words without markup, which is what "the text of the
// document" means. It is not a Markdown round-tripper and does not try to be.
package text

import (
	"strings"

	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// Renderer serialises parsed structure as plain text. It carries no
// configuration and no mutable state, so one value is safe to share across every
// parser derived from it; it therefore needs no Clone.
type Renderer struct{}

// NewRenderer builds the plain-text renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// It implements only the Renderer floor: custom nodes fall back to their text
// rather than being hosted, which is the whole point of a syntax-only reuse.
var _ renderer.Renderer = (*Renderer)(nil)

// RenderLeaf writes a leaf block's text. Headings, paragraphs and code blocks
// are each followed by a blank line; a list item's paragraph is tight and takes
// a single newline so items stack.
func (r Renderer) RenderLeaf(w renderer.Writer, leaf token.Leaf, inlines []token.Inline) {
	switch leaf.Node {
	case token.Heading:
		r.RenderInlines(w, inlines)
		w.WriteString("\n\n")
	case token.Paragraph:
		r.RenderInlines(w, inlines)
		if leaf.Tight {
			w.WriteByte('\n')
		} else {
			w.WriteString("\n\n")
		}
	case token.CodeBlock:
		writeLine(w, leaf.Content)
		w.WriteByte('\n')
	case token.ThematicBreak:
		w.WriteByte('\n')
	case token.CustomLeaf:
		// No registration to consult: emit the block's own content, the same
		// fallback the HTML renderer makes for an unregistered leaf. A literal
		// leaf keeps its body in Content; a markdown one carries inlines.
		if leaf.Literal {
			writeLine(w, leaf.Content)
		} else {
			r.RenderInlines(w, inlines)
			w.WriteByte('\n')
		}
	}
}

// RenderContainer writes a container's plain-text framing. Blockquotes and lists
// contribute no glyphs of their own; a list item is marked so items read as a
// list. Extension markers remain inline content and use their own fallback.
func (Renderer) RenderContainer(w renderer.Writer, ev token.BlockEvent) {
	if ev.Type != token.OpenBlock {
		// A list closes with a blank line so the next block stands apart; other
		// containers add nothing on the way out.
		if ev.Container == token.List {
			w.WriteByte('\n')
		}
		return
	}
	if ev.Container == token.ListItem {
		w.WriteString("- ")
	}
}

// RenderInlines walks the flat token stream, keeping the words and dropping the
// markup: emphasis, strong, links and images contribute only their content, and
// a custom inline node falls back to its own text.
func (Renderer) RenderInlines(w renderer.Writer, toks []token.Inline) {
	for _, t := range toks {
		switch t.Node {
		case token.Text, token.CodeSpan:
			w.WriteString(t.Text)
		case token.SoftBreak, token.HardBreak:
			w.WriteByte('\n')
		case token.Custom:
			// Only the opening token carries text; the closing one is empty and
			// so writes nothing.
			w.WriteString(t.Text)
		}
		// Emph, Strong, Link and Image are pure framing here: their content is
		// the tokens between the open/close pair, which the loop emits anyway.
	}
}

// writeLine writes s, guaranteeing it ends with exactly one newline so a code
// block or literal leaf sits on its own line whether or not its content already
// ended in one.
func writeLine(w renderer.Writer, s string) {
	w.WriteString(s)
	if !strings.HasSuffix(s, "\n") {
		w.WriteByte('\n')
	}
}
