package parser

import "testing"

var normalizedSourceSink string

func TestNormalizeSourceLineCleanInputDoesNotAllocate(t *testing.T) {
	const src = "clean UTF-8 文本 with no NUL"
	if got := testing.AllocsPerRun(1000, func() {
		normalizedSourceSink = normalizeSourceLine(src)
	}); got != 0 {
		t.Fatalf("normalizeSourceLine(clean) allocated %v times, want 0", got)
	}
	if normalizedSourceSink != src {
		t.Fatalf("normalizeSourceLine(clean) = %q, want %q", normalizedSourceSink, src)
	}
}
