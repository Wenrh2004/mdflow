package parser

import (
	"strings"

	"github.com/Wenrh2004/mdflow/token"
)

// ---- events and stack ----

// Emit appends a block event to the current batch, stamping it with the next
// document ordinal. It is how a [Finalise] turns a closed accumulating leaf into
// output: a table finaliser emits its container-open, row, cell and
// container-close events in order, and the core neither knows nor cares what
// tree they describe.
func (s *BlockState) Emit(ev token.BlockEvent) { s.emit(ev) }

func (s *BlockState) emit(ev token.BlockEvent) {
	if s.holdingLists {
		s.held = append(s.held, ev)
		return
	}
	ev.Seq = s.seq
	s.seq++
	s.events = append(s.events, ev)
}

func (s *BlockState) top() *container {
	if len(s.stack) == 0 {
		return nil
	}
	return &s.stack[len(s.stack)-1]
}

func (s *BlockState) pushContainer(c container) {
	s.noteDirectChild()
	if c.node == token.List {
		if !s.holdingLists {
			s.holdingLists = true
			s.held = s.held[:0]
		}
		c.openEvent = len(s.held)
		s.listDepth++
	}
	if c.node == token.ListItem {
		c.parentList = len(s.stack) - 1
		if c.parentList >= 0 {
			parent := &s.stack[c.parentList]
			if parent.node == token.List && parent.separated {
				parent.loose = true
				parent.separated = false
			}
		}
	}
	s.emit(token.BlockEvent{Type: token.OpenBlock, Container: c.node, Ordered: c.ordered, Start: c.start})
	s.stack = append(s.stack, c)
}

func (s *BlockState) closeContainersFrom(idx int) {
	for len(s.stack) > idx {
		c := s.stack[len(s.stack)-1]
		if c.node == token.ListItem && (c.pending || c.descendant) && c.parentList >= 0 && c.parentList < len(s.stack)-1 {
			s.stack[c.parentList].separated = true
		}
		s.stack = s.stack[:len(s.stack)-1]
		ev := token.BlockEvent{Type: token.CloseBlock, Container: c.node, Ordered: c.ordered, Start: c.start}
		if c.node == token.List {
			if c.separated && len(s.stack) > 0 && s.stack[len(s.stack)-1].node == token.ListItem {
				s.stack[len(s.stack)-1].descendant = true
			}
			ev.Tight = !c.loose
			if c.openEvent >= 0 && c.openEvent < len(s.held) {
				s.held[c.openEvent].Tight = ev.Tight
			}
			s.emit(ev)
			s.listDepth--
			if s.listDepth == 0 {
				s.releaseHeldLists()
			}
			continue
		}
		s.emit(ev)
	}
}

func (s *BlockState) releaseHeldLists() {
	s.finaliseListLayout()
	s.holdingLists = false
	for i := range s.held {
		ev := s.held[i]
		ev.Seq = s.seq
		s.seq++
		s.events = append(s.events, ev)
	}
	clear(s.held)
	s.held = s.held[:0]
}

type listLayoutFrame struct {
	node          token.Node
	tight         bool
	openEvent     int
	hasChild      bool
	lastTightLeaf int
}

// finaliseListLayout is one linear pass over the outer list suffix. Every
// nested list has already written its final Tight bit onto its open event, so
// the pass can annotate direct paragraphs and the two exact-HTML line-break
// hints without a renderer-side stack or repeated nested scans.
func (s *BlockState) finaliseListLayout() {
	stack := make([]listLayoutFrame, 0, 8)
	startItemChild := func(eventIndex int, tightParagraph bool) {
		if len(stack) == 0 || stack[len(stack)-1].node != token.ListItem {
			return
		}
		item := &stack[len(stack)-1]
		if !item.hasChild {
			s.held[item.openEvent].Newline = !tightParagraph
			item.hasChild = true
		}
		if item.lastTightLeaf >= 0 {
			s.held[item.lastTightLeaf].Leaf.BreakAfter = true
		}
		item.lastTightLeaf = -1
		if tightParagraph {
			item.lastTightLeaf = eventIndex
		}
	}

	for i := range s.held {
		ev := &s.held[i]
		switch ev.Type {
		case token.LeafBlock:
			tightParagraph := false
			if len(stack) > 0 && stack[len(stack)-1].node == token.ListItem {
				item := &stack[len(stack)-1]
				tightParagraph = ev.Leaf.Node == token.Paragraph && item.tight
				if tightParagraph {
					ev.Leaf.Tight = true
				}
			}
			startItemChild(i, tightParagraph)
		case token.OpenBlock:
			startItemChild(i, false)
			frame := listLayoutFrame{node: ev.Container, openEvent: i, lastTightLeaf: -1}
			switch ev.Container {
			case token.List:
				frame.tight = ev.Tight
			case token.ListItem:
				if len(stack) > 0 && stack[len(stack)-1].node == token.List {
					frame.tight = stack[len(stack)-1].tight
				}
			}
			stack = append(stack, frame)
		case token.CloseBlock:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func (s *BlockState) closeLeaf() {
	if s.leaf == nil {
		return
	}
	// Capture the leaf, then clear the slot BEFORE handing off. A finaliser
	// emits events, and a re-entrant closeLeaf from inside one must be a no-op:
	// clearing first makes both safe, so a finaliser that closes the leaf again
	// neither double-emits nor recurses.
	lf := s.leaf
	s.leaf = nil
	if fn := s.rules.finalisers[lf.tag]; fn != nil {
		lines := lf.lines
		fn(s, lines, lf.scratch)
		if s.leaf == nil {
			s.releaseLeafSlot(lines)
		}
		return
	}
	leaf := s.buildLeaf(lf)
	if leaf.Node == token.Paragraph {
		leaf.Content = s.stripReferenceDefinitions(leaf.Content)
		if leaf.Content == "" {
			s.releaseLeafSlot(lf.lines)
			return
		}
	}
	s.releaseLeafSlot(lf.lines)
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: leaf})
}

func (s *BlockState) releaseLeafSlot(lines []string) {
	clear(lines)
	s.leafStore = openLeaf{lines: lines[:0], blankFrom: -1}
}

// stripReferenceDefinitions removes the maximal leading definition prefix of
// a paragraph while registering each definition in document scope. It never
// examines non-paragraph leaves, so code and raw HTML remain literal.
func (s *BlockState) stripReferenceDefinitions(content string) string {
	consumed := scanReferenceDefinitionPrefix(content, &s.references)
	return content[consumed:]
}

// buildLeaf projects an open leaf into its closed form. A finaliser-backed leaf
// never reaches here; this covers paragraphs, headings and the literal blocks
// (code and math fences), which are one leaf apiece.
func (s *BlockState) buildLeaf(lf *openLeaf) token.Leaf {
	leaf := token.Leaf{
		Node: lf.node, Tag: lf.tag, Level: lf.fenceLen, Info: lf.info,
		Context: lf.context,
	}
	switch {
	case lf.literal:
		leaf.Level = 0
		leaf.Literal = true
		lines := lf.lines
		if lf.kind == leafIndentedCode && lf.pendingBlank > 0 {
			lines = lines[:len(lines)-lf.pendingBlank]
		}
		if len(lines) > 0 {
			leaf.Content = joinLines(lines) + "\n"
		}
	case lf.node == token.Paragraph, lf.node == token.Heading:
		leaf.Content = joinLines(lf.lines)
	}
	return leaf
}

// joinLines avoids strings.Join's extra work for the overwhelmingly common
// single-line case.
func joinLines(lines []string) string {
	switch len(lines) {
	case 0:
		return ""
	case 1:
		return lines[0]
	default:
		return strings.Join(lines, "\n")
	}
}

// Clone snapshots the state machine so a caller can speculatively feed it a
// partial line without disturbing the real parse. Used for the provisional view
// of a streaming tail.
func (s *BlockState) clone() *BlockState {
	out := &BlockState{
		rules: s.rules, seq: s.seq,
		holdingLists: s.holdingLists, listDepth: s.listDepth,
	}
	s.references.cloneInto(&out.references)
	out.stack = append([]container(nil), s.stack...)
	out.held = append([]token.BlockEvent(nil), s.held...)
	if s.leaf != nil {
		out.leafStore = *s.leaf // scratch is immutable after StartAccumulator
		out.leafStore.lines = append([]string(nil), s.leaf.lines...)
		out.leaf = &out.leafStore
	}
	return out
}

// isClosingFence recognises the closing fence of the open code block.
func (s *BlockState) isClosingFence(line blockLine) bool {
	ind := line.leadingIndent()
	if ind > 3 {
		return false
	}
	rest := line.consumeIndent(ind)
	r := rest.trimRightSpaceTab()
	if len(r) < s.leaf.fenceLen {
		return false
	}
	for i := 0; i < len(r); i++ {
		if r[i] != s.leaf.fenceChar {
			return false
		}
	}
	return true
}
