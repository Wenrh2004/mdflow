package parser

import (
	"strings"
	"sync"

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
	rules     [256][]InlineRule // dense: indexed per input byte on the hot loop
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
	out, cursor, work := p.startContext(nil, src, context, &refs)
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
// definition is not known yet. Only the latter case allocates an inlineCursor;
// ordinary leaves retain the same pooled-state fast path as Parse.
func (p *InlineRules) startContext(
	dst []token.Inline,
	src string,
	context token.InlineContext,
	refs *referenceResolver,
) ([]token.Inline, *inlineCursor, int) {
	// Fast path: most prose contains no inline trigger byte at all. Emit a
	// single text token aliasing src (zero copy) and skip the item sequence,
	// delimiter pairing and flattening machinery entirely. Escaping remains the
	// renderer's job; only bytes registered by syntax rules force the slow path.
	if !p.hasTrigger(src) {
		src = strings.TrimRight(src, " \t")
		if src == "" {
			return nil, nil, 0
		}
		return append(dst[:0], token.Inline{Node: token.Text, Text: src}), nil, 0
	}

	s, _ := p.statePool.Get().(*InlineState)
	if s == nil {
		s = &InlineState{parser: p}
	}
	s.src, s.pos, s.context, s.refs, s.items, s.text = src, 0, context, refs, s.items[:0], s.text[:0]
	s.textFrom = -1
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
		return nil, &inlineCursor{state: s}, s.work
	}
	out, work := finishInlineState(s, dst)
	return out, nil, work
}

func runInlineState(s *InlineState) bool {
	if s.waiting && !s.resumePendingReference() {
		return false
	}
	rules, triggers := &s.parser.rules, &s.parser.triggers
	for s.pos < len(s.src) {
		// Bytes no rule is registered for are literal text. Copy the whole run
		// at once instead of consulting the rule table byte by byte.
		if !triggers[s.src[s.pos]] {
			end := s.pos + 1
			for end < len(s.src) && !triggers[s.src[end]] {
				end++
			}
			s.addSource(s.pos, end)
			s.pos = end
			continue
		}
		matched := false
		for _, r := range rules[s.src[s.pos]] {
			if r.Match(s) {
				matched = true
				break
			}
		}
		if s.waiting {
			return false
		}
		if !matched {
			s.addSource(s.pos, s.pos+1)
			s.pos++
		}
	}
	return true
}

// finishInlineState completes a scan, flattening its tokens into dst[:0] (a
// fresh slice when dst is nil).
func finishInlineState(s *InlineState, dst []token.Inline) ([]token.Inline, int) {
	s.trimTrailingWhitespace()
	s.flush()
	for _, pp := range s.parser.post {
		s.work += pp.process(s)
	}
	out := flattenItems(s, dst)
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

// inlineCursor is a single-pass inline scan paused at an unresolved reference.
// Resume performs one cached-key lookup and continues in place when a matching
// definition appears or the document's definitions become sealed.
type inlineCursor struct {
	state *InlineState
}

// Resume advances the cursor. complete is false only while the same normalised
// reference key remains undefined and the document is still open.
func (c *inlineCursor) Resume() (tokens []token.Inline, complete bool) {
	if c == nil || c.state == nil {
		return nil, true
	}
	if !runInlineState(c.state) {
		return nil, false
	}
	tokens, _ = finishInlineState(c.state, nil)
	c.state = nil
	return tokens, true
}

// Release abandons a paused cursor and returns its scratch to the parser pool.
// It is safe to call after completion and more than once.
func (c *inlineCursor) Release() {
	if c == nil || c.state == nil {
		return
	}
	releaseInlineState(c.state)
	c.state = nil
}

// PendingLabel returns the normalised reference label this cursor is paused on,
// and whether it is paused at all. A streaming caller uses it to report what
// the committed stream is currently withholding output behind.
func (c *inlineCursor) PendingLabel() (label string, ok bool) {
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
func (c *inlineCursor) ForceLiteral() (tokens []token.Inline, complete bool) {
	if c == nil || c.state == nil {
		return nil, true
	}
	c.state.sealLocal = true
	return c.Resume()
}

// cloneFor snapshots a paused cursor against a cloned BlockState. It is used
// by Stream.Provisional so speculative definitions and memo frontiers never
// mutate the live incremental session.
func (c *inlineCursor) cloneFor(block *BlockState) *inlineCursor {
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
	return &inlineCursor{state: &cloned}
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
	// textFrom and textEnd record that text is exactly src[textFrom:textEnd] —
	// the usual case, prose copied through unchanged — so flush can hand out a
	// substring of src instead of allocating a copy. textFrom < 0 means text was
	// built from something else (an escape, a decoded entity, a rule's output).
	textFrom, textEnd int
	context           token.InlineContext
	memos             []inlineMemoSlot
	refs              *referenceResolver
	pending           pendingReference
	waiting           bool
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
func (s *InlineState) AddText(t string) {
	s.textFrom = -1
	s.text = append(s.text, t...)
}

// AddByte folds one literal byte into the pending text token.
func (s *InlineState) AddByte(b byte) {
	s.textFrom = -1
	s.text = append(s.text, b)
}

// addSource folds src[from:to] into the pending text, remembering while the
// pending text is still one contiguous run of the source.
func (s *InlineState) addSource(from, to int) {
	switch {
	case len(s.text) == 0:
		s.textFrom, s.textEnd = from, to
	case s.textFrom >= 0 && s.textEnd == from && s.textFrom+len(s.text) == from:
		s.textEnd = to
	default:
		s.textFrom = -1
	}
	s.text = append(s.text, s.src[from:to]...)
}

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
		text := ""
		if s.textFrom >= 0 {
			// The pending text is a prefix of one source run (trailing
			// whitespace may have been trimmed off its end): alias it.
			text = s.src[s.textFrom : s.textFrom+len(s.text)]
		} else {
			text = string(s.text)
		}
		s.appendItem(inlineItem{tok: token.Inline{Node: token.Text, Text: text}})
		s.text = s.text[:0]
	}
	s.textFrom = -1
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

func flattenItems(s *InlineState, dst []token.Inline) []token.Inline {
	out := dst[:0]
	if cap(out) < len(s.items) {
		out = make([]token.Inline, 0, len(s.items))
	}
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
