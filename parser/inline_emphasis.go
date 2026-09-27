package parser

import (
	"unicode"
	"unicode/utf8"

	"github.com/Wenrh2004/mdflow/token"
)

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
