package parser

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
