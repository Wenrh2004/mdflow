package parser

import (
	"strings"
	"unicode/utf8"
)

// blockLine is a zero-copy view of the unconsumed part of one logical input
// line. column tracks CommonMark's four-column tab stops. pad is non-zero only
// when structural indentation consumes part of a tab; the tab's remaining
// visual columns then become ordinary spaces in the remainder.
type blockLine struct {
	raw    string
	off    int
	column int
	pad    uint8
}

func newBlockLine(raw string) blockLine { return blockLine{raw: raw} }

func tabWidth(column int) int { return 4 - column%4 }

// Text returns the visible remainder. The ordinary path is a substring of raw;
// only a tab split by structural indentation needs a tiny materialisation.
func (l blockLine) Text() string {
	if l.pad == 0 {
		return l.raw[l.off:]
	}
	return "   "[:l.pad] + l.raw[l.off:]
}

func (l blockLine) peek() (byte, bool) {
	if l.pad > 0 {
		return ' ', true
	}
	if l.off >= len(l.raw) {
		return 0, false
	}
	return l.raw[l.off], true
}

func (l blockLine) consumeByte(want byte) (blockLine, bool) {
	c, ok := l.peek()
	if !ok || c != want {
		return l, false
	}
	if l.pad > 0 {
		l.pad--
	} else {
		l.off++
	}
	l.column++
	return l, true
}

// consumeIndent consumes at most columns visual columns of spaces or tabs.
func (l blockLine) consumeIndent(columns int) blockLine {
	consumed := 0
	for consumed < columns {
		if l.pad > 0 {
			l.pad--
			l.column++
			consumed++
			continue
		}
		if l.off >= len(l.raw) {
			break
		}
		switch l.raw[l.off] {
		case ' ':
			l.off++
			l.column++
			consumed++
		case '\t':
			width := tabWidth(l.column)
			take := min(width, columns-consumed)
			l.off++
			l.column += take
			consumed += take
			if take < width {
				l.pad = uint8(width - take)
			}
		default:
			return l
		}
	}
	return l
}

func (l blockLine) leadingIndent() int {
	start := l.column
	for {
		if l.pad > 0 {
			l.column += int(l.pad)
			l.pad = 0
			continue
		}
		if l.off >= len(l.raw) {
			return l.column - start
		}
		switch l.raw[l.off] {
		case ' ':
			l.off++
			l.column++
		case '\t':
			l.off++
			l.column += tabWidth(l.column)
		default:
			return l.column - start
		}
	}
}

// blank reports whether the remainder is whitespace only. pad is ignored: it
// only ever stands for the leftover columns of a split tab, which are blank.
// lead returns the line's indentation in columns and its first non-blank
// byte, or 0 when the line is blank.
func (l blockLine) lead() (indent int, first byte) {
	indent = l.leadingIndent()
	rest := l.consumeIndent(indent)
	if c, ok := rest.peek(); ok && c != ' ' && c != '\t' {
		return indent, c
	}
	return indent, 0
}

func (l blockLine) blank() bool {
	for i := l.off; i < len(l.raw); i++ {
		if l.raw[i] != ' ' && l.raw[i] != '\t' {
			return false
		}
	}
	return true
}

func (l blockLine) trimRightSpaceTab() string {
	return strings.TrimRight(l.Text(), " \t")
}

// afterExtensionRemainder preserves column state when an existing public
// ContainerRule returns a suffix of the string it received. A rule that builds
// an unrelated string keeps its historical behaviour through the fallback.
func (l blockLine) afterExtensionRemainder(before, rest string) blockLine {
	if len(rest) > len(before) || !strings.HasSuffix(before, rest) {
		return newBlockLine(rest)
	}
	removed := len(before) - len(rest)
	// A suffix beginning in the middle of a UTF-8 encoding is not a meaningful
	// column-preserving view. Keep the public rule's historical returned string
	// in that unusual case rather than inventing a partial-rune column.
	if !utf8.ValidString(before[:removed]) {
		return newBlockLine(rest)
	}
	return l.consumeVisibleBytes(removed)
}

func (l blockLine) consumeVisibleBytes(n int) blockLine {
	for n > 0 {
		if l.pad > 0 {
			take := min(n, int(l.pad))
			l.pad -= uint8(take)
			l.column += take
			n -= take
			continue
		}
		if l.off >= len(l.raw) {
			break
		}
		if l.raw[l.off] == '\t' {
			l.column += tabWidth(l.column)
			l.off++
			n--
			continue
		}
		_, size := utf8.DecodeRuneInString(l.raw[l.off:])
		l.off += size
		l.column++
		n -= size
	}
	return l
}
