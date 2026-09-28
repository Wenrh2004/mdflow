package parser

import (
	"strings"

	"github.com/Wenrh2004/mdflow/token"
)

// ---- operations exposed to rules ----

// EmitLeaf emits a terminal leaf block, closing any open leaf first.
func (s *BlockState) EmitLeaf(leaf token.Leaf) {
	s.closeLeaf()
	s.noteDirectChild()
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: leaf})
}

// InListItem reports whether the current block's immediate parent is a list
// item. Final tight/loose layout is intentionally unavailable while the item is
// open: a later blank line can still change it until the surrounding list
// closes.
func (s *BlockState) InListItem() bool {
	t := s.top()
	return t != nil && t.node == token.ListItem
}

func (s *BlockState) noteBlankLine(blankFrom int) {
	if blankFrom < 0 {
		return
	}
	// A blank belongs first to the deepest active list item. If it later
	// escapes a nested list, closeContainersFrom promotes it to the containing
	// item as descendant; marking every ancestor now would incorrectly loosen
	// the outer list in example 319.
	for i := len(s.stack) - 1; i >= blankFrom; i-- {
		if s.stack[i].node == token.ListItem {
			s.stack[i].pending = true
			return
		}
	}
}

// noteDirectChild records the start of one block whose immediate parent is a
// list item. A pending blank becomes semantically loose only now: a trailing
// blank at EOF has no following block and therefore changes nothing.
func (s *BlockState) noteDirectChild() bool {
	if len(s.stack) == 0 || s.stack[len(s.stack)-1].node != token.ListItem {
		return false
	}
	itemIndex := len(s.stack) - 1
	item := &s.stack[itemIndex]
	first := !item.hasChild
	if (item.pending || item.descendant) && item.hasChild && item.parentList >= 0 && item.parentList < itemIndex {
		s.stack[item.parentList].loose = true
	}
	item.hasChild = true
	item.pending = false
	item.descendant = false
	return first
}

// OpenParagraphLines returns the lines of the currently open paragraph, or nil.
// Rules that need one line of look-back (setext headings, GFM tables) use it.
func (s *BlockState) OpenParagraphLines() []string {
	if s.leaf == nil || s.leaf.node != token.Paragraph {
		return nil
	}
	return s.leaf.lines
}

// OpenParagraphContent returns the open paragraph's lines after any leading
// link reference definitions — the lines that would become its content — or
// nil. A rule that promotes the paragraph (a GFM table taking it as its header
// row) reads this to decide, because [PromoteParagraph] strips and registers
// those definitions first. It registers nothing itself.
func (s *BlockState) OpenParagraphContent() []string {
	lines := s.OpenParagraphLines()
	return lines[leadingDefinitionLines(lines, nil):]
}

// leadingDefinitionLines reports how many of lines the maximal leading run of
// reference definitions occupies, registering each definition with resolver
// when it is non-nil. A definition always ends at a line ending, so the run
// covers whole lines.
func leadingDefinitionLines(lines []string, resolver *referenceResolver) int {
	if len(lines) == 0 || !mayStartReferenceDefinition(lines[0]) {
		return 0
	}
	joined := joinLines(lines)
	consumed := scanReferenceDefinitionPrefix(joined, resolver)
	if consumed == 0 {
		return 0
	}
	if consumed >= len(joined) {
		return len(lines)
	}
	return strings.Count(joined[:consumed], "\n")
}

// mayStartReferenceDefinition is the cheap first-byte filter: a definition
// opens with '[' after at most three spaces.
func mayStartReferenceDefinition(line string) bool {
	for i := 0; i < len(line) && i < 4; i++ {
		switch line[i] {
		case ' ':
			continue
		case '[':
			return true
		}
		return false
	}
	return false
}

// beginLeaf reuses the single openLeaf slot, so no leaf block ever allocates its
// own struct or lines slice.
func (s *BlockState) beginLeaf(node token.Node) *openLeaf {
	return s.beginLeafTag(node, token.NoTag)
}

// A rule outside this package accumulates a multi-line leaf through the public
// seam: [StartAccumulator] or [StartLiteral] opens one on the shared slot,
// [BlockState.AppendLine] folds in each continuation line, and either a
// registered finaliser ([RuleSet.AddFinalise] via [AddFinalise]) or the literal
// path turns the buffer into events when it closes. A leaf whose whole content
// is on its opening line needs none of this and uses [BlockState.EmitLeaf].
func (s *BlockState) beginLeafTag(node token.Node, tag token.Tag) *openLeaf {
	lines := s.leafStore.lines
	clear(lines)
	s.leafStore = openLeaf{node: node, tag: tag, lines: lines[:0], blankFrom: -1}
	s.leaf = &s.leafStore
	return s.leaf
}

// appendParagraphLine folds one line into the open paragraph, starting one if
// needed.
//
// Leading block indentation is not inline content, so it is removed here.
// Trailing whitespace is preserved until the inline scan: only that layer knows
// whether it belongs to code/raw HTML or spells a hard/soft line break.
func (s *BlockState) appendParagraphLine(line string) {
	if s.leaf == nil {
		first := s.noteDirectChild()
		leaf := s.beginLeaf(token.Paragraph)
		if first {
			leaf.context |= token.InlineContextListItemHead
		}
	}
	s.leaf.lines = append(s.leaf.lines, strings.TrimLeft(line, " \t"))
}

// startCodeBlock opens a fenced code leaf.
func (s *BlockState) startCodeBlock(info string, ch byte, length, indent int) {
	s.closeLeaf()
	s.noteDirectChild()
	lf := s.beginLeaf(token.CodeBlock)
	lf.info, lf.fenceChar, lf.fenceLen, lf.fenceIndent = info, ch, length, indent
	lf.kind = leafFencedCode
	lf.literal = true
}

func (s *BlockState) startIndentedCode(line blockLine) {
	s.closeLeaf()
	s.noteDirectChild()
	lf := s.beginLeaf(token.CodeBlock)
	lf.kind = leafIndentedCode
	lf.literal = true
	s.appendIndentedLine(line)
}

func (s *BlockState) appendIndentedLine(line blockLine) {
	rest := line.consumeIndent(4)
	s.leaf.lines = append(s.leaf.lines, rest.Text())
	s.leaf.pendingBlank = 0
	s.leaf.blankFrom = -1
}

func (s *BlockState) appendIndentedBlank(line blockLine, blankFrom int) {
	rest := line.consumeIndent(4)
	s.leaf.lines = append(s.leaf.lines, rest.Text())
	s.leaf.pendingBlank++
	if s.leaf.blankFrom < 0 || blankFrom >= 0 && blankFrom < s.leaf.blankFrom {
		s.leaf.blankFrom = blankFrom
	}
}

func (s *BlockState) commitIndentedBlank() {
	if s.leaf != nil && s.leaf.pendingBlank > 0 {
		s.noteBlankLine(s.leaf.blankFrom)
	}
}

// ---- accumulating leaves (the multi-line seam) ----

// AppendLine folds one more raw line into the open accumulating leaf. A
// continuation handler calls it for each line the leaf claims; the finaliser
// registered for the leaf's tag receives the collected lines when it closes.
//
// It is a no-op when no leaf is open, so a continuation need not guard.
func (s *BlockState) AppendLine(line string) {
	if s.leaf != nil {
		s.leaf.lines = append(s.leaf.lines, line)
	}
}

// AccumulatorTag reports the tag of the open accumulating leaf, or [token.NoTag]
// when the open leaf is not one (a paragraph, say) or nothing is open. A
// continuation uses it to recognise its own leaf without reaching into state.
func (s *BlockState) AccumulatorTag() token.Tag {
	if s.leaf == nil {
		return token.NoTag
	}
	return s.leaf.tag
}

// StartAccumulator opens a multi-line custom leaf tagged tag, replacing any open
// leaf, and stows scratch for the finaliser to read back when the leaf closes.
// The type parameter is the scratch's own type, so a rule writes and a finaliser
// reads it without an interface conversion at either end:
//
//	parser.StartAccumulator(s, myTag, cols)      // in the leaf rule
//	cols := scratch                              // in the finaliser, already typed
//
// Scratch is written once here and never mutated afterwards, which is what makes
// [BlockState.Clone] — and hence the provisional view — a safe shallow copy.
func StartAccumulator[S any](s *BlockState, tag token.Tag, scratch S) {
	s.closeLeaf()
	s.noteDirectChild()
	lf := s.beginLeafTag(token.CustomLeaf, tag)
	lf.scratch = scratch
}

// PromoteParagraph is [StartAccumulator] for a leaf that grows out of the open
// paragraph rather than a fresh line — a GFM table, whose header is the
// paragraph line above the delimiter row. It absorbs that paragraph's lines
// instead of emitting them, so the header is the accumulator's first line.
// Link reference definitions leading the paragraph are registered with the
// document and left out, so the absorbed lines are [BlockState.OpenParagraphContent].
func PromoteParagraph[S any](s *BlockState, tag token.Tag, scratch S) {
	var lines []string
	if s.leaf != nil && s.leaf.node == token.Paragraph {
		// Leading link reference definitions belong to the document, not to
		// the promoted block: register them exactly as closing the paragraph
		// would, and absorb only what follows (as cmark-gfm does).
		lines = append(lines, s.leaf.lines[leadingDefinitionLines(s.leaf.lines, &s.references):]...)
	}
	s.leaf = nil // absorb the paragraph without emitting it
	lf := s.beginLeafTag(token.CustomLeaf, tag)
	lf.lines = append(lf.lines, lines...)
	lf.scratch = scratch
}

// StartLiteral opens a multi-line leaf whose body is verbatim — a `$$` math
// block, or any fenced construct that is not inline-parsed. It needs no
// finaliser: on close it emits one [token.Leaf] with Literal set, exactly as a
// code fence does, so the renderer receives the raw joined lines.
func (s *BlockState) StartLiteral(tag token.Tag) {
	s.closeLeaf()
	s.noteDirectChild()
	lf := s.beginLeafTag(token.CustomLeaf, tag)
	lf.literal = true
}

// CloseLeaf closes the open leaf now, running its finaliser or projecting it
// exactly as reaching the end of its block would. A continuation calls it to end
// a multi-line leaf early — when a table meets a line that holds no pipe — so the
// rule loop then classifies that line with no leaf still open. It is a no-op when
// nothing is open.
func (s *BlockState) CloseLeaf() { s.closeLeaf() }
