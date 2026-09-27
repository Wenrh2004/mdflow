package parser

import (
	"strings"

	"github.com/Wenrh2004/mdflow/token"
)

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
	definition, found, refused := s.refs.resolve(pending.key)
	if found {
		s.completeBracket(bracketIndex, definition.destination, definition.title, pending.after)
		return true
	}
	if !refused && s.refs != nil && !s.refs.sealed && !s.sealLocal {
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
	definition, ok, refused := s.refs.resolve(pending.key)
	if ok {
		s.waiting = false
		s.pending = pendingReference{}
		s.completeBracket(pending.bracketIndex, definition.destination, definition.title, pending.after)
		return true
	}
	if !refused && s.refs != nil && !s.refs.sealed && !s.sealLocal {
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
