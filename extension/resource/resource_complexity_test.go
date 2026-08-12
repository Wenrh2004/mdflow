package resource

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow/parser"
)

// A run of unmatched "[" must not drive the [[reference]] scanner quadratic.
// referenceRule.Match reports its scan span through InlineState.AddWork, so the
// deterministic work count exposed by ParseWork reflects the rule's real cost.
// Before memoization this grows as O(n^2) with the input; this test forbids it,
// holding the extension scanner to the same linearity standard as the built-ins.
func TestReferenceScanStaysLinear(t *testing.T) {
	rules := parser.New()
	Syntax(rules)
	inline := rules.Inline()

	var base int
	for _, scale := range []int{1, 2, 4} {
		src := strings.Repeat("[", 8192*scale)
		_, work := inline.ParseWork(src)
		if work <= 0 {
			t.Fatalf("scale %d: no work reported", scale)
		}
		if work > 16*len(src) {
			t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
		}
		if scale == 1 {
			base = work
			continue
		}
		if work > base*scale+128 {
			t.Fatalf("scale %d: work grew from %d to %d — super-linear scan", scale, base, work)
		}
	}
}
