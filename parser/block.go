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

	collected []token.BlockEvent // whole-document buffer, used only by the fan-out path
}

func newBlockState(rules *RuleSet) *BlockState { return &BlockState{rules: rules} }

// Reset rewinds the state for reuse from a pool, keeping backing arrays while
// clearing every slot that could otherwise retain the consumed document.
func (s *BlockState) reset(rules *RuleSet) {
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
	s.references.reset()
}

// feedLine feeds one line (without its newline) and returns the block events it
// triggered.
//
// The returned slice reuses one backing array: the caller must consume it before
// the next feedLine/closeAll. Every driver in this package does exactly that,
// which buys us zero allocations per line.
func (s *BlockState) feedLine(raw string) []token.BlockEvent {
	clear(s.events)
	s.events = s.events[:0]
	s.references.input += int64(len(raw)) + 1
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
			s.appendParagraphLine(rest.Text())
			return s.events
		}
		s.closeLeaf()
		s.closeContainersFrom(matched)
	}
	s.continueLine(rest)
	return s.events
}

// releaseEvents declares that the caller has finished consuming the batch most
// recently returned by feedLine or closeAll. It clears payload references while
// retaining the backing array for reuse; the returned batch must not be read
// after this call. Higher-level document drivers call it immediately after
// their synchronous consumer returns.
func (s *BlockState) releaseEvents() {
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

// closeAll closes every open block at end of input and returns the final events.
func (s *BlockState) closeAll() []token.BlockEvent {
	clear(s.events)
	s.events = s.events[:0]
	s.closeLeaf()
	s.closeContainersFrom(0)
	return s.events
}

// sealReferences declares that no later definition can appear. Document
// drivers call it only after closeAll has closed the final paragraph and
// registered every definition it contains.
func (s *BlockState) sealReferences() { s.references.seal() }

// referencesRefused reports whether any reference in this document was refused
// by the expansion budget and rendered as literal text instead. The fan-out
// path uses it to fall back to document-order rendering, where the budget is
// charged deterministically.
func (s *BlockState) referencesRefused() bool { return s.references.refused.Load() }

// heldCount reports how many block events are currently held pending an open
// list's final tightness. These events have immutable content (the buffer is
// append-only until the outer list closes), so a streaming consumer can cache
// their rendering by position; everything a snapshot emits beyond this count is
// the volatile open tail and must be re-rendered.
func (s *BlockState) heldCount() int { return len(s.held) }

// referenceFingerprint is an order-independent hash of every link reference
// definition — label, destination and title. A streaming consumer uses it as a
// version: it changes whenever a definition is added or its value differs (as a
// definition still being typed in a partial line can differ between snapshots),
// which is exactly when an earlier reference may render differently.
func (s *BlockState) referenceFingerprint() uint64 {
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

// startInline parses one closed leaf against this document's definitions. It
// returns a cursor only when the scan reaches a syntactically valid reference
// whose first definition may still appear later in the document.
func (s *BlockState) startInline(leaf token.Leaf) ([]token.Inline, *inlineCursor) {
	return s.appendInline(nil, leaf)
}

// appendInline is startInline that flattens the tokens into
// dst[:0], growing it only when it is too small, in the manner of
// strconv.AppendInt. A caller that consumes each leaf's tokens before parsing
// the next — a renderer, which is every render path — reuses one buffer for
// the whole document instead of allocating per leaf.
func (s *BlockState) appendInline(dst []token.Inline, leaf token.Leaf) ([]token.Inline, *inlineCursor) {
	if leaf.Literal || leaf.Content == "" {
		return nil, nil
	}
	switch leaf.Node {
	case token.Heading, token.Paragraph, token.CustomLeaf:
		tokens, cursor, _ := s.rules.inline.startContext(dst, leaf.Content, leaf.Context, &s.references)
		return tokens, cursor
	default:
		return nil, nil
	}
}

// parseInlineFinal parses a leaf after sealReferences. It is safe to call from
// parallel workers because the sealed resolver is read-only and InlineRules
// owns a concurrency-safe scratch pool.
func (s *BlockState) parseInlineFinal(leaf token.Leaf) []token.Inline {
	if !s.references.sealed {
		panic("mdflow/parser: parseInlineFinal called before sealReferences")
	}
	tokens, cursor := s.startInline(leaf)
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
func (s *BlockState) total() int { return s.seq }

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

// blockParagraphInterruptor is the built-ins' spelling of ParagraphInterruptor
// on the internal blockLine, which carries a partly consumed tab as well as the
// column; it saves them materialising a Line per probe.
type blockParagraphInterruptor interface {
	interruptsParagraphBlockLine(blockLine) bool
}

func interruptsParagraph(rule any, line blockLine) bool {
	if r, ok := rule.(blockParagraphInterruptor); ok {
		return r.interruptsParagraphBlockLine(line)
	}
	if r, ok := rule.(ParagraphInterruptor); ok {
		return r.InterruptsParagraph(line.public())
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
			if fn(s, rest.public()) {
				return
			}
		}
	}

	// Every container opened below re-asks whether the remainder is a thematic
	// break. Rescanning the remainder each time is quadratic in nesting depth
	// (`- - - … x`), so the suffix that could still qualify is found once per
	// raw line and every earlier remainder is rejected without a scan.
	raw := rest.raw
	tbFrom := thematicSuffixStart(raw)
	for {
		if rest.raw != raw {
			raw = rest.raw
			tbFrom = thematicSuffixStart(raw)
		}
		if rest.blank() || (rest.off >= tbFrom && isThematicBreakBlockLine(rest)) {
			break // a thematic break outranks a list marker
		}
		opened := false
		indent, first := rest.lead()
		for _, cr := range s.rules.containerRules {
			if !mayStart(cr, indent, first) {
				continue
			}
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

	indent, first := rest.lead()
	for _, lr := range s.rules.leafRules {
		if mayStart(lr, indent, first) && openLeafRule(lr, s, rest) {
			return
		}
	}
	openLeafRule(s.rules.paragraph, s, rest)
}

// blockStarter is implemented by built-in block rules whose opening line must
// begin (after at most three columns of indentation) with one of a few bytes.
// Every line is otherwise offered to every rule, and for ordinary prose each
// one rediscovers the indentation just to reject the first letter; the rule
// set is walked on every line, so that repeated rejection dominated the block
// phase for short lines. Extension rules need not implement it: a rule that
// does not is always tried, exactly as before.
type blockStarter interface {
	startsWith(c byte) bool
}

// mayStart reports whether rule r could open on a line with the given
// indentation and first non-blank byte (0 for a blank line). Blank and
// indented lines always reach the rule, which owns their subtler cases.
func mayStart(r any, indent int, first byte) bool {
	if first == 0 || indent >= 4 {
		return true
	}
	if st, ok := r.(blockStarter); ok {
		return st.startsWith(first)
	}
	return true
}

func (blockquoteRule) startsWith(c byte) bool { return c == '>' }
func (listRule) startsWith(c byte) bool {
	return c == '-' || c == '+' || c == '*' || '0' <= c && c <= '9'
}
func (blankRule) startsWith(byte) bool           { return false }
func (setextHeadingRule) startsWith(c byte) bool { return c == '=' || c == '-' }
func (thematicBreakRule) startsWith(c byte) bool { return c == '-' || c == '*' || c == '_' }
func (atxHeadingRule) startsWith(c byte) bool    { return c == '#' }
func (fenceRule) startsWith(c byte) bool         { return c == '`' || c == '~' }
func (indentedCodeRule) startsWith(byte) bool    { return false }
