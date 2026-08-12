package mdflow

import (
	"iter"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// The unified pull event stream: the surface the functional layer works on, and
// the block/inline flattening that produces it. Events is the public entry;
// rawEvents drives the parser and emitDocumentEvent/emitLeaf do the flattening.

// Events returns the unified pull event stream with the Parser's middleware
// chain applied. Ranging over it drives the parser; breaking out of the range
// stops parsing immediately — the document is never fully materialised.
func (p *Parser) Events(src string) iter.Seq[Event] {
	raw := p.rawEvents(src)
	if p.chain == nil {
		return raw
	}
	return p.chain(raw)
}

func (p *Parser) rawEvents(src string) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		bp := p.borrow()
		defer p.release(bp)
		driver := documentDriver{blocks: bp}
		defer driver.Release()
		emit := func(ev token.BlockEvent, inlines []token.Inline) bool {
			return p.emitDocumentEvent(ev, inlines, yield)
		}
		ok := true
		parser.EachLine(src, func(line string) bool {
			ok = driver.FeedLine(line, emit)
			return ok
		})
		if ok {
			driver.Close(emit)
		}
	}
}

// emitDocumentEvent flattens one completed block event into the unified
// stream, stopping immediately when yield declines an event.
func (p *Parser) emitDocumentEvent(ev token.BlockEvent, inlines []token.Inline, yield func(Event) bool) bool {
	switch ev.Type {
	case token.OpenBlock:
		return yield(ContainerEvent(EnterEvent, ev))
	case token.CloseBlock:
		return yield(ContainerEvent(LeaveEvent, ev))
	default:
		return p.emitLeaf(ev.Leaf, ev.Seq, inlines, yield)
	}
}

// emitLeaf expands one leaf block into unified events: headings, paragraphs and
// table cells expand their inline tokens; code and math blocks yield their
// literal body.
func (p *Parser) emitLeaf(leaf token.Leaf, seq int, inlines []token.Inline, yield func(Event) bool) bool {
	switch {
	case leaf.Node == token.ThematicBreak:
		return yield(Event{Type: EnterEvent, Node: token.ThematicBreak, Seq: seq}) &&
			yield(Event{Type: LeaveEvent, Node: token.ThematicBreak, Seq: seq})
	case leaf.Literal:
		// A verbatim body — a code fence, a math block, an embed target —
		// yields as one text event between the pair, so a middleware can read
		// or rewrite it without the leaf having to be a special case.
		return yield(Event{
			Type: EnterEvent, Node: leaf.Node, Tag: leaf.Tag, Info: leaf.Info,
			Level: leaf.Level, Seq: seq, Tight: leaf.Tight, BreakAfter: leaf.BreakAfter, Literal: true,
			Align: leaf.Align, Header: leaf.Header, Context: leaf.Context,
		}) &&
			yield(Event{Type: TextEvent, Node: token.Text, Text: leaf.Content}) &&
			yield(Event{Type: LeaveEvent, Node: leaf.Node, Tag: leaf.Tag, Seq: seq})
	default:
		if !yield(Event{
			Type: EnterEvent, Node: leaf.Node, Tag: leaf.Tag, Content: leaf.Content,
			Info: leaf.Info, Level: leaf.Level, Seq: seq,
			Tight: leaf.Tight, BreakAfter: leaf.BreakAfter,
			Align: leaf.Align, Header: leaf.Header, Context: leaf.Context,
		}) {
			return false
		}
		for _, t := range inlines {
			if !yield(InlineEvent(t)) {
				return false
			}
		}
		return yield(Event{Type: LeaveEvent, Node: leaf.Node, Tag: leaf.Tag, Seq: seq})
	}
}
