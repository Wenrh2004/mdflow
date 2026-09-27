package parser

import (
	"strings"
	"unicode/utf8"

	"github.com/Wenrh2004/mdflow/internal/ascii"
)

//go:generate go run ../internal/generate/entities -source ../internal/generate/entities/entities.json -out entities_gen.go

const (
	maxDecimalEntityDigits = 7
	maxHexEntityDigits     = 6
)

// scanCharacterReference scans the strict CommonMark form beginning at src[0].
// CommonMark requires the semicolon for both named and numeric references. It
// also caps decimal references at seven digits and hexadecimal references at
// six; an overlong spelling stays literal rather than decoding a prefix.
func scanCharacterReference(src string) (value string, n int, ok bool) {
	if len(src) < 3 || src[0] != '&' {
		return "", 0, false
	}
	if src[1] != '#' {
		return scanNamedCharacterReference(src)
	}

	i, base, limit := 2, uint32(10), maxDecimalEntityDigits
	if i < len(src) && (src[i] == 'x' || src[i] == 'X') {
		i++
		base, limit = 16, maxHexEntityDigits
	}
	start := i
	var codepoint uint32
	for i < len(src) && i-start < limit {
		digit, valid := entityDigit(src[i], base)
		if !valid {
			break
		}
		codepoint = codepoint*base + digit
		i++
	}
	if i == start || i >= len(src) || src[i] != ';' {
		return "", 0, false
	}

	if codepoint == 0 || codepoint > utf8.MaxRune || 0xd800 <= codepoint && codepoint <= 0xdfff {
		codepoint = utf8.RuneError
	}
	return string(rune(codepoint)), i + 1, true
}

// unescapeSource removes backslash escapes and decodes character references in
// one left-to-right pass. It is intentionally called only after a construct's
// syntax has been accepted: decoded punctuation is literal data and cannot
// participate in recognising that construct. Advancing past each accepted
// spelling also prevents one expansion from being interpreted a second time.
// Inputs without an escape or valid reference are returned unchanged.
func unescapeSource(src string) string {
	search, last := 0, 0
	changed := false
	var out strings.Builder
	for search < len(src) {
		start, value, n := search, "", 0
		switch {
		case src[search] == '\\' && search+1 < len(src) && isASCIIPunct(src[search+1]):
			value, n = src[search+1:search+2], 2
		case src[search] == '&':
			var ok bool
			value, n, ok = scanCharacterReference(src[search:])
			if !ok {
				search++
				continue
			}
		default:
			search++
			continue
		}
		if !changed {
			out.Grow(len(src))
			changed = true
		}
		out.WriteString(src[last:start])
		out.WriteString(value)
		last = start + n
		search = last
	}
	if !changed {
		return src
	}
	out.WriteString(src[last:])
	return out.String()
}

func scanNamedCharacterReference(src string) (value string, n int, ok bool) {
	i := 1
	for i < len(src) && ascii.IsAlnum(src[i]) {
		i++
	}
	if i == 1 || i >= len(src) || src[i] != ';' {
		return "", 0, false
	}
	value, ok = lookupHTMLEntity(src[1:i])
	if !ok {
		return "", 0, false
	}
	return value, i + 1, true
}

func entityDigit(c byte, base uint32) (uint32, bool) {
	switch {
	case '0' <= c && c <= '9':
		return uint32(c - '0'), true
	case base == 16 && 'a' <= c && c <= 'f':
		return uint32(c-'a') + 10, true
	case base == 16 && 'A' <= c && c <= 'F':
		return uint32(c-'A') + 10, true
	default:
		return 0, false
	}
}

// lookupHTMLEntity performs a zero-allocation binary search over the generated,
// sorted table. A map would add construction/allocation work to every parser
// process even when ordinary prose never contains a reference.
func lookupHTMLEntity(name string) (string, bool) {
	lo, hi := 0, len(entityIndex)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entityName(mid) < name {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(entityIndex) && entityName(lo) == name {
		return entityValue(lo), true
	}
	return "", false
}

// entityName and entityValue read record i of the packed table: a length byte
// and the name, then a length byte and the value. Both are substrings of the
// constant, so neither allocates.
func entityName(i int) string {
	off := int(entityIndex[i])
	n := int(entityData[off])
	return entityData[off+1 : off+1+n]
}

func entityValue(i int) string {
	off := int(entityIndex[i])
	off += 1 + int(entityData[off])
	n := int(entityData[off])
	return entityData[off+1 : off+1+n]
}
