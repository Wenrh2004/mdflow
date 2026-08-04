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

// writeEvent is the one place block events become markup: parse the leaf's
// inline content, hand both to the renderer.
func (p *Parser) writeEvent(w renderer.Writer, ev token.BlockEvent) {
	if ev.Type == token.LeafBlock {
		p.cfg.Renderer.RenderLeaf(w, ev.Leaf, p.cfg.Rules.ParseInline(ev.Leaf))
	} else {
		p.cfg.Renderer.RenderContainer(w, ev)
	}
}

func (p *Parser) renderDirect(w renderer.Writer, src string) {
	bp := p.borrow()
	defer p.release(bp)
	render := func(events []token.BlockEvent) {
		for i := range events {
			p.writeEvent(w, events[i])
		}
	}
	parser.EachLine(src, func(line string) bool {
		render(bp.FeedLine(line))
		return true
	})
	render(bp.CloseAll())
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
				Node: e.Node, Tag: e.Tag, Level: e.Level, Info: e.Info, Content: e.Text,
				Literal: e.Literal, Tight: e.Tight, Align: e.Align, Header: e.Header,
			}
			if e.Node == token.ThematicBreak {
				p.cfg.Renderer.RenderLeaf(w, leaf, nil)
				continue
			}
			inLeaf = true
			codeBody.Reset()
		case e.Type == EnterEvent:
			p.cfg.Renderer.RenderContainer(w, token.BlockEvent{
				Type: token.OpenBlock, Container: e.Node, Tag: e.Tag,
				Ordered: e.Ordered, Start: e.Start, Task: e.Task,
			})
		case e.Type == LeaveEvent:
			p.cfg.Renderer.RenderContainer(w, token.BlockEvent{
				Type: token.CloseBlock, Container: e.Node, Tag: e.Tag,
				Ordered: e.Ordered, Start: e.Start, Task: e.Task,
			})
		}
	}
}
