package parser

import "github.com/Wenrh2004/mdflow/token"

// ---- container rules ----

// The public rule interfaces intentionally keep their string surface. Built-in
// rules additionally implement these private interfaces so the core can retain
// visual-column and partial-tab state between nested rules.
type blockContainerRule interface {
	openBlockLine(*BlockState, blockLine) (blockLine, bool)
}

type blockLeafRule interface {
	openBlockLine(*BlockState, blockLine) bool
}

func openContainerRule(r ContainerRule, s *BlockState, line blockLine) (blockLine, bool) {
	if br, ok := r.(blockContainerRule); ok {
		return br.openBlockLine(s, line)
	}
	before := line.Text()
	rest, ok := r.Open(s, line.public())
	if !ok {
		return line, false
	}
	return line.afterExtensionRemainder(before, rest), true
}

func openLeafRule(r LeafRule, s *BlockState, line blockLine) bool {
	if br, ok := r.(blockLeafRule); ok {
		return br.openBlockLine(s, line)
	}
	return r.Open(s, line.public())
}

// blockquoteRule opens a blockquote.
type blockquoteRule struct{}

func (blockquoteRule) Name() string { return "blockquote" }
func (blockquoteRule) Open(s *BlockState, line Line) (string, bool) {
	r, ok := (blockquoteRule{}).openBlockLine(s, lineFrom(line))
	return r.Text(), ok
}
func (blockquoteRule) openBlockLine(s *BlockState, line blockLine) (blockLine, bool) {
	if r, ok := stripBlockquoteMarkerLine(line); ok {
		s.closeLeaf()
		s.pushContainer(container{node: token.Blockquote})
		return r, true
	}
	return line, false
}
func (blockquoteRule) interruptsParagraphBlockLine(line blockLine) bool {
	_, ok := stripBlockquoteMarkerLine(line)
	return ok
}

// listRule opens a list plus its item, switching list type when the marker changes.
type listRule struct{}

func (listRule) Name() string { return "list" }
func (listRule) Open(s *BlockState, line Line) (string, bool) {
	r, ok := (listRule{}).openBlockLine(s, lineFrom(line))
	return r.Text(), ok
}
func (listRule) openBlockLine(s *BlockState, line blockLine) (blockLine, bool) {
	m, ok := parseListMarkerLine(line)
	if !ok {
		return line, false
	}
	if s.leaf != nil && s.leaf.node == token.Paragraph && !listMarkerInterruptsParagraph(m) {
		return line, false
	}
	s.closeLeaf()
	if top := s.top(); top != nil && top.node == token.List && top.marker != m.marker {
		s.closeContainersFrom(len(s.stack) - 1) // a different marker starts a new list
	}
	if top := s.top(); top == nil || top.node != token.List {
		s.pushContainer(container{node: token.List, marker: m.marker, ordered: m.ordered, start: m.start})
	}
	s.pushContainer(container{node: token.ListItem, indent: m.width})
	return m.rest, true
}
func (listRule) interruptsParagraphBlockLine(line blockLine) bool {
	m, ok := parseListMarkerLine(line)
	return ok && listMarkerInterruptsParagraph(m)
}

func listMarkerInterruptsParagraph(m listMarkerInfo) bool {
	return !m.blank && (!m.ordered || m.start == 1)
}

// ---- leaf rules ----

// blankRule: a blank line closes the open leaf.
type blankRule struct{}

func (blankRule) Name() string { return "blank" }
func (blankRule) Open(s *BlockState, line Line) bool {
	return (blankRule{}).openBlockLine(s, lineFrom(line))
}
func (blankRule) openBlockLine(s *BlockState, line blockLine) bool {
	if line.blank() {
		s.closeLeaf()
		return true
	}
	return false
}
func (blankRule) interruptsParagraphBlockLine(blockLine) bool { return false }

// setextHeadingRule upgrades an open paragraph to a heading when the line is a
// setext underline. It is registered ahead of thematicBreakRule so that `---`
// under a paragraph reads as an <h2> rather than an <hr>, matching CommonMark.
type setextHeadingRule struct{}

func (setextHeadingRule) Name() string { return "setext_heading" }
func (setextHeadingRule) Open(s *BlockState, line Line) bool {
	return (setextHeadingRule{}).openBlockLine(s, lineFrom(line))
}
func (setextHeadingRule) openBlockLine(s *BlockState, line blockLine) bool {
	lines := s.OpenParagraphLines()
	if len(lines) == 0 {
		return false
	}
	level, ok := parseSetextUnderlineLine(line)
	if !ok {
		return false
	}
	content := s.stripReferenceDefinitions(joinLines(lines))
	s.leaf = nil // consume the paragraph without emitting it
	s.releaseLeafSlot(lines)
	if content == "" {
		// The preceding paragraph consisted only of definitions. They are now
		// registered, but there is no paragraph for this line to underline; let
		// the remaining leaf rules classify the line once as fresh input.
		return false
	}
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: token.Leaf{Node: token.Heading, Level: level, Content: content}})
	return true
}
func (setextHeadingRule) interruptsParagraphBlockLine(blockLine) bool { return false }

// thematicBreakRule handles `---`, `***`, `___`.
type thematicBreakRule struct{}

func (thematicBreakRule) Name() string { return "thematic_break" }
func (thematicBreakRule) Open(s *BlockState, line Line) bool {
	return (thematicBreakRule{}).openBlockLine(s, lineFrom(line))
}
func (thematicBreakRule) openBlockLine(s *BlockState, line blockLine) bool {
	if isThematicBreakBlockLine(line) {
		s.EmitLeaf(token.Leaf{Node: token.ThematicBreak})
		return true
	}
	return false
}
func (thematicBreakRule) interruptsParagraphBlockLine(line blockLine) bool {
	return isThematicBreakBlockLine(line)
}

// atxHeadingRule handles `## Heading`.
type atxHeadingRule struct{}

func (atxHeadingRule) Name() string { return "atx_heading" }
func (atxHeadingRule) Open(s *BlockState, line Line) bool {
	return (atxHeadingRule{}).openBlockLine(s, lineFrom(line))
}
func (atxHeadingRule) openBlockLine(s *BlockState, line blockLine) bool {
	if level, content, ok := parseATXHeadingLine(line); ok {
		s.EmitLeaf(token.Leaf{Node: token.Heading, Level: level, Content: content})
		return true
	}
	return false
}
func (atxHeadingRule) interruptsParagraphBlockLine(line blockLine) bool {
	_, _, ok := parseATXHeadingLine(line)
	return ok
}

// fenceRule opens a fenced code block.
type fenceRule struct{}

func (fenceRule) Name() string { return "fenced_code" }
func (fenceRule) Open(s *BlockState, line Line) bool {
	return (fenceRule{}).openBlockLine(s, lineFrom(line))
}
func (fenceRule) openBlockLine(s *BlockState, line blockLine) bool {
	if ch, n, indent, info, ok := parseFenceOpenLine(line); ok {
		s.startCodeBlock(unescapeSource(info), ch, n, indent)
		return true
	}
	return false
}
func (fenceRule) interruptsParagraphBlockLine(line blockLine) bool {
	_, _, _, _, ok := parseFenceOpenLine(line)
	return ok
}

// indentedCodeRule opens a literal code block after four columns of indentation.
// It deliberately declines an open paragraph: indented code cannot interrupt
// paragraph continuation in CommonMark.
type indentedCodeRule struct{}

func (indentedCodeRule) Name() string { return "indented_code" }
func (indentedCodeRule) Open(s *BlockState, line Line) bool {
	return (indentedCodeRule{}).openBlockLine(s, lineFrom(line))
}
func (indentedCodeRule) openBlockLine(s *BlockState, line blockLine) bool {
	if (s.leaf != nil && s.leaf.node == token.Paragraph) || line.blank() || line.leadingIndent() < 4 {
		return false
	}
	s.startIndentedCode(line)
	return true
}
func (indentedCodeRule) interruptsParagraphBlockLine(blockLine) bool { return false }

// paragraphRule is the catch-all: it always matches.
type paragraphRule struct{}

func (paragraphRule) Name() string { return "paragraph" }
func (paragraphRule) Open(s *BlockState, line Line) bool {
	s.appendParagraphLine(line.Text)
	return true
}
func (paragraphRule) openBlockLine(s *BlockState, line blockLine) bool {
	s.appendParagraphLine(line.Text())
	return true
}
