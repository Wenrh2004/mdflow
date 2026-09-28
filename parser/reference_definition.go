package parser

import (
	"strings"
	"unicode/utf8"
)

const maxReferenceLabelRunes = 999

// referenceDefinition is the syntax payload stored for a normalised link
// label. The block parser decides where definitions are allowed; this file
// only recognises a definition at the beginning of the supplied source.
type referenceDefinition struct {
	destination string
	title       string
}

// referenceResolver owns the document-wide, first-definition-wins lookup.
// Its zero value is ready for use.
type referenceResolver struct {
	definitions map[string]referenceDefinition
	sealed      bool

	// input counts the source bytes the block phase has consumed. It sizes the
	// expansion budget and is written only by the block phase, before any
	// parallel worker reads it.
	input int64
	// expanded is the destination and title bytes references have expanded to
	// so far. Every [x] copies its definition into the output, so a long
	// definition used many times turns kilobytes of input into gigabytes of
	// output. cmark bounds the total the same way.
	expanded int64
}

// minReferenceExpansion is the expansion budget a small document always has,
// so an ordinary short page is never refused.
const minReferenceExpansion = 100 << 10

// resolve looks a normalised key up and charges its expansion against the
// document budget: max(input so far, minReferenceExpansion). Past the budget a
// defined reference is refused and renders as literal text, which is what keeps
// output linear in input.
func (r *referenceResolver) resolve(key string) (definition referenceDefinition, found, refused bool) {
	definition, found = r.lookupNormalized(key)
	if !found {
		return referenceDefinition{}, false, false
	}
	cost := int64(len(definition.destination) + len(definition.title))
	if cost == 0 {
		return definition, true, false
	}
	if r.expanded += cost; r.expanded > max(r.input, minReferenceExpansion) {
		return referenceDefinition{}, false, true
	}
	return definition, true, false
}

func (r *referenceResolver) define(label string, definition referenceDefinition) bool {
	if r == nil || r.sealed {
		return false
	}
	key := normalizeReferenceLabel(label)
	if key == "" {
		return false
	}
	if r.definitions == nil {
		r.definitions = make(map[string]referenceDefinition)
	} else if _, exists := r.definitions[key]; exists {
		return false
	}
	r.definitions[strings.Clone(key)] = referenceDefinition{
		destination: strings.Clone(definition.destination),
		title:       strings.Clone(definition.title),
	}
	return true
}

func (r *referenceResolver) lookup(label string) (referenceDefinition, bool) {
	return r.lookupNormalized(normalizeReferenceLabel(label))
}

func (r *referenceResolver) lookupNormalized(key string) (referenceDefinition, bool) {
	if r == nil || r.definitions == nil {
		return referenceDefinition{}, false
	}
	definition, ok := r.definitions[key]
	return definition, ok
}

func (r *referenceResolver) seal() {
	if r != nil {
		r.sealed = true
	}
}

func (r *referenceResolver) reset() {
	if r != nil {
		r.definitions = nil
		r.sealed = false
		r.input = 0
		r.expanded = 0
	}
}

// cloneInto copies r into dst, giving dst its own definitions map.
func (r *referenceResolver) cloneInto(dst *referenceResolver) {
	if r == nil {
		return
	}
	dst.sealed, dst.input, dst.expanded = r.sealed, r.input, r.expanded
	if len(r.definitions) == 0 {
		return
	}
	dst.definitions = make(map[string]referenceDefinition, len(r.definitions))
	for label, definition := range r.definitions {
		dst.definitions[label] = definition
	}
}

// scanReferenceDefinition recognises one CommonMark link reference
// definition. end excludes the terminating line ending, so a block driver can
// choose whether that line ending belongs to the definition or to its input
// framing.
func scanReferenceDefinition(src string) (label string, definition referenceDefinition, end int, ok bool) {
	pos := 0
	for pos < len(src) && pos < 3 && src[pos] == ' ' {
		pos++
	}
	if pos >= len(src) || src[pos] != '[' {
		return "", referenceDefinition{}, 0, false
	}

	label, pos, ok = scanReferenceDefinitionLabel(src, pos)
	if !ok || pos >= len(src) || src[pos] != ':' {
		return "", referenceDefinition{}, 0, false
	}
	pos++
	pos = scanReferenceDefinitionSpace(src, pos)

	destination, pos, ok := scanReferenceDefinitionDestination(src, pos)
	if !ok {
		return "", referenceDefinition{}, 0, false
	}
	destination = unescapeSource(destination)

	noTitleEnd := scanReferenceDefinitionHorizontalSpace(src, pos)
	noTitleOK := noTitleEnd == len(src) || referenceDefinitionLineEndingWidth(src, noTitleEnd) > 0

	separatorEnd := scanReferenceDefinitionSpace(src, pos)
	if separatorEnd > pos && separatorEnd < len(src) && isLinkTitleOpener(src[separatorEnd]) {
		title, titleEnd, titleOK := scanReferenceDefinitionTitle(src, separatorEnd)
		if titleOK {
			titleEnd = scanReferenceDefinitionHorizontalSpace(src, titleEnd)
			if titleEnd == len(src) || referenceDefinitionLineEndingWidth(src, titleEnd) > 0 {
				return label, referenceDefinition{
					destination: destination,
					title:       unescapeSource(title),
				}, titleEnd, true
			}
		}
		if noTitleOK {
			return label, referenceDefinition{destination: destination}, noTitleEnd, true
		}
		return "", referenceDefinition{}, 0, false
	}

	if !noTitleOK {
		return "", referenceDefinition{}, 0, false
	}
	return label, referenceDefinition{destination: destination}, noTitleEnd, true
}

// scanReferenceDefinitionPrefix removes the maximal leading run of reference
// definitions and registers each one. Duplicate normalised labels are still
// consumed, but the resolver keeps the first definition.
func scanReferenceDefinitionPrefix(src string, resolver *referenceResolver) int {
	consumed := 0
	for consumed < len(src) {
		label, definition, end, ok := scanReferenceDefinition(src[consumed:])
		if !ok {
			break
		}
		if resolver != nil {
			resolver.define(label, definition)
		}
		consumed += end
		width := referenceDefinitionLineEndingWidth(src, consumed)
		if width == 0 {
			break
		}
		consumed += width
	}
	return consumed
}

func scanReferenceDefinitionLabel(src string, start int) (label string, end int, ok bool) {
	if start >= len(src) || src[start] != '[' {
		return "", 0, false
	}
	labelStart := start + 1
	labelEnd, ok := scanReferenceLabelContent(src, labelStart, true)
	if !ok {
		return "", 0, false
	}
	return src[labelStart:labelEnd], labelEnd + 1, true
}

// normalizeReferenceCandidate validates a complete label body and normalises
// it once. A pending inline cursor retains this key, so unrelated definitions
// only cost one map lookup when the cursor resumes.
func normalizeReferenceCandidate(content string) (string, bool) {
	end, ok := scanReferenceLabelContent(content, 0, false)
	if !ok || end != len(content) {
		return "", false
	}
	return normalizeReferenceLabel(content), true
}

// scanReferenceLabelContent validates either a bracket-delimited label body
// (stopAtClose=true) or an exact body slice. It counts Unicode characters, not
// UTF-8 bytes, treats CRLF as one line ending, and rejects blank lines and
// unescaped brackets exactly as CommonMark's link-label grammar requires.
func scanReferenceLabelContent(src string, start int, stopAtClose bool) (end int, ok bool) {
	pos := start
	runes := 0
	hasNonWhitespace := false
	afterLineEnding := false
	for pos < len(src) {
		if width := referenceDefinitionLineEndingWidth(src, pos); width > 0 {
			if afterLineEnding {
				return 0, false
			}
			runes++
			if runes > maxReferenceLabelRunes {
				return 0, false
			}
			pos += width
			afterLineEnding = true
			continue
		}

		c := src[pos]
		if c == ']' {
			if stopAtClose && hasNonWhitespace {
				return pos, true
			}
			return 0, false
		}
		if c == '[' {
			return 0, false
		}
		if c == '\\' && pos+1 < len(src) && isASCIIPunct(src[pos+1]) {
			runes += 2
			if runes > maxReferenceLabelRunes {
				return 0, false
			}
			hasNonWhitespace = true
			afterLineEnding = false
			pos += 2
			continue
		}

		_, width := utf8.DecodeRuneInString(src[pos:])
		runes++
		if runes > maxReferenceLabelRunes {
			return 0, false
		}
		if c != ' ' && c != '\t' {
			hasNonWhitespace = true
			afterLineEnding = false
		}
		pos += width
	}
	if !stopAtClose && hasNonWhitespace {
		return pos, true
	}
	return 0, false
}

// scanReferenceDefinitionSpace consumes spaces and tabs plus at most one line
// ending. A second line ending is intentionally left for the caller to reject.
func scanReferenceDefinitionSpace(src string, start int) int {
	pos := scanReferenceDefinitionHorizontalSpace(src, start)
	if width := referenceDefinitionLineEndingWidth(src, pos); width > 0 {
		pos += width
		pos = scanReferenceDefinitionHorizontalSpace(src, pos)
	}
	return pos
}

func scanReferenceDefinitionHorizontalSpace(src string, start int) int {
	pos := start
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	return pos
}

func scanReferenceDefinitionDestination(src string, start int) (destination string, end int, ok bool) {
	if start >= len(src) {
		return "", 0, false
	}
	if src[start] == '<' {
		for pos := start + 1; pos < len(src); pos++ {
			switch src[pos] {
			case '\n', '\r', '<':
				return "", 0, false
			case '\\':
				if pos+1 < len(src) && isASCIIPunct(src[pos+1]) {
					pos++
				}
			case '>':
				return src[start+1 : pos], pos + 1, true
			}
		}
		return "", 0, false
	}

	pos := start
	depth := 0
	for pos < len(src) {
		c := src[pos]
		if c <= 0x20 || c == 0x7f {
			break
		}
		if c == '\\' && pos+1 < len(src) && isASCIIPunct(src[pos+1]) {
			pos += 2
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return "", 0, false
			}
			depth--
		}
		pos++
	}
	if pos == start || depth != 0 {
		return "", 0, false
	}
	return src[start:pos], pos, true
}

func scanReferenceDefinitionTitle(src string, start int) (title string, end int, ok bool) {
	open := src[start]
	close := open
	if open == '(' {
		close = ')'
	}
	pos := start + 1
	afterLineEnding := false
	for pos < len(src) {
		c := src[pos]
		if c == '\\' && pos+1 < len(src) && isASCIIPunct(src[pos+1]) {
			pos += 2
			afterLineEnding = false
			continue
		}
		if c == close {
			return src[start+1 : pos], pos + 1, true
		}
		if open == '(' && c == '(' {
			return "", 0, false
		}
		if width := referenceDefinitionLineEndingWidth(src, pos); width > 0 {
			if afterLineEnding {
				return "", 0, false
			}
			pos += width
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

func referenceDefinitionLineEndingWidth(src string, pos int) int {
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
