package parser

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func TestAutolinkCandidateScanWorkScalesLinearly(t *testing.T) {
	rules := New().inline
	var base int
	for _, scale := range []int{1, 2, 4} {
		src := strings.Repeat("<", 1024*scale) + "ab:x>"
		_, work := rules.parse(src)
		t.Logf("autolink scale=%d bytes=%d work=%d", scale, len(src), work)
		if work <= 0 {
			t.Fatalf("scale %d: autolink scanner reported no work", scale)
		}
		if work > 4*len(src) {
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

func TestUnmatchedVaryingBacktickScanWorkScalesLinearly(t *testing.T) {
	rules := New().inline
	var base int
	for _, scale := range []int{1, 2, 4} {
		src := unmatchedVaryingBackticks(4096 * scale)
		_, work := rules.parse(src)
		t.Logf("backticks scale=%d bytes=%d work=%d", scale, len(src), work)
		if work <= 0 {
			t.Fatalf("scale %d: backtick scanner reported no work", scale)
		}
		if work > 4*len(src) {
			t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
		}
		if scale == 1 {
			base = work
			continue
		}
		if work > base*scale+64 {
			t.Fatalf("scale %d: work grew from %d to %d", scale, base, work)
		}
	}
}

func unmatchedVaryingBackticks(size int) string {
	var src strings.Builder
	src.Grow(size)
	for run := 1; src.Len()+run+1 <= size; run++ {
		src.WriteString(strings.Repeat("`", run))
		src.WriteByte('x')
	}
	if src.Len() < size {
		src.WriteString(strings.Repeat("a", size-src.Len()))
	}
	return src.String()
}

var mergedInlineSink []token.Inline

func TestMergeAdjacentEntityTextUsesConstantAllocations(t *testing.T) {
	parts := []string{"&", "©", "Æ", "ⅆ"}
	for _, scale := range []int{1, 2, 4} {
		input := make([]token.Inline, 256*scale)
		for i := range input {
			input[i] = token.Inline{Node: token.Text, Text: parts[i%len(parts)]}
		}

		allocs := testing.AllocsPerRun(25, func() {
			working := append([]token.Inline(nil), input...)
			mergedInlineSink = mergeAdjacentText(working)
		})
		t.Logf("merge scale=%d tokens=%d allocations=%.0f", scale, len(input), allocs)
		if allocs > 4 {
			t.Fatalf("scale %d: merging one text run allocated %.0f times, want at most 4", scale, allocs)
		}
		if got := len(mergedInlineSink); got != 1 {
			t.Fatalf("scale %d: merged token count = %d, want 1", scale, got)
		}
	}
}
