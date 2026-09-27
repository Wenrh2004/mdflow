package parser

import "strings"

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

// thematicSuffixStart returns the smallest offset from which raw could still
// hold a thematic break: the start of the longest suffix made only of spaces,
// tabs and a single marker byte. Any suffix starting earlier contains either a
// non-marker byte or two different markers, so it can never qualify.
func thematicSuffixStart(raw string) int {
	i := len(raw)
	var ch byte
	for i > 0 {
		c := raw[i-1]
		switch {
		case c == ' ' || c == '\t':
		case ch == 0 && (c == '-' || c == '*' || c == '_'):
			ch = c
		case c == ch:
		default:
			return i
		}
		i--
	}
	return 0
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
