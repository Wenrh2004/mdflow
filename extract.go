package mdflow

import (
	"iter"
	"strings"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// The extraction terminals: read a document for its structure or its words
// rather than rendering it. Blocks yields block structure without inline
// parsing; Text and Headings walk the unified stream once and collect.

// Blocks returns block structure only, without expanding inline content. Leaf
// contents are the raw, unparsed text — useful for outlines, indexing and
// chunking, where inline parsing would be wasted work.
func (p *Parser) Blocks(src string) iter.Seq[token.BlockEvent] {
	return func(yield func(token.BlockEvent) bool) {
		bp := p.borrow()
		defer p.release(bp)
		emit := func(events []token.BlockEvent) bool {
			for i := range events {
				if !yield(events[i]) {
					return false
				}
			}
			return true
		}
		ok := true
		parser.EachLine(src, func(line string) bool {
			ok = emit(bp.FeedLine(line))
			return ok
		})
		if ok {
			emit(bp.CloseAll())
		}
	}
}

// Text extracts the document's plain text: headings and paragraphs joined by
// blank lines, with all markup removed. Code blocks are skipped.
func (p *Parser) Text(src string) string { return collectText(p.Events(src)) }

// collectText is the event-stream half of Text, shared with TextContext.
func collectText(seq iter.Seq[Event]) string {
	var b strings.Builder
	inCode := false
	for e := range seq {
		switch {
		case e.Type == EnterEvent && e.Node == token.CodeBlock:
			inCode = true
		case e.Type == LeaveEvent && e.Node == token.CodeBlock:
			inCode = false
		case e.Type == LeaveEvent && IsLeafNode(e.Node):
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
		case (e.Type == TextEvent || e.Type == CodeEvent) && !inCode:
			b.WriteString(e.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// Headings extracts the document outline in one pass, without rendering.
func (p *Parser) Headings(src string) []Heading { return collectHeadings(p.Events(src)) }

// collectHeadings is the event-stream half of Headings, shared with
// HeadingsContext.
func collectHeadings(seq iter.Seq[Event]) []Heading {
	var (
		out  []Heading
		cur  Heading
		open bool
		buf  strings.Builder
	)
	for e := range seq {
		switch {
		case e.Type == EnterEvent && e.Node == token.Heading:
			cur, open = Heading{Level: e.Level}, true
			buf.Reset()
		case e.Type == LeaveEvent && e.Node == token.Heading && open:
			cur.Text = buf.String()
			out = append(out, cur)
			open = false
		case open && (e.Type == TextEvent || e.Type == CodeEvent):
			buf.WriteString(e.Text)
		}
	}
	return out
}
