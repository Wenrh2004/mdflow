package parser

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Wenrh2004/mdflow/token"
)

// token.Inline parsing: a rule registry plus per-parse state.
//
//	InlineRules  trigger byte -> rules, plus a post-processing chain
//	InlineState   per-parse mutable state (source, cursor, item sequence)
//	InlineRule    one rule per construct (escape / code span / link / emphasis / ...)
//
// The algorithm is CommonMark's two passes: a linear scan producing settled
// tokens and pending delimiter runs, then a second pass (emphasisPost) that
// pairs `*`/`_` runs into <em>/<strong> using a delimiter stack.

// InlineRules is stateless and safe to share across goroutines.
//
// statePool recycles InlineState (including its scratch arrays), so a document
// with hundreds of leaf blocks shares a handful of buffers instead of
// allocating per block. triggers is a bitmap of "bytes that could start an
// inline construct", which powers the no-markup fast path.
type InlineRules struct {
	rules     map[byte][]InlineRule
	post      []inlinePost
	triggers  [256]bool
	statePool sync.Pool
}

// Parse turns leaf-block text into a flat inline token stream.
func (p *InlineRules) Parse(src string) []token.Inline {
	out, _ := p.parseContext(src, 0)
	return out
}

// ParseContext is Parse with structural context supplied by the block phase.
// Fragment parsing should normally use Parse, whose context is empty.
func (p *InlineRules) ParseContext(src string, context token.InlineContext) []token.Inline {
	out, _ := p.parseContext(src, context)
	return out
}

// parse is Parse with a deterministic work count for delimiter and bracket
// processing. The count is private: it is a complexity-test seam, not API.
func (p *InlineRules) parse(src string) ([]token.Inline, int) {
	return p.parseContext(src, 0)
}

// ParseWork parses src and returns its inline tokens together with a
// deterministic work count: the number of delimiter- and bracket-processing
// steps the single-pass scan took. The count is machine-independent — it is a
// complexity-regression seam, not a performance measurement. It lets a rule in
// another module hold its own scanner to the standard the built-ins meet:
// feed a growing pathological input through a rule-enabled InlineRules and
// assert the count stays linear in input size. References are treated as
// sealed, so an undefined label resolves to literal text rather than pausing.
func (p *InlineRules) ParseWork(src string) ([]token.Inline, int) {
	return p.parse(src)
}

func (p *InlineRules) parseContext(src string, context token.InlineContext) ([]token.Inline, int) {
	refs := referenceResolver{sealed: true}
	out, cursor, work := p.startContext(src, context, &refs)
	if cursor == nil {
		return out, work
	}
	out, complete := cursor.Resume()
	if !complete {
		panic("mdflow/parser: sealed fragment left an unresolved reference")
	}
	return out, work
}

// startContext scans once until completion or the first reference whose
// definition is not known yet. Only the latter case allocates an InlineCursor;
// ordinary leaves retain the same pooled-state fast path as Parse.
func (p *InlineRules) startContext(
	src string,
	context token.InlineContext,
	refs *referenceResolver,
) ([]token.Inline, *InlineCursor, int) {
	// Fast path: most prose contains no inline trigger byte at all. Emit a
	// single text token aliasing src (zero copy) and skip the item sequence,
	// delimiter pairing and flattening machinery entirely. Escaping remains the
	// renderer's job; only bytes registered by syntax rules force the slow path.
	if !p.hasTrigger(src) {
		src = strings.TrimRight(src, " \t")
		if src == "" {
			return nil, nil, 0
		}
		return []token.Inline{{Node: token.Text, Text: src}}, nil, 0
	}

	s, _ := p.statePool.Get().(*InlineState)
	if s == nil {
		s = &InlineState{parser: p}
	}
	s.src, s.pos, s.context, s.refs, s.items, s.text = src, 0, context, refs, s.items[:0], s.text[:0]
	s.head, s.tail = noInlineIndex, noInlineIndex
	s.delimiters = s.delimiters[:0]
	s.delimiterHead, s.delimiterTail = noInlineIndex, noInlineIndex
	s.brackets = s.brackets[:0]
	s.bracketTop, s.inactiveLinks = noInlineIndex, noInlineIndex
	s.imageBangAt = noInlineIndex
	s.bareLinkDest.ready = false
	s.autolink.reset()
	s.backticks.reset()
	s.imageDepth, s.work = 0, 0
	s.waiting = false
	s.sealLocal = false
	s.pending = pendingReference{}
	s.resetMemos()

	if !runInlineState(s) {
		return nil, &InlineCursor{state: s}, s.work
	}
	out, work := finishInlineState(s)
	return out, nil, work
}

func runInlineState(s *InlineState) bool {
	if s.waiting && !s.resumePendingReference() {
		return false
	}
	for s.pos < len(s.src) {
		matched := false
		for _, r := range s.parser.rules[s.src[s.pos]] {
			if r.Match(s) {
				matched = true
				break
			}
		}
		if s.waiting {
			return false
		}
		if !matched {
			s.AddByte(s.src[s.pos])
			s.pos++
		}
	}
	return true
}

func finishInlineState(s *InlineState) ([]token.Inline, int) {
	s.trimTrailingWhitespace()
	s.flush()
	for _, pp := range s.parser.post {
		s.work += pp.process(s)
	}
	out := flattenItems(s)
	work := s.work
	// Safe to recycle: `out`'s tokens and their strings do not alias s.items or
	// s.text (text was copied out by flush). A recursive call during the scan
	// took a different instance from the pool.
	releaseInlineState(s)
	return out, work
}

func releaseInlineState(s *InlineState) {
	parser := s.parser
	s.src = ""
	s.context = 0
	s.refs = nil
	s.waiting = false
	s.sealLocal = false
	s.pending = pendingReference{}
	// The arena is pooled, but its finished tokens may point into a very large
	// leaf source. Clear the used entries before retaining the backing array so
	// a resolved reference suffix does not stay live solely through the pool.
	clear(s.items)
	s.items = s.items[:0]
	s.resetMemos()
	parser.statePool.Put(s)
}

// InlineCursor is a single-pass inline scan paused at an unresolved reference.
// Resume performs one cached-key lookup and continues in place when a matching
// definition appears or the document's definitions become sealed.
type InlineCursor struct {
	state *InlineState
}

// Resume advances the cursor. complete is false only while the same normalised
// reference key remains undefined and the document is still open.
func (c *InlineCursor) Resume() (tokens []token.Inline, complete bool) {
	if c == nil || c.state == nil {
		return nil, true
	}
	if !runInlineState(c.state) {
		return nil, false
	}
	tokens, _ = finishInlineState(c.state)
	c.state = nil
	return tokens, true
}

// Release abandons a paused cursor and returns its scratch to the parser pool.
// It is safe to call after completion and more than once.
func (c *InlineCursor) Release() {
	if c == nil || c.state == nil {
		return
	}
	releaseInlineState(c.state)
	c.state = nil
}

// PendingLabel returns the normalised reference label this cursor is paused on,
// and whether it is paused at all. A streaming caller uses it to report what
// the committed stream is currently withholding output behind.
func (c *InlineCursor) PendingLabel() (label string, ok bool) {
	if c == nil || c.state == nil || !c.state.waiting {
		return "", false
	}
	return c.state.pending.key, true
}

// ForceLiteral resumes the cursor as if the document were already sealed: the
// pending reference — and any further undefined reference in the same leaf —
// becomes literal text, and the scan runs to completion. It does not seal the
// shared resolver, so forward definitions elsewhere in the document keep
// resolving. It is the streaming seal-undefined policy's resolution step, and
// like Resume it returns complete=true once finished.
func (c *InlineCursor) ForceLiteral() (tokens []token.Inline, complete bool) {
	if c == nil || c.state == nil {
		return nil, true
	}
	c.state.sealLocal = true
	return c.Resume()
}

// CloneFor snapshots a paused cursor against a cloned BlockState. It is used
// by Stream.Provisional so speculative definitions and memo frontiers never
// mutate the live incremental session.
func (c *InlineCursor) CloneFor(block *BlockState) *InlineCursor {
	if c == nil || c.state == nil {
		return nil
	}
	source := c.state
	cloned := *source
	cloned.items = append([]inlineItem(nil), source.items...)
	cloned.delimiters = append([]delimiter(nil), source.delimiters...)
	cloned.brackets = append([]bracket(nil), source.brackets...)
	cloned.text = append([]byte(nil), source.text...)
	cloned.bareLinkDest.parenClose = append([]int(nil), source.bareLinkDest.parenClose...)
	cloned.bareLinkDest.end = append([]int(nil), source.bareLinkDest.end...)
	cloned.bareLinkDest.stack = append([]int(nil), source.bareLinkDest.stack...)
	cloned.backticks = source.backticks.clone()
	cloned.memos = append([]inlineMemoSlot(nil), source.memos...)
	for i := range cloned.memos {
		cloned.memos[i].value = cloned.memos[i].value.CloneInlineMemo()
	}
	if block == nil {
		cloned.refs = nil
	} else {
		cloned.refs = &block.references
	}
	return &InlineCursor{state: &cloned}
}

// hasTrigger reports whether src contains any byte that could start an inline
// construct.
func (p *InlineRules) hasTrigger(src string) bool {
	for i := 0; i < len(src); i++ {
		if p.triggers[src[i]] {
			return true
		}
	}
	return false
}

// InlineMemo is mutable rule scratch scoped to one inline parse.
//
// CloneInlineMemo must return a deeply independent copy: Stream.Provisional
// resumes a cloned inline cursor, and any slices, maps, or pointers shared with
// the live cursor would let a tentative parse corrupt later committed output.
// Requiring this method in Memo's initializer type makes clone support a
// compile-time part of every memo implementation rather than an optional
// runtime convention.
type InlineMemo interface {
	CloneInlineMemo() InlineMemo
}

// InlineState is one inline parse's mutable state.
type InlineState struct {
	src           string
	pos           int
	parser        *InlineRules
	items         []inlineItem // append-only arena addressed by stable integer indices
	head          int
	tail          int
	delimiters    []delimiter // append-only delimiter-stack arena
	delimiterHead int
	delimiterTail int
	brackets      []bracket
	bracketTop    int
	inactiveLinks int
	imageBangAt   int
	bareLinkDest  bareLinkDestinationCache
	autolink      autolinkScanCache
	backticks     backtickScanCache
	imageDepth    int
	work          int
	text          []byte // text accumulator, reused across leaves via statePool
	context       token.InlineContext
	memos         []inlineMemoSlot
	refs          *referenceResolver
	pending       pendingReference
	waiting       bool
	// sealLocal forces an unresolved reference in this one parse to fall back to
	// literal text, exactly as a sealed document would, without touching the
	// shared resolver. It is how the streaming seal-undefined policy resolves a
	// blocked cursor early while forward definitions elsewhere still work.
	sealLocal bool
}

type inlineMemoSlot struct {
	value InlineMemo
	key   token.Tag
}

const noInlineIndex = -1

// Src returns the string being parsed.
func (s *InlineState) Src() string { return s.src }

// Pos returns the current scan offset.
func (s *InlineState) Pos() int { return s.pos }

// Context reports structural facts supplied by the block phase for this leaf.
// It is empty for standalone fragment parsing and recursive extension parses.
func (s *InlineState) Context() token.InlineContext { return s.context }

// Memo returns mutable scratch scoped to this inline parse. Rules use a private
// tag as the key and provide an initializer for the first access; passing a nil
// initializer reads without creating a slot. The value must implement
// InlineMemo, whose deep-clone contract keeps provisional and live cursors
// isolated. Values are discarded before the state returns to the pool, so
// neither source data nor rule state crosses a parse boundary.
func (s *InlineState) Memo(key token.Tag, init func() InlineMemo) InlineMemo {
	if key == token.NoTag {
		panic("mdflow/parser: inline memo key must be a private tag")
	}
	for i := range s.memos {
		if s.memos[i].key == key {
			return s.memos[i].value
		}
	}
	if init == nil {
		return nil
	}
	value := init()
	s.memos = append(s.memos, inlineMemoSlot{key: key, value: value})
	return value
}

func (s *InlineState) resetMemos() {
	for i := range s.memos {
		s.memos[i] = inlineMemoSlot{}
	}
	s.memos = s.memos[:0]
}

// Advance moves the scan offset forward by n bytes.
func (s *InlineState) Advance(n int) { s.pos += n }

// AddWork adds n to this parse's deterministic work count. A rule whose match
// scans more than a constant number of bytes should report that span, so its
// cost becomes visible to the complexity-regression seam ([InlineRules.ParseWork]).
// This is how an extension scanner is held to the same linearity standard the
// built-ins meet: n is a step or byte count, never wall-clock time.
func (s *InlineState) AddWork(n int) { s.work += n }

// AddText folds literal text into the pending text token.
func (s *InlineState) AddText(t string) { s.text = append(s.text, t...) }

// AddByte folds one literal byte into the pending text token.
func (s *InlineState) AddByte(b byte) { s.text = append(s.text, b) }

// Emit settles the pending text, then appends one finished token.
func (s *InlineState) Emit(tok token.Inline) {
	s.flush()
	s.observeSettledToken(tok)
	s.appendItem(inlineItem{tok: tok})
}

// EmitAll appends a run of finished tokens (e.g. a recursively parsed link text).
func (s *InlineState) EmitAll(toks []token.Inline) {
	s.flush()
	for _, t := range toks {
		s.observeSettledToken(t)
		s.appendItem(inlineItem{tok: t})
	}
}

// Parse recursively parses a substring with the same rule set.
func (s *InlineState) Parse(sub string) []token.Inline { return s.parser.Parse(sub) }

func (s *InlineState) flush() {
	if len(s.text) > 0 {
		s.appendItem(inlineItem{tok: token.Inline{Node: token.Text, Text: string(s.text)}})
		s.text = s.text[:0]
	}
}

// trimTrailingWhitespace removes source whitespace that is insignificant at a
// paragraph boundary. It returns the number of consecutive ASCII spaces at the
// very end, which is what distinguishes a hard break from a soft one before a
// line ending. Tabs are trimmed but never count toward the two-space spelling.
func (s *InlineState) trimTrailingWhitespace() int {
	spaces := 0
	for i := len(s.text) - 1; i >= 0 && s.text[i] == ' '; i-- {
		spaces++
	}
	for len(s.text) > 0 {
		last := s.text[len(s.text)-1]
		if last != ' ' && last != '\t' {
			break
		}
		s.text = s.text[:len(s.text)-1]
	}
	return spaces
}

// addDelimiter records a pending delimiter run for the pairing pass.
//
// The literal fallback string is deliberately not materialised here: most runs
// are either consumed by a pair or are short, so only genuinely leftover runs
// pay for a strings.Repeat, in flattenItems.
func (s *InlineState) addDelimiter(ch byte, n int, canOpen, canClose bool) {
	s.flush()
	item := s.appendItem(inlineItem{delimiterChar: ch, delimiterLen: n})
	index := len(s.delimiters)
	s.delimiters = append(s.delimiters, delimiter{
		item: item, prev: s.delimiterTail, next: noInlineIndex,
		char: ch, count: n, original: n, canOpen: canOpen, canClose: canClose,
	})
	if s.delimiterTail == noInlineIndex {
		s.delimiterHead = index
	} else {
		s.delimiters[s.delimiterTail].next = index
	}
	s.delimiterTail = index
}

// inlineItem is one stable-index node in an append-only arena. prev/next carry
// presentation order, allowing emphasis nodes to be inserted without moving
// any item or repeatedly rebuilding the token slice.
type inlineItem struct {
	tok           token.Inline
	prev          int
	next          int
	delimiterLen  int
	delimiterChar byte
	removed       bool
}

// delimiter is one stable entry in the emphasis delimiter stack. original is
// retained after partial consumption because CommonMark's rule of three is
// defined in terms of the runs the delimiters came from.
type delimiter struct {
	item              int
	prev, next        int
	count, original   int
	char              byte
	canOpen, canClose bool
	removed           bool
}

// bracket is a potential link or image opener. inactiveLinks is an ordinal
// high-water mark for ordinary links only: image openers stay usable when a
// link appears in their description.
type bracket struct {
	item            int
	delimiterBottom int
	prev            int
	sourceStart     int
	image           bool
}

type pendingReference struct {
	key          string
	bracketIndex int
	after        int
}

func (s *InlineState) observeSettledToken(tok token.Inline) {
	switch tok.Node {
	case token.Image:
		if tok.Close {
			s.imageDepth--
		} else {
			s.imageDepth++
		}
	case token.Link:
		if !tok.Close && s.imageDepth == 0 && s.bracketTop != noInlineIndex {
			s.inactiveLinks = max(s.inactiveLinks, s.bracketTop)
		}
	}
}

func (s *InlineState) appendItem(item inlineItem) int {
	index := len(s.items)
	item.prev, item.next = s.tail, noInlineIndex
	s.items = append(s.items, item)
	if s.tail == noInlineIndex {
		s.head = index
	} else {
		s.items[s.tail].next = index
	}
	s.tail = index
	return index
}

func (s *InlineState) insertAfter(at int, tok token.Inline) int {
	next := s.items[at].next
	index := len(s.items)
	s.items = append(s.items, inlineItem{tok: tok, prev: at, next: next})
	s.items[at].next = index
	if next == noInlineIndex {
		s.tail = index
	} else {
		s.items[next].prev = index
	}
	return index
}

func (s *InlineState) insertBefore(at int, tok token.Inline) int {
	prev := s.items[at].prev
	index := len(s.items)
	s.items = append(s.items, inlineItem{tok: tok, prev: prev, next: at})
	s.items[at].prev = index
	if prev == noInlineIndex {
		s.head = index
	} else {
		s.items[prev].next = index
	}
	return index
}

func (s *InlineState) removeItem(index int) {
	item := &s.items[index]
	if item.removed {
		return
	}
	if item.prev == noInlineIndex {
		s.head = item.next
	} else {
		s.items[item.prev].next = item.next
	}
	if item.next == noInlineIndex {
		s.tail = item.prev
	} else {
		s.items[item.next].prev = item.prev
	}
	item.removed = true
}

func (s *InlineState) addBracket(item int, image bool) {
	index := len(s.brackets)
	s.brackets = append(s.brackets, bracket{
		item: item, delimiterBottom: s.delimiterTail, prev: s.bracketTop,
		sourceStart: s.pos + 1, image: image,
	})
	s.bracketTop = index
	s.work++
}

// ---- default inline rules ----

// escapeRule handles backslash escapes: `\*` is a literal `*` that takes no
// part in emphasis pairing.
type escapeRule struct{}

func (escapeRule) Name() string     { return "escape" }
func (escapeRule) Triggers() []byte { return []byte{'\\'} }
func (escapeRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	if i+1 < len(src) && isASCIIPunct(src[i+1]) {
		s.AddByte(src[i+1])
		s.pos += 2
		return true
	}
	return false
}

// entityRule decodes a semicolon-terminated HTML character reference into
// literal text. Adding the expansion to the text accumulator is important:
// syntax characters produced by a reference are text, not Markdown to scan a
// second time (for example, &#42; does not open emphasis).
type entityRule struct{}

func (entityRule) Name() string     { return "entity" }
func (entityRule) Triggers() []byte { return []byte{'&'} }
func (entityRule) Match(s *InlineState) bool {
	value, n, ok := scanCharacterReference(s.src[s.pos:])
	if !ok {
		return false
	}
	s.AddText(value)
	s.pos += n
	return true
}

// hardBreakRule turns a backslash at end of line into a <br />. The block layer
// already rewrote the "two trailing spaces" spelling into this one, so both
// forms funnel through a single rule.
type hardBreakRule struct{}

func (hardBreakRule) Name() string     { return "hard_break" }
func (hardBreakRule) Triggers() []byte { return []byte{'\\'} }
func (hardBreakRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	if i+1 < len(src) && src[i+1] == '\n' {
		s.Emit(token.Inline{Node: token.HardBreak})
		s.pos += 2
		return true
	}
	return false
}

// softBreakRule preserves an ordinary line ending as its own inline event.
type softBreakRule struct{}

func (softBreakRule) Name() string     { return "soft_break" }
func (softBreakRule) Triggers() []byte { return []byte{'\n'} }
func (softBreakRule) Match(s *InlineState) bool {
	node := token.SoftBreak
	if s.trimTrailingWhitespace() >= 2 {
		node = token.HardBreak
	}
	s.Emit(token.Inline{Node: node})
	s.pos++
	return true
}

// codeSpanRule handles inline code: equal-length backtick runs, literal inside.
type codeSpanRule struct{}

func (codeSpanRule) Name() string     { return "code_span" }
func (codeSpanRule) Triggers() []byte { return []byte{'`'} }
func (codeSpanRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	n := runLength(src, i, '`')
	if end, ok := s.findBacktickClose(i+n, n); ok {
		s.Emit(token.Inline{Node: token.CodeSpan, Text: normalizeCodeSpan(src[i+n : end])})
		s.pos = end + n
	} else {
		s.AddText(src[i : i+n]) // unmatched run is literal
		s.pos += n
	}
	return true
}

// linkRule records a potential link or image opener. imageRule leaves a pending
// `!` marker at exactly this source offset, but rules prepended for `[` still get
// their normal first chance before this default opener is reached.
type linkRule struct{}

func (linkRule) Name() string     { return "link" }
func (linkRule) Triggers() []byte { return []byte{'['} }
func (linkRule) Match(s *InlineState) bool {
	image := s.imageBangAt == s.pos
	if image {
		// imageRule appended this byte immediately before advancing here. Remove
		// only that pending marker; earlier prose remains in source order.
		s.text = s.text[:len(s.text)-1]
	}
	s.flush()
	marker := "["
	if image {
		marker = "!["
	}
	item := s.appendItem(inlineItem{tok: token.Inline{Node: token.Text, Text: marker}})
	s.addBracket(item, image)
	s.pos++
	return true
}

// linkCloseRule closes the most recent bracket opener and, when an inline-link
// tail follows, turns the two marker items into flat Link open/close tokens.
type linkCloseRule struct{}

func (linkCloseRule) Name() string     { return "link_close" }
func (linkCloseRule) Triggers() []byte { return []byte{']'} }
func (linkCloseRule) Match(s *InlineState) bool {
	s.work++
	if s.bracketTop == noInlineIndex {
		s.AddByte(']')
		s.pos++
		return true
	}

	bracketIndex := s.bracketTop
	opener := s.brackets[bracketIndex]
	if !opener.image && bracketIndex <= s.inactiveLinks {
		s.failBracket(bracketIndex)
		return true
	}

	dest, title, next, ok := scanInlineLinkTail(s, s.pos+1)
	if ok {
		s.completeBracket(bracketIndex, dest, title, next)
		return true
	}

	pending, ok := scanPendingReference(s, bracketIndex)
	if !ok {
		s.failBracket(bracketIndex)
		return true
	}
	if definition, found := s.refs.lookupNormalized(pending.key); found {
		s.completeBracket(bracketIndex, definition.destination, definition.title, pending.after)
		return true
	}
	if s.refs != nil && !s.refs.sealed && !s.sealLocal {
		s.pending = pending
		s.waiting = true
		return true
	}
	s.failBracket(bracketIndex)
	return true
}

func scanPendingReference(s *InlineState, bracketIndex int) (pendingReference, bool) {
	opener := s.brackets[bracketIndex]
	content := s.src[opener.sourceStart:s.pos]
	next := s.pos + 1

	if next+1 < len(s.src) && s.src[next] == '[' && s.src[next+1] == ']' {
		if key, ok := normalizeReferenceCandidate(content); ok {
			return pendingReference{
				key: key, bracketIndex: bracketIndex, after: next + 2,
			}, true
		}
		return pendingReference{}, false
	}

	if next < len(s.src) && s.src[next] == '[' {
		if label, after, ok := scanReferenceDefinitionLabel(s.src, next); ok {
			return pendingReference{
				key: normalizeReferenceLabel(label), bracketIndex: bracketIndex,
				after: after,
			}, true
		}
	}

	key, ok := normalizeReferenceCandidate(content)
	if !ok {
		return pendingReference{}, false
	}
	return pendingReference{
		key: key, bracketIndex: bracketIndex, after: s.pos + 1,
	}, true
}

func (s *InlineState) resumePendingReference() bool {
	if !s.waiting {
		return true
	}
	pending := s.pending
	if definition, ok := s.refs.lookupNormalized(pending.key); ok {
		s.waiting = false
		s.pending = pendingReference{}
		s.completeBracket(pending.bracketIndex, definition.destination, definition.title, pending.after)
		return true
	}
	if s.refs != nil && !s.refs.sealed && !s.sealLocal {
		return false
	}
	s.waiting = false
	s.pending = pendingReference{}
	s.failBracket(pending.bracketIndex)
	return true
}

func (s *InlineState) completeBracket(bracketIndex int, dest, title string, after int) {
	opener := s.brackets[bracketIndex]
	s.bracketTop = opener.prev
	s.flush()
	s.work += processEmphasis(s, opener.delimiterBottom)
	node := token.Link
	if opener.image {
		node = token.Image
	}
	s.items[opener.item].tok = token.Inline{Node: node, Dest: dest, Title: title}
	s.appendItem(inlineItem{tok: token.Inline{Node: node, Close: true}})
	if !opener.image {
		s.inactiveLinks = max(s.inactiveLinks, opener.prev)
	}
	s.pos = after
}

func (s *InlineState) failBracket(bracketIndex int) {
	s.bracketTop = s.brackets[bracketIndex].prev
	s.AddByte(']')
	s.pos++
}

// imageRule records only the `!` half of `![`. The next scanner step remains a
// normal `[` dispatch, preserving PrependInlineRule precedence. If no extension
// claims it, linkRule combines the pending marker into an image bracket.
type imageRule struct{}

func (imageRule) Name() string     { return "image" }
func (imageRule) Triggers() []byte { return []byte{'!'} }
func (imageRule) Match(s *InlineState) bool {
	if s.pos+1 >= len(s.src) || s.src[s.pos+1] != '[' {
		return false
	}
	s.AddByte('!')
	s.imageBangAt = s.pos + 1
	s.pos++
	return true
}

// autolinkRule handles `<https://example.com>` and `<user@example.com>`.
type autolinkRule struct{}

func (autolinkRule) Name() string     { return "autolink" }
func (autolinkRule) Triggers() []byte { return []byte{'<'} }
func (autolinkRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	end, ok := s.nextAutolinkClose(i + 1)
	if !ok {
		return false
	}
	body := src[i+1 : end]
	if body == "" || s.autolinkBodyHasForbiddenByte(i+1, end) {
		return false
	}
	dest := body
	switch {
	case hasURIScheme(body):
	case isEmailLike(body):
		dest = "mailto:" + body
	default:
		return false
	}
	s.Emit(token.Inline{Node: token.Link, Dest: dest})
	s.Emit(token.Inline{Node: token.Text, Text: body})
	s.Emit(token.Inline{Node: token.Link, Close: true})
	s.pos = end + 1
	return true
}

// autolinkScanCache remembers the first unconsumed closing angle bracket.
// Inline candidates are visited left-to-right, so every search either reuses
// this close or advances beyond it. noClose proves the remaining suffix once.
type autolinkScanCache struct {
	close   int
	noClose bool
}

func (c *autolinkScanCache) reset() {
	c.close = noInlineIndex
	c.noClose = false
}

func (s *InlineState) nextAutolinkClose(from int) (int, bool) {
	cache := &s.autolink
	if cache.close >= from {
		return cache.close, true
	}
	if cache.noClose {
		return 0, false
	}
	relative := strings.IndexByte(s.src[from:], '>')
	if relative < 0 {
		s.work += len(s.src) - from
		cache.noClose = true
		return 0, false
	}
	s.work += relative + 1
	cache.close = from + relative
	return cache.close, true
}

func (s *InlineState) autolinkBodyHasForbiddenByte(from, end int) bool {
	for i := from; i < end; i++ {
		s.work++
		switch s.src[i] {
		case ' ', '\t', '\n', '<':
			return true
		}
	}
	return false
}

// emphasisRule records `*`/`_` runs as pending delimiters for emphasisPost.
type emphasisRule struct{}

func (emphasisRule) Name() string     { return "emphasis" }
func (emphasisRule) Triggers() []byte { return []byte{'*', '_'} }
func (emphasisRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	c := src[i]
	n := runLength(src, i, c)
	canOpen, canClose := scanEmphasisFlanking(src, i, n, c)
	s.addDelimiter(c, n, canOpen, canClose)
	s.pos += n
	return true
}

type emphasisPost struct{}

func (emphasisPost) process(s *InlineState) int { return processEmphasis(s, noInlineIndex) }

// ---- lexical helpers (pure) ----

func runLength(s string, pos int, c byte) int {
	n := 0
	for pos+n < len(s) && s[pos+n] == c {
		n++
	}
	return n
}

type backtickRunPositions struct {
	starts []int
	next   int
}

// backtickScanCache is a lazy forward index of backtick runs. A failed opener
// may have to inspect the unresolved suffix, but later openers reuse the runs
// learned by that proof instead of rescanning the suffix. Per-length cursors
// discard cached positions monotonically, keeping lookup work linear too.
type backtickScanCache struct {
	byLength map[int]backtickRunPositions
	frontier int
}

func (c *backtickScanCache) reset() {
	c.frontier = 0
	clear(c.byLength)
}

func (c backtickScanCache) clone() backtickScanCache {
	cloned := backtickScanCache{frontier: c.frontier}
	if len(c.byLength) == 0 {
		return cloned
	}
	cloned.byLength = make(map[int]backtickRunPositions, len(c.byLength))
	for length, positions := range c.byLength {
		positions.starts = append([]int(nil), positions.starts...)
		cloned.byLength[length] = positions
	}
	return cloned
}

func (s *InlineState) findBacktickClose(from, length int) (int, bool) {
	cache := &s.backticks
	if positions, ok := cache.byLength[length]; ok {
		for positions.next < len(positions.starts) && positions.starts[positions.next] < from {
			positions.next++
			s.work++
		}
		cache.byLength[length] = positions
		if positions.next < len(positions.starts) {
			return positions.starts[positions.next], true
		}
	}

	scan := max(cache.frontier, from)
	for scan < len(s.src) {
		relative := strings.IndexByte(s.src[scan:], '`')
		if relative < 0 {
			s.work += len(s.src) - scan
			cache.frontier = len(s.src)
			return 0, false
		}
		start := scan + relative
		run := runLength(s.src, start, '`')
		s.work += relative + run
		if cache.byLength == nil {
			cache.byLength = make(map[int]backtickRunPositions)
		}
		positions := cache.byLength[run]
		positions.starts = append(positions.starts, start)
		cache.byLength[run] = positions
		cache.frontier = start + run
		if run == length {
			return start, true
		}
		scan = cache.frontier
	}
	return 0, false
}

func normalizeCodeSpan(s string) string {
	if strings.IndexByte(s, '\n') >= 0 {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	if len(s) >= 2 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.TrimSpace(s) != "" {
		s = s[1 : len(s)-1]
	}
	return s
}

func hasURIScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 2 || colon > 32 || !isASCIIAlpha(s[0]) {
		return false
	}
	for i := 1; i < colon; i++ {
		c := s[i]
		if !isASCIIAlphanumeric(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	for i := colon + 1; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c == 0x7f || c == '<' || c == '>' {
			return false
		}
	}
	return true
}

func isEmailLike(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 || strings.IndexByte(s[at+1:], '@') >= 0 {
		return false
	}
	for i := 0; i < at; i++ {
		if !isEmailLocalByte(s[i]) {
			return false
		}
	}
	for start := at + 1; start < len(s); {
		end := strings.IndexByte(s[start:], '.')
		if end < 0 {
			end = len(s)
		} else {
			end += start
		}
		if !isEmailDomainLabel(s[start:end]) {
			return false
		}
		if end == len(s) {
			return true
		}
		start = end + 1
	}
	return false
}

func isASCIIAlpha(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
}

func isEmailLocalByte(c byte) bool {
	if isASCIIAlphanumeric(c) {
		return true
	}
	switch c {
	case '.', '!', '#', '$', '%', '&', '\'', '*', '+', '/', '=', '?', '^', '_', '`', '{', '|', '}', '~', '-':
		return true
	default:
		return false
	}
}

func isEmailDomainLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 || !isASCIIAlphanumeric(s[0]) || !isASCIIAlphanumeric(s[len(s)-1]) {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		if !isASCIIAlphanumeric(s[i]) && s[i] != '-' {
			return false
		}
	}
	return true
}

// bareLinkDestinationCache gives every possible unbracketed destination start
// its terminal byte (or noInlineIndex when an opening parenthesis is
// unmatched). It is built lazily at the first inline-link tail, then reused by
// later failed candidates so nested `](` suffixes cannot be rescanned.
type bareLinkDestinationCache struct {
	ready      bool
	parenClose []int
	end        []int
	stack      []int
}

func (c *bareLinkDestinationCache) prepare(src string, work *int) {
	if c.ready {
		return
	}
	n := len(src)
	if cap(c.parenClose) < n {
		c.parenClose = make([]int, n)
	} else {
		c.parenClose = c.parenClose[:n]
	}
	if cap(c.end) < n+1 {
		c.end = make([]int, n+1)
	} else {
		c.end = c.end[:n+1]
	}
	for i := range c.parenClose {
		c.parenClose[i] = noInlineIndex
	}

	c.stack = c.stack[:0]
	for i := 0; i < n; i++ {
		*work++
		ch := src[i]
		if ch == '\\' && i+1 < n && isASCIIPunct(src[i+1]) {
			i++
			*work++
			continue
		}
		if ch <= 0x20 || ch == 0x7f {
			c.stack = c.stack[:0]
			continue
		}
		switch ch {
		case '(':
			c.stack = append(c.stack, i)
		case ')':
			if len(c.stack) == 0 {
				continue
			}
			last := len(c.stack) - 1
			c.parenClose[c.stack[last]] = i
			c.stack = c.stack[:last]
		}
	}

	c.end[n] = n
	for i := n - 1; i >= 0; i-- {
		*work++
		ch := src[i]
		switch {
		case ch == '\\' && i+1 < n && isASCIIPunct(src[i+1]):
			c.end[i] = c.end[i+2]
		case ch <= 0x20 || ch == 0x7f || ch == ')':
			c.end[i] = i
		case ch == '(':
			close := c.parenClose[i]
			if close == noInlineIndex {
				c.end[i] = noInlineIndex
			} else {
				c.end[i] = c.end[close+1]
			}
		default:
			c.end[i] = c.end[i+1]
		}
	}
	c.ready = true
}

// scanInlineLinkTail parses the `(destination "title")` following a closed
// link-text bracket. It never scans link text itself: the bracket stack has
// already consumed that prefix once.
func scanInlineLinkTail(s *InlineState, start int) (dest, title string, next int, ok bool) {
	src, work := s.src, &s.work
	if start >= len(src) || src[start] != '(' {
		return "", "", 0, false
	}
	*work++
	pos := scanInlineLinkSpace(src, start+1, work)

	var destOK bool
	dest, pos, destOK = scanInlineLinkDestination(s, pos)
	if !destOK {
		return "", "", 0, false
	}

	beforeSeparator := pos
	pos = scanInlineLinkSpace(src, pos, work)
	if pos > beforeSeparator && pos < len(src) && isLinkTitleOpener(src[pos]) {
		var titleOK bool
		title, pos, titleOK = scanInlineLinkTitle(src, pos, work)
		if !titleOK {
			return "", "", 0, false
		}
		pos = scanInlineLinkSpace(src, pos, work)
	}

	if pos >= len(src) || src[pos] != ')' {
		return "", "", 0, false
	}
	*work++
	return unescapeSource(dest), unescapeSource(title), pos + 1, true
}

// scanInlineLinkSpace consumes spaces/tabs and at most one line ending. A
// second line ending is left for the caller to reject as non-syntax.
func scanInlineLinkSpace(src string, start int, work *int) int {
	pos := start
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
		*work++
	}
	if width := lineEndingWidth(src, pos); width > 0 {
		pos += width
		*work += width
		for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
			pos++
			*work++
		}
	}
	return pos
}

func lineEndingWidth(src string, pos int) int {
	if pos >= len(src) {
		return 0
	}
	if src[pos] == '\n' {
		return 1
	}
	if src[pos] != '\r' {
		return 0
	}
	if pos+1 < len(src) && src[pos+1] == '\n' {
		return 2
	}
	return 1
}

func scanInlineLinkDestination(s *InlineState, start int) (string, int, bool) {
	src, work := s.src, &s.work
	if start < len(src) && src[start] == '<' {
		pos := start + 1
		*work++
		for pos < len(src) {
			c := src[pos]
			*work++
			switch {
			case c == '\n' || c == '\r' || c == '<':
				return "", 0, false
			case c == '\\' && pos+1 < len(src) && isASCIIPunct(src[pos+1]):
				pos += 2
				*work++
			case c == '>':
				return src[start+1 : pos], pos + 1, true
			default:
				pos++
			}
		}
		return "", 0, false
	}

	s.bareLinkDest.prepare(src, work)
	pos := s.bareLinkDest.end[start]
	*work++
	if pos == noInlineIndex {
		return "", 0, false
	}
	return src[start:pos], pos, true
}

func isLinkTitleOpener(c byte) bool { return c == '"' || c == '\'' || c == '(' }

func scanInlineLinkTitle(src string, start int, work *int) (string, int, bool) {
	open := src[start]
	close := open
	if open == '(' {
		close = ')'
	}
	pos := start + 1
	*work++
	afterLineEnding := false
	for pos < len(src) {
		c := src[pos]
		*work++
		if c == '\\' && pos+1 < len(src) && isASCIIPunct(src[pos+1]) {
			pos += 2
			*work++
			afterLineEnding = false
			continue
		}
		if c == close {
			return src[start+1 : pos], pos + 1, true
		}
		if open == '(' && c == '(' {
			return "", 0, false
		}
		if width := lineEndingWidth(src, pos); width > 0 {
			if afterLineEnding {
				return "", 0, false
			}
			pos += width
			*work += width - 1
			afterLineEnding = true
			continue
		}
		if c != ' ' && c != '\t' {
			afterLineEnding = false
		}
		pos++
	}
	return "", 0, false
}

// scanEmphasisFlanking implements CommonMark's left/right-flanking tests. Go's
// Unicode tables supply the required Unicode whitespace, punctuation and
// symbol categories; ASCII punctuation is included in those same predicates.
func scanEmphasisFlanking(src string, start, length int, delimiter byte) (canOpen, canClose bool) {
	end := start + length
	beforeSpace, beforePunct := true, false
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(src[:start])
		beforeSpace = isUnicodeWhitespace(r)
		beforePunct = isUnicodePunctuation(r)
	}
	afterSpace, afterPunct := true, false
	if end < len(src) {
		r, _ := utf8.DecodeRuneInString(src[end:])
		afterSpace = isUnicodeWhitespace(r)
		afterPunct = isUnicodePunctuation(r)
	}

	leftFlanking := !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	rightFlanking := !beforeSpace && (!beforePunct || afterSpace || afterPunct)
	if delimiter == '_' {
		return leftFlanking && (!rightFlanking || beforePunct),
			rightFlanking && (!leftFlanking || afterPunct)
	}
	return leftFlanking, rightFlanking
}

func isUnicodeWhitespace(r rune) bool {
	return r == '\t' || r == '\n' || r == '\f' || r == '\r' || unicode.Is(unicode.Zs, r)
}

func isUnicodePunctuation(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// processEmphasis is CommonMark's delimiter-stack procedure. Arena indices are
// stable even when inserting the flat open/close tokens, while openersBottom
// prevents repeated failed backward searches from becoming quadratic.
//
// The returned work count includes delimiter visits, opener probes and stack
// removals. It is used only by deterministic complexity tests.
func processEmphasis(s *InlineState, stackBottom int) int {
	current := s.delimiterHead
	if stackBottom != noInlineIndex {
		current = s.delimiters[stackBottom].next
	}
	if current == noInlineIndex {
		return 0
	}

	var openersBottom [2][3][2]int
	for charIndex := range openersBottom {
		for mod := range openersBottom[charIndex] {
			for both := range openersBottom[charIndex][mod] {
				openersBottom[charIndex][mod][both] = stackBottom
			}
		}
	}

	work := 0
	for current != noInlineIndex {
		work++
		closer := &s.delimiters[current]
		if closer.removed || !closer.canClose {
			current = closer.next
			continue
		}

		charIndex := 0
		if closer.char == '_' {
			charIndex = 1
		}
		bothIndex := 0
		if closer.canOpen {
			bothIndex = 1
		}
		bottom := openersBottom[charIndex][closer.original%3][bothIndex]

		openerIndex := closer.prev
		for openerIndex != noInlineIndex && openerIndex > bottom {
			work++
			opener := &s.delimiters[openerIndex]
			if opener.canOpen && opener.char == closer.char && !violatesRuleOfThree(opener, closer) {
				break
			}
			openerIndex = opener.prev
		}

		if openerIndex == noInlineIndex || openerIndex <= bottom {
			openersBottom[charIndex][closer.original%3][bothIndex] = closer.prev
			next := closer.next
			if !closer.canOpen {
				s.removeDelimiter(current)
				work++
			}
			current = next
			continue
		}

		opener := &s.delimiters[openerIndex]
		use := 1
		node := token.Emph
		if opener.count >= 2 && closer.count >= 2 {
			use = 2
			node = token.Strong
		}

		s.insertAfter(opener.item, token.Inline{Node: node})
		s.insertBefore(closer.item, token.Inline{Node: node, Close: true})
		opener.count -= use
		closer.count -= use
		s.items[opener.item].delimiterLen = opener.count
		s.items[closer.item].delimiterLen = closer.count

		for between := opener.next; between != current; {
			next := s.delimiters[between].next
			s.removeDelimiter(between)
			work++
			between = next
		}

		if opener.count == 0 {
			s.removeItem(opener.item)
			s.removeDelimiter(openerIndex)
			work++
		}
		if closer.count == 0 {
			next := closer.next
			s.removeItem(closer.item)
			s.removeDelimiter(current)
			work++
			current = next
		}
		// A partially consumed closer stays current: the remaining delimiter
		// may close another, outer emphasis span.
	}

	// Delimiters in a completed link's text must not match delimiters outside
	// that link. Their marker items remain literal; only stack metadata goes.
	for s.delimiterTail != stackBottom && s.delimiterTail != noInlineIndex {
		s.removeDelimiter(s.delimiterTail)
		work++
	}

	return work
}

func violatesRuleOfThree(opener, closer *delimiter) bool {
	if !closer.canOpen && !opener.canClose {
		return false
	}
	return (opener.original+closer.original)%3 == 0 &&
		(opener.original%3 != 0 || closer.original%3 != 0)
}

func (s *InlineState) removeDelimiter(index int) {
	d := &s.delimiters[index]
	if d.removed {
		return
	}
	if d.prev == noInlineIndex {
		s.delimiterHead = d.next
	} else {
		s.delimiters[d.prev].next = d.next
	}
	if d.next == noInlineIndex {
		s.delimiterTail = d.prev
	} else {
		s.delimiters[d.next].prev = d.prev
	}
	d.removed = true
}

// flattenItems traverses presentation order, not arena allocation order. Any
// unconsumed delimiter runs materialise as literal text only at this boundary.
func flattenItems(s *InlineState) []token.Inline {
	out := make([]token.Inline, 0, len(s.items))
	for index := s.head; index != noInlineIndex; index = s.items[index].next {
		item := &s.items[index]
		if item.removed {
			continue
		}
		if item.delimiterChar != 0 {
			if item.delimiterLen > 0 {
				out = append(out, token.Inline{
					Node: token.Text,
					Text: strings.Repeat(string(item.delimiterChar), item.delimiterLen),
				})
			}
			continue
		}
		out = append(out, item.tok)
	}
	return mergeAdjacentText(out)
}

func mergeAdjacentText(in []token.Inline) []token.Inline {
	out := in[:0]
	for i := 0; i < len(in); {
		if in[i].Node != token.Text {
			out = append(out, in[i])
			i++
			continue
		}

		end, size := i+1, len(in[i].Text)
		for end < len(in) && in[end].Node == token.Text {
			size += len(in[end].Text)
			end++
		}
		if end == i+1 {
			out = append(out, in[i])
			i = end
			continue
		}

		var merged strings.Builder
		merged.Grow(size)
		for part := i; part < end; part++ {
			merged.WriteString(in[part].Text)
		}
		tok := in[i]
		tok.Text = merged.String()
		out = append(out, tok)
		i = end
	}
	return out
}

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}
