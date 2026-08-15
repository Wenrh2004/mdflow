package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

var opaqueEmphasisTag = token.NewAtomicTag("parser_test_opaque_emphasis")

type opaqueEmphasisRule struct{}

func (opaqueEmphasisRule) Name() string     { return "opaque_emphasis_test" }
func (opaqueEmphasisRule) Triggers() []byte { return []byte{'~'} }
func (opaqueEmphasisRule) Match(s *InlineState) bool {
	s.Emit(token.Inline{Node: token.Custom, Tag: opaqueEmphasisTag, Text: "opaque"})
	s.Advance(1)
	return true
}

func TestEmphasisWrapsOpaqueExtensionToken(t *testing.T) {
	rules := New()
	rules.AddInlineRule(opaqueEmphasisRule{})
	got := rules.Inline().Parse("*~*")
	want := []token.Inline{
		{Node: token.Emph},
		{Node: token.Custom, Tag: opaqueEmphasisTag, Text: "opaque"},
		{Node: token.Emph, Close: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse(*~*) = %#v, want %#v", got, want)
	}
}

func TestCommonMarkUnicodeWhitespaceClass(t *testing.T) {
	for _, r := range []rune{'\t', '\n', '\f', '\r', ' ', '\u00a0'} {
		if !isUnicodeWhitespace(r) {
			t.Errorf("%U must be CommonMark Unicode whitespace", r)
		}
	}
	for _, r := range []rune{'\v', '\u0085', '\u2028', '\u2029'} {
		if isUnicodeWhitespace(r) {
			t.Errorf("%U is not in CommonMark's Unicode whitespace class", r)
		}
	}
}

func TestEmphasisDelimiterWorkScalesLinearly(t *testing.T) {
	makeInput := func(n int) string {
		// Alternating potentially bidirectional runs force the matcher through
		// every rule-of-three/openers-bottom bucket while leaving many runs
		// unmatched. This is the shape that turns a naive backward scan into
		// quadratic work.
		return strings.Repeat("a**b*c_", n) + " " + strings.Repeat("_d*e**f", n)
	}

	rules := New().inline
	var base int
	for _, scale := range []int{1, 2, 4} {
		src := makeInput(256 * scale)
		_, work := rules.parse(src)
		t.Logf("emphasis scale=%d bytes=%d work=%d", scale, len(src), work)
		if work <= 0 {
			t.Fatalf("scale %d: emphasis pass reported no work", scale)
		}
		if work > 16*len(src) {
			t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
		}
		if scale == 1 {
			base = work
			continue
		}
		// A fixed-size separator accounts for the small additive term. Doubling
		// or quadrupling the delimiter corpus may not multiply work by more than
		// the same factor plus that constant.
		if work > base*scale+64 {
			t.Fatalf("scale %d: work grew from %d to %d", scale, base, work)
		}
	}
}

func TestNestedBracketWorkScalesLinearly(t *testing.T) {
	rules := New().inline
	var base int
	for _, scale := range []int{1, 2, 4} {
		src := strings.Repeat("[", 512*scale) + strings.Repeat("]", 512*scale)
		_, work := rules.parse(src)
		t.Logf("brackets scale=%d bytes=%d work=%d", scale, len(src), work)
		if work <= 0 {
			t.Fatalf("scale %d: inline pass reported no bracket work", scale)
		}
		if work > 8*len(src) {
			t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
		}
		if scale == 1 {
			base = work
			continue
		}
		if work > base*scale+32 {
			t.Fatalf("scale %d: work grew from %d to %d", scale, base, work)
		}
	}
}

func TestNestedImageBracketWorkScalesLinearly(t *testing.T) {
	inputs := map[string]func(int) string{
		"nested openers": func(n int) string {
			return strings.Repeat("![", n) + strings.Repeat("]", n)
		},
		"failed tails": func(n int) string {
			return strings.Repeat("![](", n)
		},
	}

	for name, makeInput := range inputs {
		t.Run(name, func(t *testing.T) {
			rules := New().inline
			var base int
			for _, scale := range []int{1, 2, 4} {
				src := makeInput(512 * scale)
				_, work := rules.parse(src)
				t.Logf("image brackets scale=%d bytes=%d work=%d", scale, len(src), work)
				if work <= 0 {
					t.Fatalf("scale %d: inline pass reported no image-bracket work", scale)
				}
				if work > 8*len(src) {
					t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
				}
				if scale == 1 {
					base = work
					continue
				}
				if work > base*scale+32 {
					t.Fatalf("scale %d: work grew from %d to %d", scale, base, work)
				}
			}
		})
	}
}
