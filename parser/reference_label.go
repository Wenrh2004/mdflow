package parser

import (
	"strings"
	"unicode/utf8"
)

//go:generate go run ../internal/generate/casefold -source ../internal/generate/casefold/CaseFolding-15.0.0.txt -out casefold_gen.go

// normalizeReferenceLabel implements CommonMark reference-label matching:
// ASCII space, tab, CR, and LF runs collapse to one space after trimming, then
// Unicode's default full case fold is applied. It deliberately performs no
// Unicode normalisation; canonically equivalent spellings remain distinct.
func normalizeReferenceLabel(label string) string {
	if isASCIIString(label) {
		return normalizeASCIIReferenceLabel(label)
	}

	var out strings.Builder
	out.Grow(len(label))
	spacePending := false
	wrote := false
	for len(label) > 0 {
		r, size := utf8.DecodeRuneInString(label)
		label = label[size:]
		if r < utf8.RuneSelf && isReferenceLabelSpace(byte(r)) {
			spacePending = wrote
			continue
		}
		if spacePending {
			out.WriteByte(' ')
			spacePending = false
		}
		writeFullCaseFold(&out, r)
		wrote = true
	}
	return out.String()
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func normalizeASCIIReferenceLabel(label string) string {
	if asciiReferenceLabelIsNormalized(label) {
		return label
	}

	var out strings.Builder
	out.Grow(len(label))
	spacePending := false
	wrote := false
	for i := 0; i < len(label); i++ {
		c := label[i]
		if isReferenceLabelSpace(c) {
			spacePending = wrote
			continue
		}
		if spacePending {
			out.WriteByte(' ')
			spacePending = false
		}
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out.WriteByte(c)
		wrote = true
	}
	return out.String()
}

func asciiReferenceLabelIsNormalized(label string) bool {
	if len(label) == 0 || isReferenceLabelSpace(label[0]) || isReferenceLabelSpace(label[len(label)-1]) {
		return len(label) == 0
	}
	previousWasSpace := false
	for i := 0; i < len(label); i++ {
		c := label[i]
		if 'A' <= c && c <= 'Z' {
			return false
		}
		space := isReferenceLabelSpace(c)
		if space && (previousWasSpace || c != ' ') {
			return false
		}
		previousWasSpace = space
	}
	return true
}

func isReferenceLabelSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func writeFullCaseFold(out *strings.Builder, r rune) {
	if 'A' <= r && r <= 'Z' {
		out.WriteByte(byte(r + ('a' - 'A')))
		return
	}
	lo, hi := 0, len(fullCaseFoldEntries)
	for lo < hi {
		middle := int(uint(lo+hi) >> 1)
		if fullCaseFoldEntries[middle].source < r {
			lo = middle + 1
		} else {
			hi = middle
		}
	}
	if lo == len(fullCaseFoldEntries) || fullCaseFoldEntries[lo].source != r {
		out.WriteRune(r)
		return
	}
	entry := fullCaseFoldEntries[lo]
	for _, folded := range fullCaseFoldRunes[int(entry.offset) : int(entry.offset)+int(entry.length)] {
		out.WriteRune(folded)
	}
}
