package rawhtml

import (
	"strings"

	"github.com/Wenrh2004/mdflow/internal/ascii"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// inlineRule recognises the six raw-HTML forms in CommonMark 0.31.2. Each
// match stays one atomic token containing the exact source spelling.
type inlineRule struct{}

var inlineMemoTag = token.NewTag("github.com/Wenrh2004/mdflow/internal/rawhtml.raw_html_inline_memo")

func (inlineRule) Name() string     { return "raw_html" }
func (inlineRule) Triggers() []byte { return []byte{'<'} }
func (inlineRule) Match(s *parser.InlineState) bool {
	start := s.Pos()
	memo := s.Memo(inlineMemoTag, newInlineMemo).(*inlineMemo)
	end, ok := scanInlineWithMemo(s.Src(), start, memo)
	if !ok {
		return false
	}
	s.Emit(token.Inline{Node: token.Custom, Tag: InlineTag, Text: s.Src()[start:end]})
	s.Advance(end - start)
	return true
}

func scanInline(src string, start int) (int, bool) {
	return scanInlineWithMemo(src, start, nil)
}

type inlineTerminator uint8

const (
	commentTerminator inlineTerminator = iota
	instructionTerminator
	cdataTerminator
	declarationTerminator
)

type inlineMemo struct {
	absentFrom [4]int
	absent     uint8
	work       int
}

func newInlineMemo() parser.InlineMemo { return new(inlineMemo) }

func (m *inlineMemo) CloneInlineMemo() parser.InlineMemo {
	clone := *m
	return &clone
}

func scanInlineWithMemo(src string, start int, memo *inlineMemo) (int, bool) {
	if start < 0 || start+2 > len(src) || src[start] != '<' {
		return 0, false
	}
	tail := src[start:]
	switch {
	case strings.HasPrefix(tail, "<!--"):
		return scanUntil(src, start+2, "-->", commentTerminator, memo)
	case strings.HasPrefix(tail, "<?"):
		return scanUntil(src, start+2, "?>", instructionTerminator, memo)
	case strings.HasPrefix(tail, "<![CDATA["):
		return scanUntil(src, start+9, "]]>", cdataTerminator, memo)
	case tail[1] == '!' && start+2 < len(src) && ascii.IsAlpha(src[start+2]):
		return scanUntil(src, start+3, ">", declarationTerminator, memo)
	case tail[1] == '/':
		return scanCloseTag(src, start)
	default:
		return scanOpenTag(src, start)
	}
}

func scanUntil(src string, from int, terminator string, kind inlineTerminator, memo *inlineMemo) (int, bool) {
	mask := uint8(1) << kind
	if memo != nil && memo.absent&mask != 0 && from >= memo.absentFrom[kind] {
		return 0, false
	}
	i := strings.Index(src[from:], terminator)
	if memo != nil {
		if i < 0 {
			memo.work += len(src) - from
		} else {
			memo.work += i + len(terminator)
		}
	}
	if i < 0 {
		if memo != nil {
			memo.absent |= mask
			memo.absentFrom[kind] = from
		}
		return 0, false
	}
	return from + i + len(terminator), true
}

func scanOpenTag(src string, start int) (int, bool) {
	i := start + 1
	if i >= len(src) || !ascii.IsAlpha(src[i]) {
		return 0, false
	}
	i++
	for i < len(src) && isTagNameRest(src[i]) {
		i++
	}

	for {
		if end, ok := scanTagEnd(src, i); ok {
			return end, true
		}
		if i >= len(src) || !isHTMLWhitespace(src[i]) {
			return 0, false
		}
		var whitespaceOK bool
		i, whitespaceOK = skipHTMLWhitespace(src, i)
		if !whitespaceOK {
			return 0, false
		}
		if end, ok := scanTagEnd(src, i); ok {
			return end, true
		}
		var ok bool
		i, ok = scanAttribute(src, i)
		if !ok {
			return 0, false
		}
	}
}

func scanCloseTag(src string, start int) (int, bool) {
	i := start + 2
	if i >= len(src) || !ascii.IsAlpha(src[i]) {
		return 0, false
	}
	i++
	for i < len(src) && isTagNameRest(src[i]) {
		i++
	}
	var whitespaceOK bool
	i, whitespaceOK = skipHTMLWhitespace(src, i)
	if !whitespaceOK {
		return 0, false
	}
	if i >= len(src) || src[i] != '>' {
		return 0, false
	}
	return i + 1, true
}

func scanTagEnd(src string, i int) (int, bool) {
	if i >= len(src) {
		return 0, false
	}
	if src[i] == '>' {
		return i + 1, true
	}
	if src[i] == '/' && i+1 < len(src) && src[i+1] == '>' {
		return i + 2, true
	}
	return 0, false
}

func scanAttribute(src string, i int) (int, bool) {
	if i >= len(src) || !isAttributeNameStart(src[i]) {
		return 0, false
	}
	i++
	for i < len(src) && isAttributeNameRest(src[i]) {
		i++
	}
	nameEnd := i
	var whitespaceOK bool
	i, whitespaceOK = skipHTMLWhitespace(src, i)
	if !whitespaceOK {
		return 0, false
	}
	if i >= len(src) || src[i] != '=' {
		// Leave separator whitespace for the outer attribute loop. Consuming it
		// here would make a following boolean attribute look adjacent.
		return nameEnd, true
	}
	i, whitespaceOK = skipHTMLWhitespace(src, i+1)
	if !whitespaceOK {
		return 0, false
	}
	if i >= len(src) {
		return 0, false
	}

	if src[i] == '"' || src[i] == '\'' {
		quote := src[i]
		i++
		for i < len(src) && src[i] != quote {
			i++
		}
		if i >= len(src) {
			return 0, false
		}
		return i + 1, true
	}

	start := i
	for i < len(src) && !isHTMLWhitespace(src[i]) && !isForbiddenUnquoted(src[i]) {
		i++
	}
	if i == start {
		return 0, false
	}
	return i, true
}

func skipHTMLWhitespace(src string, i int) (int, bool) {
	seenLineEnding := false
	for i < len(src) {
		switch src[i] {
		case ' ', '\t':
			i++
		case '\n':
			if seenLineEnding {
				return i, false
			}
			seenLineEnding = true
			i++
		case '\r':
			if seenLineEnding {
				return i, false
			}
			seenLineEnding = true
			i++
			if i < len(src) && src[i] == '\n' {
				i++ // CRLF is one line ending.
			}
		default:
			return i, true
		}
	}
	return i, true
}

func isHTMLWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isTagNameRest(c byte) bool { return ascii.IsAlpha(c) || ascii.IsDigit(c) || c == '-' }

func isAttributeNameStart(c byte) bool { return ascii.IsAlpha(c) || c == '_' || c == ':' }

func isAttributeNameRest(c byte) bool {
	return isAttributeNameStart(c) || ascii.IsDigit(c) || c == '.' || c == '-'
}

func isForbiddenUnquoted(c byte) bool {
	switch c {
	case '"', '\'', '=', '<', '>', '`':
		return true
	}
	return false
}
