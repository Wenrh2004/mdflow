package mdflow

import (
	"iter"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// The unified pull event stream: the surface the functional layer works on, and
// the block/inline flattening that produces it. Events is the public entry;
// rawEvents drives the parser and emitBlockEvents/emitLeaf do the flattening.

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
		ok := true
		parser.EachLine(src, func(line string) bool {
			ok = p.emitBlockEvents(bp.FeedLine(line), yield)
			return ok
		})
		if ok {
			p.emitBlockEvents(bp.CloseAll(), yield)
		}
	}
}

// emitBlockEvents fans one batch of block events into the unified stream,
// stopping and reporting false the moment yield declines one. It is the body
// shared by the plain and context-aware raw streams.
func (p *Parser) emitBlockEvents(events []token.BlockEvent, yield func(Event) bool) bool {
	for i := range events {
		ev := events[i]
		switch ev.Type {
		case token.OpenBlock:
			if !yield(ContainerEvent(EnterEvent, ev)) {
				return false
			}
		case token.CloseBlock:
			if !yield(ContainerEvent(LeaveEvent, ev)) {
				return false
			}
		default:
			if !p.emitLeaf(ev.Leaf, yield) {
				return false
			}
		}
	}
	return true
}

// emitLeaf expands one leaf block into unified events: headings, paragraphs and
// table cells expand their inline tokens; code and math blocks yield their
// literal body.
func (p *Parser) emitLeaf(leaf token.Leaf, yield func(Event) bool) bool {
	switch {
	case leaf.Node == token.ThematicBreak:
		return yield(Event{Type: EnterEvent, Node: token.ThematicBreak}) &&
			yield(Event{Type: LeaveEvent, Node: token.ThematicBreak})
	case leaf.Literal:
		// A verbatim body — a code fence, a math block, an embed target —
		// yields as one text event between the pair, so a middleware can read
		// or rewrite it without the leaf having to be a special case.
		return yield(Event{
			Type: EnterEvent, Node: leaf.Node, Tag: leaf.Tag, Info: leaf.Info, Literal: true,
		}) &&
			yield(Event{Type: TextEvent, Node: token.Text, Text: leaf.Content}) &&
			yield(Event{Type: LeaveEvent, Node: leaf.Node, Tag: leaf.Tag})
	default:
		if !yield(Event{
			Type: EnterEvent, Node: leaf.Node, Tag: leaf.Tag, Level: leaf.Level,
			Tight: leaf.Tight, Align: leaf.Align, Header: leaf.Header,
		}) {
			return false
		}
		for _, t := range p.cfg.Rules.ParseInline(leaf) {
			if !yield(InlineEvent(t)) {
				return false
			}
		}
		return yield(Event{Type: LeaveEvent, Node: leaf.Node, Tag: leaf.Tag})
	}
}
