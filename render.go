package mdflow

import (
	"bufio"
	"io"
	"iter"
	"strings"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// The terminal operations that turn a document into markup: HTML and Render,
// and the internal render paths they and their context twins share.

// HTML renders src to an HTML string.
func (p *Parser) HTML(src string) string {
	var b strings.Builder
	b.Grow(len(src) + len(src)/2) // one reservation instead of repeated doubling
	p.renderTo(&b, src)
	return b.String()
}

// Render streams src's HTML straight into w, never building a document-sized
// intermediate string.
func (p *Parser) Render(w io.Writer, src string) error {
	if bw, ok := w.(renderer.Writer); ok {
		p.renderTo(bw, src)
		return nil
	}
	bw := bufio.NewWriter(w)
	p.renderTo(bw, src)
	return bw.Flush()
}

// renderTo picks the fast path when no middleware is installed. With an empty
// pipeline the block state machine writes directly into the renderer and the
// unified event stream is never materialised — the functional layer is strictly
// pay-for-what-you-use.
func (p *Parser) renderTo(w renderer.Writer, src string) {
	switch {
	case p.parallelEligible(src):
		p.renderParallel(w, src)
	case p.chain == nil:
		p.renderDirect(w, src)
	default:
		p.renderEvents(w, p.Events(src))
	}
}

// writeDocumentEvent is the one place completed document events become markup.
// The document driver has already resolved the leaf's inline phase.
func (p *Parser) writeDocumentEvent(w renderer.Writer, ev token.BlockEvent, inlines []token.Inline) {
	if ev.Type == token.LeafBlock {
		p.cfg.Renderer.RenderLeaf(w, ev.Leaf, inlines)
	} else {
		p.cfg.Renderer.RenderContainer(w, ev)
	}
}

// writeFinalEvent is the sealed-resolver spelling used by parallel workers.
// No cursor can escape ParseInlineFinal, so workers share BlockState only for
// immutable reference lookup.
func (p *Parser) writeFinalEvent(w renderer.Writer, blocks *parser.BlockState, ev token.BlockEvent) {
	if ev.Type == token.LeafBlock {
		p.cfg.Renderer.RenderLeaf(w, ev.Leaf, blocks.ParseInlineFinal(ev.Leaf))
	} else {
		p.cfg.Renderer.RenderContainer(w, ev)
	}
}

func (p *Parser) renderDirect(w renderer.Writer, src string) {
	bp := p.borrow()
	defer p.release(bp)
	driver := newBatchDriver(bp, src)
	defer driver.Release()
	render := func(ev token.BlockEvent, inlines []token.Inline) bool {
		p.writeDocumentEvent(w, ev, inlines)
		return true
	}
	parser.EachLine(src, func(line string) bool {
		return driver.FeedLine(line, render)
	})
	driver.Close(render)
}

// renderEvents reconstructs leaves from a (possibly transformed) event stream
// and feeds them to the same Renderer, so a middleware chain and the fast path
// produce identical markup for identical events.
func (p *Parser) renderEvents(w renderer.Writer, seq iter.Seq[Event]) {
	var (
		leaf     token.Leaf
		inlines  []token.Inline
		inLeaf   bool
		codeBody strings.Builder
	)
	for e := range seq {
		if inLeaf {
			// Tag as well as Node: every custom leaf shares one node type, so
			// the node alone no longer identifies which leaf is closing.
			if e.Type == LeaveEvent && e.Node == leaf.Node && e.Tag == leaf.Tag && IsLeafNode(e.Node) {
				if leaf.Literal {
					leaf.Content = codeBody.String()
				}
				p.cfg.Renderer.RenderLeaf(w, leaf, inlines)
				inLeaf, inlines = false, inlines[:0]
				continue
			}
			if leaf.Literal && e.Type == TextEvent {
				codeBody.WriteString(e.Text)
				continue
			}
			inlines = append(inlines, e.InlineToken())
			continue
		}
		switch {
		case e.Type == EnterEvent && IsLeafNode(e.Node):
			leaf = token.Leaf{
				Node: e.Node, Tag: e.Tag, Level: e.Level, Info: e.Info, Content: e.Content,
				Literal: e.Literal, Tight: e.Tight, BreakAfter: e.BreakAfter,
				Align: e.Align, Header: e.Header, Context: e.Context,
			}
			inLeaf = true
			codeBody.Reset()
		case e.Type == EnterEvent:
			p.cfg.Renderer.RenderContainer(w, token.BlockEvent{
				Type: token.OpenBlock, Container: e.Node, Tag: e.Tag, Seq: e.Seq,
				Ordered: e.Ordered, Start: e.Start, Tight: e.Tight,
				Newline: e.Newline, Align: e.Align,
			})
		case e.Type == LeaveEvent:
			p.cfg.Renderer.RenderContainer(w, token.BlockEvent{
				Type: token.CloseBlock, Container: e.Node, Tag: e.Tag, Seq: e.Seq,
				Ordered: e.Ordered, Start: e.Start, Tight: e.Tight,
				Newline: e.Newline, Align: e.Align,
			})
		}
	}
}
