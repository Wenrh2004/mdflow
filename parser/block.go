package parser

import (
	"strconv"
	"strings"

	"github.com/Wenrh2004/mdflow/token"
)

// Block-level parsing: a line-driven state machine split into "state + rules".
//
//	BlockState                per-parse mutable state: container stack, at most
//	                          one open leaf, and an event buffer
//	ContainerRule / LeafRule  registrable rules (see config.go)
//
// Every incoming line does three things (phase 1 of CommonMark's appendix A):
//
//	1. matchPrefix    match the existing container prefixes; close what fails
//	2. open           try to open new containers
//	3. classify       hand the remainder to the leaf rules (paragraph catches all)
//
// The entire state is small enough to draw on a napkin, and that is precisely
// what makes streaming work: a closed block is never re-parsed, so appending
// input only ever touches the top of the stack. Incremental cost is O(new lines),
// not O(document).

// container is one frame of the container stack.
type container struct {
	node    token.Node
	marker  byte
	ordered bool
	start   int
	indent  int
	task    int8
}

// openLeaf is the currently open leaf block: a paragraph, a fenced block, or an
// accumulating custom leaf (a table, a math block) buffering lines until it
// closes.
type openLeaf struct {
	lines        []string
	scratch      any // opaque per-leaf state a finaliser reads back; written once at open
	info         string
	fenceLen     int
	fenceIndent  int
	node         token.Node
	tag          token.Tag // CustomLeaf discriminator, and the accumulator's own marker
	fenceChar    byte
	literal      bool // content is verbatim (a code fence, a math block)
	pendingBreak bool // last paragraph line ended in hard-break spaces; the break
	// is only real if a continuation line follows, so we hold it here until one does
}

// BlockState is the block state machine's mutable state. Rules read and write
// the container stack through it and emit events.
type BlockState struct {
	rules     *RuleSet
	stack     []container
	leaf      *openLeaf // points at leafStore while open, else nil
	leafStore openLeaf  // the single leaf slot: at most one leaf is open at a
	// time, so the whole document reuses one, and its lines
	// backing array with it
	seq    int
	events []token.BlockEvent

	collected []token.BlockEvent // whole-document buffer, used only by the fan-out path
}

func NewBlockState(rules *RuleSet) *BlockState { return &BlockState{rules: rules} }

// Reset rewinds the state for reuse from a pool, keeping every backing array.
func (s *BlockState) Reset(rules *RuleSet) {
	s.rules = rules
	s.stack = s.stack[:0]
	s.leaf = nil
	s.leafStore.scratch = nil // drop the last accumulator's scratch for the GC
	s.seq = 0
	s.events = s.events[:0]
}

// FeedLine feeds one line (without its newline) and returns the block events it
// triggered.
//
// The returned slice reuses one backing array: the caller must consume it before
// the next feedLine/closeAll. Every driver in this package does exactly that,
// which buys us zero allocations per line.
func (s *BlockState) FeedLine(line string) []token.BlockEvent {
	s.events = s.events[:0]

	// Fenced code wins: while the container prefix still matches, the line is
	// either code or the closing fence.
	if s.leaf != nil && s.leaf.node == token.CodeBlock {
		rest, matched := s.matchPrefix(line)
		if matched == len(s.stack) {
			if s.isClosingFence(rest) {
				s.closeLeaf()
			} else {
				s.leaf.lines = append(s.leaf.lines, stripIndent(rest, s.leaf.fenceIndent))
			}
			return s.events
		}
		s.closeLeaf()
		s.closeContainersFrom(matched)
		s.continueLine(rest)
		return s.events
	}

	rest, matched := s.matchPrefix(line)
	if matched < len(s.stack) {
		s.closeLeaf()
		s.closeContainersFrom(matched)
	}
	s.continueLine(rest)
	return s.events
}

// CloseAll closes every open block at end of input and returns the final events.
func (s *BlockState) CloseAll() []token.BlockEvent {
	s.events = s.events[:0]
	s.closeLeaf()
	s.closeContainersFrom(0)
	return s.events
}

// Total reports how many events have been emitted so far.
func (s *BlockState) Total() int { return s.seq }

// ---- the three phases ----

// matchPrefix walks the container stack bottom-up, stripping matched prefixes.
func (s *BlockState) matchPrefix(line string) (rest string, matched int) {
	rest = line
	for i := range s.stack {
		r, ok := continueContainer(&s.stack[i], rest)
		if !ok {
			return rest, i
		}
		rest = r
		matched = i + 1
	}
	return rest, matched
}

// continueContainer decides whether an open container continues on this line.
func continueContainer(c *container, line string) (string, bool) {
	switch c.node {
	case token.Blockquote:
		return stripBlockquoteMarker(line)
	case token.List:
		return line, true // the list container itself consumes no prefix
	case token.ListItem:
		if isBlankLine(line) || leadingSpaces(line) < c.indent {
			return line, false
		}
		return line[c.indent:], true
	}
	return line, true
}

// continueLine runs phases two and three: open as many containers as possible,
// then classify the remainder as a leaf.
func (s *BlockState) continueLine(rest string) {
	// A multi-line leaf that owns its own terminator (a table's body rows, a
	// math block's contents) claims the line before any rule gets a say. The
	// handlers come from RuleSet, so the core loop needs no knowledge of them.
	if s.leaf != nil {
		for _, fn := range s.rules.continuations {
			if fn(s, rest) {
				return
			}
		}
	}

	for {
		if isBlankLine(rest) || isThematicBreakLine(rest) {
			break // a thematic break outranks a list marker
		}
		opened := false
		for _, cr := range s.rules.containerRules {
			if r, ok := cr.Open(s, rest); ok {
				rest = r
				opened = true
				break
			}
		}
		if !opened {
			break
		}
	}

	// Dangling list: a List on top means the current item already closed.
	if top := s.top(); top != nil && top.node == token.List && !isBlankLine(rest) {
		s.closeLeaf()
		s.closeContainersFrom(len(s.stack) - 1)
	}

	for _, lr := range s.rules.leafRules {
		if lr.Open(s, rest) {
			return
		}
	}
	s.rules.paragraph.Open(s, rest)
}

// ---- operations exposed to rules ----

// EmitLeaf emits a terminal leaf block, closing any open leaf first.
func (s *BlockState) EmitLeaf(leaf token.Leaf) {
	s.closeLeaf()
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: leaf})
}

// InTightItem reports whether we are inside a tight list item, in which case
// paragraphs render without a <p> wrapper.
func (s *BlockState) InTightItem() bool {
	t := s.top()
	return t != nil && t.node == token.ListItem
}

// OpenParagraphLines returns the lines of the currently open paragraph, or nil.
// Rules that need one line of look-back (setext headings, GFM tables) use it.
func (s *BlockState) OpenParagraphLines() []string {
	if s.leaf == nil || s.leaf.node != token.Paragraph {
		return nil
	}
	return s.leaf.lines
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
	s.leafStore = openLeaf{node: node, tag: tag, lines: s.leafStore.lines[:0]}
	s.leaf = &s.leafStore
	return s.leaf
}

// AppendParagraphLine folds one line into the open paragraph, starting one if
// needed.
//
// Trailing whitespace is normalised here rather than in the inline scanner: a
// line ending in two or more spaces is the *other* spelling of a hard break, so
// we rewrite it to the backslash form and let one inline rule handle both. The
// rewrite is deferred: a hard break only means something when another line
// follows, so the trailing spaces are remembered as pendingBreak and turned into
// a backslash on the previous line only once a continuation line arrives. At the
// end of a paragraph the pending break is dropped, matching CommonMark's rule
// that trailing spaces on a final line are stripped, not a break.
func (s *BlockState) AppendParagraphLine(line string) {
	if s.leaf == nil {
		s.beginLeaf(token.Paragraph)
	}
	if s.leaf.pendingBreak {
		last := len(s.leaf.lines) - 1
		s.leaf.lines[last] += "\\"
		s.leaf.pendingBreak = false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed != "" && hasHardBreakSpaces(line) {
		s.leaf.pendingBreak = true
	}
	s.leaf.lines = append(s.leaf.lines, trimmed)
}

// startCodeBlock opens a fenced code leaf.
func (s *BlockState) startCodeBlock(info string, ch byte, length, indent int) {
	s.closeLeaf()
	lf := s.beginLeaf(token.CodeBlock)
	lf.info, lf.fenceChar, lf.fenceLen, lf.fenceIndent = info, ch, length, indent
	lf.literal = true
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
	lf := s.beginLeafTag(token.CustomLeaf, tag)
	lf.scratch = scratch
}

// PromoteParagraph is [StartAccumulator] for a leaf that grows out of the open
// paragraph rather than a fresh line — a GFM table, whose header is the
// paragraph line above the delimiter row. It absorbs that paragraph's lines
// instead of emitting them, so the header is the accumulator's first line.
func PromoteParagraph[S any](s *BlockState, tag token.Tag, scratch S) {
	var lines []string
	if s.leaf != nil && s.leaf.node == token.Paragraph {
		lines = append(lines, s.leaf.lines...)
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
	lf := s.beginLeafTag(token.CustomLeaf, tag)
	lf.literal = true
}

// CloseLeaf closes the open leaf now, running its finaliser or projecting it
// exactly as reaching the end of its block would. A continuation calls it to end
// a multi-line leaf early — when a table meets a line that holds no pipe — so the
// rule loop then classifies that line with no leaf still open. It is a no-op when
// nothing is open.
func (s *BlockState) CloseLeaf() { s.closeLeaf() }

// ---- events and stack ----

// Emit appends a block event to the current batch, stamping it with the next
// document ordinal. It is how a [Finalise] turns a closed accumulating leaf into
// output: a table finaliser emits its container-open, row, cell and
// container-close events in order, and the core neither knows nor cares what
// tree they describe.
func (s *BlockState) Emit(ev token.BlockEvent) { s.emit(ev) }

func (s *BlockState) emit(ev token.BlockEvent) {
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
	s.emit(token.BlockEvent{Type: token.OpenBlock, Container: c.node, Ordered: c.ordered, Start: c.start, Task: c.task})
	s.stack = append(s.stack, c)
}

func (s *BlockState) closeContainersFrom(idx int) {
	for len(s.stack) > idx {
		c := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		s.emit(token.BlockEvent{Type: token.CloseBlock, Container: c.node, Ordered: c.ordered, Start: c.start, Task: c.task})
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
		fn(s, lf.lines, lf.scratch)
		return
	}
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: s.buildLeaf(lf)})
}

// buildLeaf projects an open leaf into its closed form. A finaliser-backed leaf
// never reaches here; this covers paragraphs, headings and the literal blocks
// (code and math fences), which are one leaf apiece.
func (s *BlockState) buildLeaf(lf *openLeaf) token.Leaf {
	leaf := token.Leaf{Node: lf.node, Tag: lf.tag, Level: lf.fenceLen, Info: lf.info, Tight: s.InTightItem()}
	switch {
	case lf.literal:
		leaf.Level = 0
		leaf.Literal = true
		if len(lf.lines) > 0 {
			leaf.Content = joinLines(lf.lines) + "\n"
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

// OpenState returns the not-yet-closed tail of the document: the open container
// frames outermost first, and a view of the open leaf in one of two shapes.
//
// A leaf that projects to a single block — a paragraph, a heading, a literal
// fence — comes back as leaf, ready to render. A leaf that closes into many
// events — a GFM table — has no single-block projection, so it comes back as
// tail: the events its finaliser would emit, run on a throwaway snapshot so the
// live parse keeps its open leaf untouched. Exactly one of leaf and tail is ever
// non-nil.
//
// It returns data rather than rendering, because rendering is another layer's
// job. The facade turns this into the "provisional" HTML a streaming UI shows
// between chunks.
func (s *BlockState) OpenState() (containers []token.BlockEvent, leaf *token.Leaf, tail []token.BlockEvent) {
	if len(s.stack) == 0 && s.leaf == nil {
		return nil, nil, nil
	}
	containers = make([]token.BlockEvent, 0, len(s.stack))
	for i := range s.stack {
		c := s.stack[i]
		containers = append(containers, token.BlockEvent{
			Type: token.OpenBlock, Container: c.node, Ordered: c.ordered, Start: c.start, Task: c.task,
		})
	}
	if s.leaf == nil {
		return containers, nil, nil
	}
	if fn := s.rules.finalisers[s.leaf.tag]; fn != nil {
		// A multi-event leaf has no half-open projection: finalise a snapshot so
		// the caller sees the whole construct (a header-only table, say) while the
		// real parse keeps accumulating.
		snap := s.Clone()
		lf := snap.leaf
		snap.leaf = nil
		fn(snap, lf.lines, lf.scratch)
		return containers, nil, snap.events
	}
	l := s.buildLeaf(s.leaf)
	return containers, &l, nil
}

// Clone snapshots the state machine so a caller can speculatively feed it a
// partial line without disturbing the real parse. Used for the provisional view
// of a streaming tail.
func (s *BlockState) Clone() *BlockState {
	out := &BlockState{rules: s.rules, seq: s.seq}
	out.stack = append([]container(nil), s.stack...)
	if s.leaf != nil {
		out.leafStore = *s.leaf // copies scratch shallowly, which is the contract
		out.leafStore.lines = append([]string(nil), s.leaf.lines...)
		// Scratch is written once at open and only read afterwards, so sharing it
		// is safe. A scratch that holds mutable state opts into a deep copy.
		if cs, ok := s.leaf.scratch.(interface{ CloneScratch() any }); ok {
			out.leafStore.scratch = cs.CloneScratch()
		}
		out.leaf = &out.leafStore
	}
	return out
}

// CollectAll parses src to completion and returns every block event at once,
// which is what the fan-out path needs in order to partition the document.
//
// The sequential path never calls this: it streams events and keeps only the
// open container stack, so materialising the whole document is the memory cost
// that distinguishes the two. Callers must finish with the returned slice
// before this BlockState is reused — the slice is its internal buffer, kept and
// regrown across documents rather than reallocated per parse, which measured
// larger than everything the fan-out saves.
func (s *BlockState) CollectAll(src string) []token.BlockEvent {
	all := s.collected[:0]
	EachLine(src, func(line string) bool {
		all = append(all, s.FeedLine(line)...)
		return true
	})
	all = append(all, s.CloseAll()...)
	s.collected = all // keep the grown array for the next document
	return all
}

// isClosingFence recognises the closing fence of the open code block.
func (s *BlockState) isClosingFence(line string) bool {
	ind := leadingSpaces(line)
	if ind > 3 {
		return false
	}
	r := strings.TrimRight(line[ind:], " ")
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

// ---- container rules ----

// blockquoteRule opens a blockquote.
type blockquoteRule struct{}

func (blockquoteRule) Name() string { return "blockquote" }
func (blockquoteRule) Open(s *BlockState, line string) (string, bool) {
	if r, ok := stripBlockquoteMarker(line); ok {
		s.closeLeaf()
		s.pushContainer(container{node: token.Blockquote})
		return r, true
	}
	return line, false
}

// listRule opens a list plus its item, switching list type when the marker changes.
type listRule struct{}

func (listRule) Name() string { return "list" }
func (listRule) Open(s *BlockState, line string) (string, bool) {
	m, ok := parseListMarker(line)
	if !ok {
		return line, false
	}
	s.closeLeaf()
	if top := s.top(); top != nil && top.node == token.List && top.marker != m.marker {
		s.closeContainersFrom(len(s.stack) - 1) // a different marker starts a new list
	}
	if top := s.top(); top == nil || top.node != token.List {
		s.pushContainer(container{node: token.List, marker: m.marker, ordered: m.ordered, start: m.start})
	}
	rest, task := splitTaskMarker(m.rest)
	s.pushContainer(container{node: token.ListItem, indent: m.width, task: task})
	return rest, true
}

// ---- leaf rules ----

// blankRule: a blank line closes the open leaf.
type blankRule struct{}

func (blankRule) Name() string { return "blank" }
func (blankRule) Open(s *BlockState, line string) bool {
	if isBlankLine(line) {
		s.closeLeaf()
		return true
	}
	return false
}

// setextHeadingRule upgrades an open paragraph to a heading when the line is a
// setext underline. It is registered ahead of thematicBreakRule so that `---`
// under a paragraph reads as an <h2> rather than an <hr>, matching CommonMark.
type setextHeadingRule struct{}

func (setextHeadingRule) Name() string { return "setext_heading" }
func (setextHeadingRule) Open(s *BlockState, line string) bool {
	lines := s.OpenParagraphLines()
	if len(lines) == 0 {
		return false
	}
	level, ok := parseSetextUnderline(line)
	if !ok {
		return false
	}
	content := joinLines(lines)
	s.leaf = nil // consume the paragraph without emitting it
	s.emit(token.BlockEvent{Type: token.LeafBlock, Leaf: token.Leaf{Node: token.Heading, Level: level, Content: content, Tight: s.InTightItem()}})
	return true
}

// thematicBreakRule handles `---`, `***`, `___`.
type thematicBreakRule struct{}

func (thematicBreakRule) Name() string { return "thematic_break" }
func (thematicBreakRule) Open(s *BlockState, line string) bool {
	if isThematicBreakLine(line) {
		s.EmitLeaf(token.Leaf{Node: token.ThematicBreak})
		return true
	}
	return false
}

// atxHeadingRule handles `## Heading`.
type atxHeadingRule struct{}

func (atxHeadingRule) Name() string { return "atx_heading" }
func (atxHeadingRule) Open(s *BlockState, line string) bool {
	if level, content, ok := parseATXHeading(line); ok {
		s.EmitLeaf(token.Leaf{Node: token.Heading, Level: level, Content: content, Tight: s.InTightItem()})
		return true
	}
	return false
}

// fenceRule opens a fenced code block.
type fenceRule struct{}

func (fenceRule) Name() string { return "fenced_code" }
func (fenceRule) Open(s *BlockState, line string) bool {
	if ch, n, indent, info, ok := parseFenceOpen(line); ok {
		s.startCodeBlock(info, ch, n, indent)
		return true
	}
	return false
}

// paragraphRule is the catch-all: it always matches.
type paragraphRule struct{}

func (paragraphRule) Name() string { return "paragraph" }
func (paragraphRule) Open(s *BlockState, line string) bool {
	s.AppendParagraphLine(line)
	return true
}

// ---- line predicates (pure) ----

// IsBlank reports whether line is empty or holds only whitespace. A continuation
// handler in another module uses it to recognise the blank line that ends its
// multi-line leaf, so the same notion of "blank" the core uses is available
// without re-deriving it.
func IsBlank(line string) bool { return isBlankLine(line) }

func isBlankLine(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' {
			return false
		}
	}
	return true
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func stripIndent(s string, n int) string {
	i := 0
	for i < n && i < len(s) && s[i] == ' ' {
		i++
	}
	return s[i:]
}

func hasHardBreakSpaces(line string) bool {
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == ' '; i-- {
		n++
	}
	return n >= 2
}

func stripBlockquoteMarker(s string) (string, bool) {
	ind := leadingSpaces(s)
	if ind > 3 || ind >= len(s) || s[ind] != '>' {
		return s, false
	}
	r := s[ind+1:]
	if len(r) > 0 && r[0] == ' ' {
		r = r[1:]
	}
	return r, true
}

// splitTaskMarker recognises the GFM task-list prefix `[ ] ` / `[x] `.
func splitTaskMarker(rest string) (string, int8) {
	if len(rest) >= 3 && rest[0] == '[' && rest[2] == ']' {
		if len(rest) == 3 || rest[3] == ' ' {
			body := ""
			if len(rest) > 3 {
				body = rest[4:]
			}
			switch rest[1] {
			case ' ':
				return body, 1
			case 'x', 'X':
				return body, 2
			}
		}
	}
	return rest, 0
}

type listMarkerInfo struct {
	marker  byte
	ordered bool
	start   int
	width   int
	rest    string
}

func parseListMarker(s string) (listMarkerInfo, bool) {
	ind := leadingSpaces(s)
	if ind > 3 {
		return listMarkerInfo{}, false
	}
	r := s[ind:]
	if len(r) >= 2 && (r[0] == '-' || r[0] == '*' || r[0] == '+') && r[1] == ' ' {
		return listMarkerInfo{marker: r[0], width: ind + 2, rest: r[2:]}, true
	}
	n := 0
	for n < len(r) && n < 9 && r[n] >= '0' && r[n] <= '9' {
		n++
	}
	if n >= 1 && n+1 < len(r) && (r[n] == '.' || r[n] == ')') && r[n+1] == ' ' {
		start, _ := strconv.Atoi(r[:n])
		return listMarkerInfo{marker: r[n], ordered: true, start: start, width: ind + n + 2, rest: r[n+2:]}, true
	}
	return listMarkerInfo{}, false
}

func isThematicBreakLine(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	var ch byte
	count := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			continue
		}
		if c != '-' && c != '*' && c != '_' {
			return false
		}
		if ch == 0 {
			ch = c
		} else if c != ch {
			return false
		}
		count++
	}
	return count >= 3
}

// parseSetextUnderline recognises `===` (h1) and `---` (h2) underlines.
func parseSetextUnderline(s string) (int, bool) {
	ind := leadingSpaces(s)
	if ind > 3 {
		return 0, false
	}
	r := strings.TrimRight(s[ind:], " \t")
	if r == "" {
		return 0, false
	}
	c := r[0]
	if c != '=' && c != '-' {
		return 0, false
	}
	for i := 0; i < len(r); i++ {
		if r[i] != c {
			return 0, false
		}
	}
	if c == '=' {
		return 1, true
	}
	return 2, true
}

func parseATXHeading(s string) (level int, content string, ok bool) {
	ind := leadingSpaces(s)
	if ind > 3 {
		return 0, "", false
	}
	r := s[ind:]
	n := 0
	for n < len(r) && r[n] == '#' {
		n++
	}
	if n < 1 || n > 6 {
		return 0, "", false
	}
	if len(r) > n && r[n] != ' ' {
		return 0, "", false
	}
	content = strings.TrimSpace(r[n:])
	if trimmed := strings.TrimRight(content, "#"); trimmed != content {
		if trimmed == "" {
			content = ""
		} else if strings.HasSuffix(trimmed, " ") {
			content = strings.TrimRight(trimmed, " ")
		}
	}
	return n, content, true
}

func parseFenceOpen(s string) (ch byte, length, indent int, info string, ok bool) {
	ind := leadingSpaces(s)
	if ind > 3 {
		return 0, 0, 0, "", false
	}
	r := s[ind:]
	if len(r) < 3 || (r[0] != '`' && r[0] != '~') {
		return 0, 0, 0, "", false
	}
	c := r[0]
	n := 0
	for n < len(r) && r[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, 0, "", false
	}
	rest := strings.TrimSpace(r[n:])
	if c == '`' && strings.ContainsRune(rest, '`') {
		return 0, 0, 0, "", false
	}
	return c, n, ind, rest, true
}
