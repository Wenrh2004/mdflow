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
	rules     map[byte][]InlineRule
	post      []inlinePost
	triggers  [256]bool
	statePool sync.Pool
}

// Parse turns leaf-block text into a flat inline token stream.
func (p *InlineRules) Parse(src string) []token.Inline {
	// Fast path: most prose contains no inline trigger byte at all. Emit a
	// single text token aliasing src (zero copy) and skip the item sequence,
	// delimiter pairing and flattening machinery entirely. HTML-special bytes
	// (< > & ") are not triggers — escaping is the renderer's job — so this
	// path stays correct for them.
	if !p.hasTrigger(src) {
		if src == "" {
			return nil
		}
		return []token.Inline{{Node: token.Text, Text: src}}
	}

	s, _ := p.statePool.Get().(*InlineState)
	if s == nil {
		s = &InlineState{parser: p}
	}
	s.src, s.pos, s.items, s.text = src, 0, s.items[:0], s.text[:0]

	for s.pos < len(src) {
		matched := false
		for _, r := range p.rules[src[s.pos]] {
			if r.Match(s) {
				matched = true
				break
			}
		}
		if !matched {
			s.AddByte(src[s.pos])
			s.pos++
		}
	}
	s.flush()
	items := s.items
	for _, pp := range p.post {
		items = pp.process(items)
	}
	out := flattenItems(items)
	// Safe to recycle: `out`'s tokens and their strings do not alias s.items or
	// s.text (text was copied out by flush). A recursive call during the scan
	// took a different instance from the pool.
	p.statePool.Put(s)
	return out
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

// InlineState is one inline parse's mutable state.
type InlineState struct {
	src    string
	pos    int
	parser *InlineRules
	items  []inlineItem // value slice: one backing array for the whole run
	text   []byte       // text accumulator, reused across leaves via statePool
}

// Src returns the string being parsed.
func (s *InlineState) Src() string { return s.src }

// Pos returns the current scan offset.
func (s *InlineState) Pos() int { return s.pos }

// Advance moves the scan offset forward by n bytes.
func (s *InlineState) Advance(n int) { s.pos += n }

// AddText folds literal text into the pending text token.
func (s *InlineState) AddText(t string) { s.text = append(s.text, t...) }

// AddByte folds one literal byte into the pending text token.
func (s *InlineState) AddByte(b byte) { s.text = append(s.text, b) }

// Emit settles the pending text, then appends one finished token.
func (s *InlineState) Emit(tok token.Inline) {
	s.flush()
	s.items = append(s.items, inlineItem{tok: tok})
}

// EmitAll appends a run of finished tokens (e.g. a recursively parsed link text).
func (s *InlineState) EmitAll(toks []token.Inline) {
	s.flush()
	for _, t := range toks {
		s.items = append(s.items, inlineItem{tok: t})
	}
}

// Parse recursively parses a substring with the same rule set.
func (s *InlineState) Parse(sub string) []token.Inline { return s.parser.Parse(sub) }

func (s *InlineState) flush() {
	if len(s.text) > 0 {
		s.items = append(s.items, inlineItem{tok: token.Inline{Node: token.Text, Text: string(s.text)}})
		s.text = s.text[:0]
	}
}

// addDelimiter records a pending delimiter run for the pairing pass.
//
// The literal fallback string is deliberately not materialised here: most runs
// are either consumed by a pair or are short, so only genuinely leftover runs
// pay for a strings.Repeat, in flattenItems.
func (s *InlineState) addDelimiter(ch byte, n int, canOpen, canClose bool) {
	s.flush()
	s.items = append(s.items, inlineItem{delim: delimRun{isDelim: true, char: ch, length: n, canOpen: canOpen, canClose: canClose}})
}

// inlineItem is the intermediate representation between the two passes: either
// a settled token or a pending delimiter run. delim is inlined by value so the
// value slice eliminates per-token heap allocation.
type inlineItem struct {
	tok   token.Inline
	delim delimRun
}

type delimRun struct {
	isDelim  bool
	char     byte
	length   int
	canOpen  bool
	canClose bool
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

// codeSpanRule handles inline code: equal-length backtick runs, literal inside.
type codeSpanRule struct{}

func (codeSpanRule) Name() string     { return "code_span" }
func (codeSpanRule) Triggers() []byte { return []byte{'`'} }
func (codeSpanRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	n := runLength(src, i, '`')
	if end, ok := findBacktickClose(src, i+n, n); ok {
		s.Emit(token.Inline{Node: token.CodeSpan, Text: normalizeCodeSpan(src[i+n : end])})
		s.pos = end + n
	} else {
		s.AddText(src[i : i+n]) // unmatched run is literal
		s.pos += n
	}
	return true
}

// linkRule handles `[text](dest "title")`.
type linkRule struct{}

func (linkRule) Name() string     { return "link" }
func (linkRule) Triggers() []byte { return []byte{'['} }
func (linkRule) Match(s *InlineState) bool {
	dest, title, inner, next, ok := tryParseLink(s.parser, s.src, s.pos)
	if !ok {
		return false
	}
	s.Emit(token.Inline{Node: token.Link, Dest: dest, Title: title})
	s.EmitAll(inner)
	s.Emit(token.Inline{Node: token.Link, Close: true})
	s.pos = next
	return true
}

// imageRule handles `![alt](src "title")`. The alt text is parsed with the same
// rules and flattened to plain text by the renderer.
type imageRule struct{}

func (imageRule) Name() string     { return "image" }
func (imageRule) Triggers() []byte { return []byte{'!'} }
func (imageRule) Match(s *InlineState) bool {
	if s.pos+1 >= len(s.src) || s.src[s.pos+1] != '[' {
		return false
	}
	dest, title, inner, next, ok := tryParseLink(s.parser, s.src, s.pos+1)
	if !ok {
		return false
	}
	s.Emit(token.Inline{Node: token.Image, Dest: dest, Title: title})
	s.EmitAll(inner)
	s.Emit(token.Inline{Node: token.Image, Close: true})
	s.pos = next
	return true
}

// autolinkRule handles `<https://example.com>` and `<user@example.com>`.
type autolinkRule struct{}

func (autolinkRule) Name() string     { return "autolink" }
func (autolinkRule) Triggers() []byte { return []byte{'<'} }
func (autolinkRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	end := strings.IndexByte(src[i:], '>')
	if end < 0 {
		return false
	}
	body := src[i+1 : i+end]
	if body == "" || strings.ContainsAny(body, " \t\n<") {
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
	s.pos = i + end + 1
	return true
}

// emphasisRule records `*`/`_` runs as pending delimiters for emphasisPost.
type emphasisRule struct{}

func (emphasisRule) Name() string     { return "emphasis" }
func (emphasisRule) Triggers() []byte { return []byte{'*', '_'} }
func (emphasisRule) Match(s *InlineState) bool {
	src, i := s.src, s.pos
	c := src[i]
	n := runLength(src, i, c)
	// Simplified flanking: openable if followed by non-space, closable if
	// preceded by non-space.
	canOpen := i+n < len(src) && !isSpaceByte(src[i+n])
	canClose := i > 0 && !isSpaceByte(src[i-1])
	s.addDelimiter(c, n, canOpen, canClose)
	s.pos += n
	return true
}

type emphasisPost struct{}

func (emphasisPost) process(items []inlineItem) []inlineItem { return processEmphasis(items) }

// ---- lexical helpers (pure) ----

func runLength(s string, pos int, c byte) int {
	n := 0
	for pos+n < len(s) && s[pos+n] == c {
		n++
	}
	return n
}

// findBacktickClose finds a backtick run of length exactly n starting at or
// after from.
func findBacktickClose(s string, from, n int) (int, bool) {
	for i := from; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		run := runLength(s, i, '`')
		if run == n {
			return i, true
		}
		i += run
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
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			return i > 0
		}
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-') {
			return false
		}
	}
	return false
}

func isEmailLike(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.IndexByte(s[at+1:], '.') > 0
}

// tryParseLink parses `[text](dest "title")` starting at the '[' at pos.
func tryParseLink(p *InlineRules, src string, pos int) (dest, title string, inner []token.Inline, next int, ok bool) {
	depth := 1
	textEnd := -1
	i := pos + 1
	for i < len(src) {
		c := src[i]
		if c == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				textEnd = i
				break
			}
		}
		i++
	}
	if textEnd < 0 || textEnd+1 >= len(src) || src[textEnd+1] != '(' {
		return "", "", nil, 0, false
	}

	j := textEnd + 2
	if j < len(src) && src[j] == '<' {
		k := strings.IndexByte(src[j:], '>')
		if k < 0 {
			return "", "", nil, 0, false
		}
		dest = src[j+1 : j+k]
		j += k + 1
	} else {
		parens := 0
		k := j
		for k < len(src) {
			c := src[k]
			if isSpaceByte(c) {
				break // whitespace ends the destination; a title may follow
			}
			if c == '(' {
				parens++
			} else if c == ')' {
				if parens == 0 {
					break
				}
				parens--
			}
			k++
		}
		if k >= len(src) {
			return "", "", nil, 0, false
		}
		dest = src[j:k]
		j = k
	}

	// Optional title: "..." or '...' or (...)
	for j < len(src) && isSpaceByte(src[j]) {
		j++
	}
	if j < len(src) && (src[j] == '"' || src[j] == '\'') {
		q := src[j]
		k := strings.IndexByte(src[j+1:], q)
		if k < 0 {
			return "", "", nil, 0, false
		}
		title = src[j+1 : j+1+k]
		j += k + 2
		for j < len(src) && isSpaceByte(src[j]) {
			j++
		}
	}

	if j >= len(src) || src[j] != ')' {
		return "", "", nil, 0, false
	}
	return dest, title, p.Parse(src[pos+1 : textEnd]), j + 1, true
}

// processEmphasis is the second pass: pair pending delimiters into Emph/Strong.
func processEmphasis(items []inlineItem) []inlineItem {
	for {
		matched := false
		for ci := 0; ci < len(items); ci++ {
			closer := items[ci].delim
			if !closer.isDelim || !closer.canClose || closer.length == 0 {
				continue
			}
			oi := -1
			for k := ci - 1; k >= 0; k-- {
				opener := items[k].delim
				if opener.isDelim && opener.canOpen && opener.length > 0 && opener.char == closer.char {
					oi = k
					break
				}
			}
			if oi < 0 {
				continue
			}

			use := 1
			node := token.Emph
			if items[oi].delim.length >= 2 && items[ci].delim.length >= 2 {
				use = 2
				node = token.Strong
			}
			items[oi].delim.length -= use
			items[ci].delim.length -= use

			// Flat: bracket the span with an open and a close token instead of
			// collecting the middle into Children.
			rebuilt := make([]inlineItem, 0, len(items)+2)
			rebuilt = append(rebuilt, items[:oi]...)
			if items[oi].delim.length > 0 {
				rebuilt = append(rebuilt, items[oi])
			}
			rebuilt = append(rebuilt, inlineItem{tok: token.Inline{Node: node}})
			rebuilt = append(rebuilt, items[oi+1:ci]...)
			rebuilt = append(rebuilt, inlineItem{tok: token.Inline{Node: node, Close: true}})
			if items[ci].delim.length > 0 {
				rebuilt = append(rebuilt, items[ci])
			}
			rebuilt = append(rebuilt, items[ci+1:]...)
			items = rebuilt
			matched = true
			break
		}
		if !matched {
			return items
		}
	}
}

// flattenItems produces the final flat token stream; leftover delimiter runs
// materialise as literal text only now.
func flattenItems(items []inlineItem) []token.Inline {
	out := make([]token.Inline, 0, len(items))
	for i := range items {
		it := &items[i]
		if it.delim.isDelim {
			if it.delim.length == 0 {
				continue
			}
			out = append(out, token.Inline{Node: token.Text, Text: strings.Repeat(string(it.delim.char), it.delim.length)})
			continue
		}
		out = append(out, it.tok)
	}
	return mergeAdjacentText(out)
}

func mergeAdjacentText(in []token.Inline) []token.Inline {
	out := in[:0]
	for _, n := range in {
		if len(out) > 0 && n.Node == token.Text && out[len(out)-1].Node == token.Text {
			out[len(out)-1].Text += n.Text
			continue
		}
		out = append(out, n)
	}
	return out
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}
