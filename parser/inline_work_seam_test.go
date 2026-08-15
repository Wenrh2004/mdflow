package parser

import (
	"strings"
	"testing"
)

// The deterministic work counter is the complexity-regression seam that keeps
// the built-in scanners provably linear. ParseWork is its public entry so an
// extension in another module can hold its own rules to the same standard.
// This pins that the public method is reachable, returns a non-zero count, and
// scales linearly with input the way the internal path does.
func TestParseWorkIsPublicAndLinear(t *testing.T) {
	rules := New().Inline()
	var base int
	for _, scale := range []int{1, 2, 4} {
		src := strings.Repeat("*a* ", 1024*scale)
		tokens, work := rules.ParseWork(src)
		if len(tokens) == 0 {
			t.Fatalf("scale %d: ParseWork returned no tokens", scale)
		}
		if work <= 0 {
			t.Fatalf("scale %d: ParseWork reported no work", scale)
		}
		if work > 8*len(src) {
			t.Fatalf("scale %d: %d work units for %d bytes is not linear", scale, work, len(src))
		}
		if scale == 1 {
			base = work
			continue
		}
		if work > base*scale+64 {
			t.Fatalf("scale %d: work grew from %d to %d, super-linearly", scale, base, work)
		}
	}
}
