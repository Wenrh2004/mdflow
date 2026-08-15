package rawhtml

import (
	"strings"
	"testing"
)

func TestInlineTagWhitespaceAllowsAtMostOneLineEndingPerRun(t *testing.T) {
	valid := []string{
		"<a\nx=y>",
		"<a\r\nx=y>",
		"<a\nx\n=\ny>",
		"</a\n>",
	}
	for _, src := range valid {
		if end, ok := scanInline(src, 0); !ok || end != len(src) {
			t.Errorf("valid tag rejected: %q (end=%d ok=%v)", src, end, ok)
		}
	}

	invalid := []string{
		"<a\n\nx=y>",
		"<a\r\rx=y>",
		"<a x\n\n=y>",
		"<a x=\n\ny>",
		"</a\n\n>",
	}
	for _, src := range invalid {
		if _, ok := scanInline(src, 0); ok {
			t.Errorf("tag with two line endings in one whitespace run accepted: %q", src)
		}
	}
}

func TestUnclosedInlineTerminatorsHaveLinearScanWork(t *testing.T) {
	cases := []struct {
		name, unit string
	}{
		{"comment", "<!--x>"},
		{"instruction", "<?x>"},
		{"cdata", "<![CDATA[x>"},
		{"declaration", "<!X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const base = 128
			work := make([]int, 3)
			for i, scale := range []int{1, 2, 4} {
				src := strings.Repeat(tc.unit, base*scale)
				memo := new(inlineMemo)
				for at := 0; at < len(src); at++ {
					if src[at] == '<' {
						scanInlineWithMemo(src, at, memo)
					}
				}
				work[i] = memo.work
				if memo.work > len(src) {
					t.Fatalf("scanner revisited an already-proved suffix at %dx: work=%d input=%d", scale, memo.work, len(src))
				}
			}
			if work[1] > 2*work[0]+len(tc.unit) || work[2] > 2*work[1]+len(tc.unit) {
				t.Fatalf("scanner work is not linear at 1x/2x/4x: %v", work)
			}
		})
	}
}

func TestInlineMemoCloneIsIndependent(t *testing.T) {
	original := &inlineMemo{absentFrom: [4]int{1, 2, 3, 4}, absent: 0xf, work: 9}
	clone := original.CloneInlineMemo().(*inlineMemo)
	clone.absentFrom[0] = 99
	clone.absent = 0
	clone.work = 0

	if original.absentFrom[0] != 1 || original.absent != 0xf || original.work != 9 {
		t.Fatalf("memo clone aliases original: original=%+v clone=%+v", original, clone)
	}
}
