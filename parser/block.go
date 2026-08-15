package parser

import (
	"strings"
	"unicode/utf8"

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
	node       token.Node
	marker     byte
	ordered    bool
	loose      bool // List: a confirmed blank-separated item/block
	separated  bool // List: previous item ended after a pending blank
	hasChild   bool // ListItem: at least one direct block has begun
	pending    bool // ListItem: a blank may separate the next direct block/item
	descendant bool // ListItem: trailing blank escaped a nested list
	start      int
	indent     int // visual columns consumed by a ListItem prefix
	parentList int // ListItem: stable stack index of its owning List
	openEvent  int // List: index of its held OpenBlock event
}

type openLeafKind uint8

const (
	leafOrdinary openLeafKind = iota
	leafFencedCode
	leafIndentedCode
)

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
	kind         openLeafKind
	literal      bool // content is verbatim (a code fence, a math block)
	pendingBlank int  // trailing blank lines held by an indented code block
	blankFrom    int  // shallowest list-item depth affected by pending blanks
	context      token.InlineContext
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
	held   []token.BlockEvent // unresolved outer-list suffix

	holdingLists bool
	listDepth    int
	references   referenceResolver
	// extensionColumn is valid only while a public extension Open method runs.
	// Private built-in rules carry the richer blockLine value directly.
	extensionColumn int

	collected []token.BlockEvent // whole-document buffer, used only by the fan-out path
}

func NewBlockState(rules *RuleSet) *BlockState { return &BlockState{rules: rules} }

// Reset rewinds the state for reuse from a pool, keeping backing arrays while
// clearing every slot that could otherwise retain the consumed document.
func (s *BlockState) Reset(rules *RuleSet) {
	s.rules = rules
	s.stack = s.stack[:0]
	s.leaf = nil
	lines := s.leafStore.lines
	clear(lines)
	s.leafStore = openLeaf{lines: lines[:0], blankFrom: -1}
	s.seq = 0
	clear(s.events)
	s.events = s.events[:0]
	clear(s.held)
	s.held = s.held[:0]
	clear(s.collected)
	s.collected = s.collected[:0]
	s.holdingLists = false
	s.listDepth = 0
	s.extensionColumn = 0
	s.references.reset()
}

// FeedLine feeds one line (without its newline) and returns the block events it
// triggered.
//
// The returned slice reuses one backing array: the caller must consume it before
// the next feedLine/closeAll. Every driver in this package does exactly that,
// which buys us zero allocations per line.
func (s *BlockState) FeedLine(raw string) []token.BlockEvent {
	clear(s.events)
	s.events = s.events[:0]
	line := newBlockLine(normalizeSourceLine(raw))

	// Fenced code wins: while the container prefix still matches, the line is
	// either code or the closing fence.
	if s.leaf != nil && s.leaf.kind == leafFencedCode {
		rest, matched, _ := s.matchPrefix(line)
		if matched == len(s.stack) {
			if s.isClosingFence(rest) {
				s.closeLeaf()
			} else {
				s.leaf.lines = append(s.leaf.lines, stripIndentLine(rest, s.leaf.fenceIndent))
			}
			return s.events
		}
		s.closeLeaf()
		s.closeContainersFrom(matched)
		s.continueLine(rest)
		return s.events
	}

	// Indented code owns blank and sufficiently-indented continuation lines. A
	// non-matching line closes it and is classified once through the ordinary
	// block path below; no previously consumed input is revisited.
	if s.leaf != nil && s.leaf.kind == leafIndentedCode {
		rest, matched, blankFrom := s.matchPrefix(line)
		if matched < len(s.stack) {
			s.commitIndentedBlank()
			s.closeLeaf()
			s.closeContainersFrom(matched)
			s.continueLine(rest)
			return s.events
		}
		switch {
		case rest.blank():
			s.appendIndentedBlank(rest, blankFrom)
		case rest.leadingIndent() >= 4:
			s.appendIndentedLine(rest)
		default:
			s.commitIndentedBlank()
			s.closeLeaf()
			s.continueLine(rest)
		}
		return s.events
	}

	rest, matched, blankFrom := s.matchPrefix(line)
	if rest.blank() {
		s.noteBlankLine(blankFrom)
	}
	if matched < len(s.stack) {
		if s.continuesParagraphLazily(rest, matched) {
			s.AppendParagraphLine(rest.Text())
			return s.events
		}
		s.closeLeaf()
		s.closeContainersFrom(matched)
	}
	s.continueLine(rest)
	return s.events
}

// ReleaseEvents declares that the caller has finished consuming the batch most
// recently returned by FeedLine or CloseAll. It clears payload references while
// retaining the backing array for reuse; the returned batch must not be read
// after this call. Higher-level document drivers call it immediately after
// their synchronous consumer returns.
func (s *BlockState) ReleaseEvents() {
	clear(s.events)
	s.events = s.events[:0]
}

// normalizeSourceLine applies CommonMark's mandatory U+0000 replacement and
// gives Go's byte-oriented string APIs a deterministic invalid-UTF-8 policy.
// strings.Map decodes one invalid unit at a time, so adjacent invalid bytes each
// become U+FFFD. The overwhelmingly common clean path returns raw unchanged.
func normalizeSourceLine(raw string) string {
	if strings.IndexByte(raw, 0) < 0 && utf8.ValidString(raw) {
		return raw
	}
	return strings.Map(func(r rune) rune {
		if r == 0 {
			return utf8.RuneError
		}
		return r
	}, raw)
}

// CloseAll closes every open block at end of input and returns the final events.
func (s *BlockState) CloseAll() []token.BlockEvent {
	clear(s.events)
	s.events = s.events[:0]
	s.closeLeaf()
	s.closeContainersFrom(0)
	return s.events
}

// SealReferences declares that no later definition can appear. Document
// drivers call it only after CloseAll has closed the final paragraph and
// registered every definition it contains.
func (s *BlockState) SealReferences() { s.references.seal() }

// HeldCount reports how many block events are currently held pending an open
// list's final tightness. These events have immutable content (the buffer is
// append-only until the outer list closes), so a streaming consumer can cache
// their rendering by position; everything a snapshot emits beyond this count is
// the volatile open tail and must be re-rendered.
func (s *BlockState) HeldCount() int { return len(s.held) }

// ReferenceFingerprint is an order-independent hash of every link reference
// definition — label, destination and title. A streaming consumer uses it as a
// version: it changes whenever a definition is added or its value differs (as a
// definition still being typed in a partial line can differ between snapshots),
// which is exactly when an earlier reference may render differently.
func (s *BlockState) ReferenceFingerprint() uint64 {
	var sum uint64
	for label, def := range s.references.definitions {
		h := fnvHash(fnvHash(fnvHash(fnvOffset, label), def.destination), def.title)
		sum += h // summation is commutative, so map iteration order does not matter
	}
	return sum
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

// fnvHash extends an FNV-1a hash with s and a trailing separator so that
// concatenation boundaries cannot collide (e.g. "ab"+"c" vs "a"+"bc").
func fnvHash(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * fnvPrime
	}
	return (h ^ 0xff) * fnvPrime
}

// StartInline parses one closed leaf against this document's definitions. It
// returns a cursor only when the scan reaches a syntactically valid reference
// whose first definition may still appear later in the document.
func (s *BlockState) StartInline(leaf token.Leaf) ([]token.Inline, *InlineCursor) {
	if leaf.Literal || leaf.Content == "" {
		return nil, nil
	}
	switch leaf.Node {
	case token.Heading, token.Paragraph, token.CustomLeaf:
		tokens, cursor, _ := s.rules.inline.startContext(leaf.Content, leaf.Context, &s.references)
		return tokens, cursor
	default:
		return nil, nil
	}
}

// ParseInlineFinal parses a leaf after SealReferences. It is safe to call from
// parallel workers because the sealed resolver is read-only and InlineRules
// owns a concurrency-safe scratch pool.
func (s *BlockState) ParseInlineFinal(leaf token.Leaf) []token.Inline {
	if !s.references.sealed {
		panic("mdflow/parser: ParseInlineFinal called before SealReferences")
	}
	tokens, cursor := s.StartInline(leaf)
	if cursor == nil {
		return tokens
	}
	tokens, complete := cursor.Resume()
	if !complete {
		panic("mdflow/parser: sealed document left an unresolved inline cursor")
	}
	return tokens
}

// Total reports how many events have been emitted so far.
func (s *BlockState) Total() int { return s.seq }

// ---- the three phases ----

// matchPrefix walks the container stack bottom-up, stripping matched prefixes.
func (s *BlockState) matchPrefix(line blockLine) (rest blockLine, matched, blankFrom int) {
	rest = line
	blankFrom = -1
	if line.blank() {
		blankFrom = 0
	}
	for i := range s.stack {
		r, ok := continueContainer(&s.stack[i], rest)
		if !ok {
			return rest, i, blankFrom
		}
		rest = r
		matched = i + 1
		if blankFrom < 0 && rest.blank() {
			blankFrom = matched
		}
	}
	return rest, matched, blankFrom
}

// continuesParagraphLazily reports whether a line whose container prefixes did
// not all match still belongs to the open paragraph. CommonMark permits that
// omission only for non-blank paragraph continuation text; a line that can
// start an interrupting block closes the unmatched containers instead.
func (s *BlockState) continuesParagraphLazily(line blockLine, matched int) bool {
	if s.leaf == nil || s.leaf.node != token.Paragraph || line.blank() {
		return false
	}
	// Once a list container matched but its current item did not, any valid
	// marker begins the next item (or a differently-delimited list). The usual
	// start-at-1 paragraph-interruption restriction no longer applies because
	// the missing ListItem prefix has already ended that item.
	if matched > 0 && matched < len(s.stack) &&
		s.stack[matched-1].node == token.List && s.stack[matched].node == token.ListItem {
		if _, ok := parseListMarkerLine(line); ok {
			return false
		}
	}
	for _, rule := range s.rules.containerRules {
		if interruptsParagraph(rule, line) {
			return false
		}
	}
	for _, rule := range s.rules.leafRules {
		if interruptsParagraph(rule, line) {
			return false
		}
	}
	return true
}

// blockParagraphInterruptor keeps the built-ins on blockLine so their tab and
// visual-column decisions stay exact. Extensions use the public string-based
// interruption capabilities, optionally with the remainder's starting column,
// without depending on parser internals.
type blockParagraphInterruptor interface {
	interruptsParagraphBlockLine(blockLine) bool
}

func interruptsParagraph(rule any, line blockLine) bool {
	if r, ok := rule.(blockParagraphInterruptor); ok {
		return r.interruptsParagraphBlockLine(line)
	}
	if r, ok := rule.(ColumnParagraphInterruptor); ok {
		return r.InterruptsParagraphAt(line.Text(), line.column)
	}
	if r, ok := rule.(ParagraphInterruptor); ok {
		return r.InterruptsParagraph(line.Text())
	}
	return false
}

// continueContainer decides whether an open container continues on this line.
func continueContainer(c *container, line blockLine) (blockLine, bool) {
	switch c.node {
	case token.Blockquote:
		return stripBlockquoteMarkerLine(line)
	case token.List:
		return line, true // the list container itself consumes no prefix
	case token.ListItem:
		if line.blank() {
			return line, c.hasChild
		}
		if line.leadingIndent() < c.indent {
			return line, false
		}
		rest := line.consumeIndent(c.indent)
		return rest, true
	}
	return line, true
}

// continueLine runs phases two and three: open as many containers as possible,
// then classify the remainder as a leaf.
func (s *BlockState) continueLine(rest blockLine) {
	// A multi-line leaf that owns its own terminator (a table's body rows, a
	// math block's contents) claims the line before any rule gets a say. The
	// handlers come from RuleSet, so the core loop needs no knowledge of them.
	if s.leaf != nil {
		for _, fn := range s.rules.continuations {
			if fn(s, rest.Text()) {
				return
			}
		}
	}

	for {
		if rest.blank() || isThematicBreakBlockLine(rest) {
			break // a thematic break outranks a list marker
		}
		opened := false
		for _, cr := range s.rules.containerRules {
			if r, ok := openContainerRule(cr, s, rest); ok {
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
	if top := s.top(); top != nil && top.node == token.List && !rest.blank() {
		s.closeLeaf()
		s.closeContainersFrom(len(s.stack) - 1)
	}

	for _, lr := range s.rules.leafRules {
		if openLeafRule(lr, s, rest) {
			return
		}
	}
	openLeafRule(s.rules.paragraph, s, rest)
}

// ---- operations exposed to rules ----

// RemainderColumn reports the zero-based visual column where the line passed to
// the current public ContainerRule.Open or LeafRule.Open call begins. Outside
// such a call it returns zero. The line itself is unchanged; extensions that
// interpret leading tabs can combine both values without importing blockLine.
func (s *BlockState) RemainderColumn() int { return s.extensionColumn }

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

// AppendParagraphLine folds one line into the open paragraph, starting one if
// needed.
//
// Leading block indentation is not inline content, so it is removed here.
// Trailing whitespace is preserved until the inline scan: only that layer knows
// whether it belongs to code/raw HTML or spells a hard/soft line break.
func (s *BlockState) AppendParagraphLine(line string) {
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
func (s *BlockState) Clone() *BlockState {
	out := &BlockState{
		rules: s.rules, seq: s.seq,
		holdingLists: s.holdingLists, listDepth: s.listDepth,
		references: s.references.clone(),
	}
	out.stack = append([]container(nil), s.stack...)
	out.held = append([]token.BlockEvent(nil), s.held...)
	if s.leaf != nil {
		out.leafStore = *s.leaf // scratch is immutable after StartAccumulator
		out.leafStore.lines = append([]string(nil), s.leaf.lines...)
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
	previousColumn := s.extensionColumn
	s.extensionColumn = line.column
	rest, ok := r.Open(s, before)
	s.extensionColumn = previousColumn
	if !ok {
		return line, false
	}
	return line.afterExtensionRemainder(before, rest), true
}

func openLeafRule(r LeafRule, s *BlockState, line blockLine) bool {
	if br, ok := r.(blockLeafRule); ok {
		return br.openBlockLine(s, line)
	}
	previousColumn := s.extensionColumn
	s.extensionColumn = line.column
	ok := r.Open(s, line.Text())
	s.extensionColumn = previousColumn
	return ok
}

// blockquoteRule opens a blockquote.
type blockquoteRule struct{}

func (blockquoteRule) Name() string { return "blockquote" }
func (blockquoteRule) Open(s *BlockState, line string) (string, bool) {
	r, ok := (blockquoteRule{}).openBlockLine(s, newBlockLine(line))
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
func (listRule) Open(s *BlockState, line string) (string, bool) {
	r, ok := (listRule{}).openBlockLine(s, newBlockLine(line))
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
func (blankRule) Open(s *BlockState, line string) bool {
	return (blankRule{}).openBlockLine(s, newBlockLine(line))
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
func (setextHeadingRule) Open(s *BlockState, line string) bool {
	return (setextHeadingRule{}).openBlockLine(s, newBlockLine(line))
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
func (thematicBreakRule) Open(s *BlockState, line string) bool {
	return (thematicBreakRule{}).openBlockLine(s, newBlockLine(line))
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
func (atxHeadingRule) Open(s *BlockState, line string) bool {
	return (atxHeadingRule{}).openBlockLine(s, newBlockLine(line))
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
func (fenceRule) Open(s *BlockState, line string) bool {
	return (fenceRule{}).openBlockLine(s, newBlockLine(line))
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
func (indentedCodeRule) Open(s *BlockState, line string) bool {
	return (indentedCodeRule{}).openBlockLine(s, newBlockLine(line))
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
func (paragraphRule) Open(s *BlockState, line string) bool {
	s.AppendParagraphLine(line)
	return true
}
func (paragraphRule) openBlockLine(s *BlockState, line blockLine) bool {
	s.AppendParagraphLine(line.Text())
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

func stripIndentLine(line blockLine, columns int) string {
	rest := line.consumeIndent(columns)
	return rest.Text()
}

func stripBlockquoteMarkerLine(line blockLine) (blockLine, bool) {
	ind := line.leadingIndent()
	if ind > 3 {
		return line, false
	}
	rest := line.consumeIndent(ind)
	var ok bool
	if rest, ok = rest.consumeByte('>'); !ok {
		return line, false
	}
	if c, exists := rest.peek(); exists && (c == ' ' || c == '\t') {
		rest = rest.consumeIndent(1)
	}
	return rest, true
}

type listMarkerInfo struct {
	marker  byte
	ordered bool
	blank   bool
	start   int
	width   int
	rest    blockLine
}

func parseListMarkerLine(line blockLine) (listMarkerInfo, bool) {
	ind := line.leadingIndent()
	if ind > 3 {
		return listMarkerInfo{}, false
	}
	r := line.consumeIndent(ind)
	if marker, ok := r.peek(); ok && (marker == '-' || marker == '*' || marker == '+') {
		afterMarker, _ := r.consumeByte(marker)
		return finishListMarker(line, afterMarker, listMarkerInfo{marker: marker})
	}
	n, start := 0, 0
	for n < 9 {
		c, ok := r.peek()
		if !ok || c < '0' || c > '9' {
			break
		}
		start = start*10 + int(c-'0')
		r, _ = r.consumeByte(c)
		n++
	}
	if c, ok := r.peek(); n >= 1 && ok && (c == '.' || c == ')') {
		afterMarker, _ := r.consumeByte(c)
		return finishListMarker(line, afterMarker, listMarkerInfo{marker: c, ordered: true, start: start})
	}
	return listMarkerInfo{}, false
}

func finishListMarker(original, afterMarker blockLine, info listMarkerInfo) (listMarkerInfo, bool) {
	padding := afterMarker.leadingIndent()
	if afterMarker.blank() {
		// An empty item always acts as if the marker had one following space,
		// regardless of how much trailing whitespace the source contains.
		info.blank = true
		info.width = afterMarker.column - original.column + 1
		info.rest = afterMarker.consumeIndent(padding)
		return info, true
	}
	if padding == 0 {
		return listMarkerInfo{}, false
	}
	if padding > 4 {
		padding = 1
	}
	info.rest = afterMarker.consumeIndent(padding)
	info.width = info.rest.column - original.column
	return info, true
}

func isThematicBreakBlockLine(line blockLine) bool {
	ind := line.leadingIndent()
	if ind > 3 {
		return false
	}
	rest := line.consumeIndent(ind)
	s := rest.trimRightSpaceTab()
	if s == "" {
		return false
	}
	var ch byte
	count := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
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

// parseSetextUnderlineLine recognises `===` (h1) and `---` (h2) underlines.
func parseSetextUnderlineLine(line blockLine) (int, bool) {
	ind := line.leadingIndent()
	if ind > 3 {
		return 0, false
	}
	rest := line.consumeIndent(ind)
	r := rest.trimRightSpaceTab()
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

func parseATXHeadingLine(line blockLine) (level int, content string, ok bool) {
	ind := line.leadingIndent()
	if ind > 3 {
		return 0, "", false
	}
	rest := line.consumeIndent(ind)
	r := rest.Text()
	n := 0
	for n < len(r) && r[n] == '#' {
		n++
	}
	if n < 1 || n > 6 {
		return 0, "", false
	}
	if len(r) > n && r[n] != ' ' && r[n] != '\t' {
		return 0, "", false
	}
	content = strings.TrimSpace(r[n:])
	if trimmed := strings.TrimRight(content, "#"); trimmed != content {
		if trimmed == "" {
			content = ""
		} else if strings.HasSuffix(trimmed, " ") || strings.HasSuffix(trimmed, "\t") {
			content = strings.TrimRight(trimmed, " \t")
		}
	}
	return n, content, true
}

func parseFenceOpenLine(line blockLine) (ch byte, length, indent int, info string, ok bool) {
	ind := line.leadingIndent()
	if ind > 3 {
		return 0, 0, 0, "", false
	}
	restLine := line.consumeIndent(ind)
	r := restLine.Text()
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
