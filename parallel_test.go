package mdflow_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

// bigDoc builds a document large enough to clear the fan-out size threshold.
// It stays within the CommonMark core the module ships, so the facade tests
// need no extension module to exercise the parallel path.
func bigDoc(sections int) string {
	var b strings.Builder
	for i := 0; i < sections; i++ {
		s := strconv.Itoa(i)
		b.WriteString("## Section " + s + "\n\n")
		b.WriteString("Prose with *emphasis*, **strong**, `code`, [a link](https://x.dev/" + s +
			") and an autolink <https://go.dev/" + s + ">.\n\n")
		b.WriteString("```go\nfunc f" + s + "() {}\n```\n\n")
		b.WriteString("- one\n- two\n\n> quoted **text**\n\n")
		b.WriteString("1. first\n2. second\n\n")
	}
	return b.String()
}

// The fan-out must be a pure optimisation: identical bytes, or it is worthless.
func TestParallelMatchesSequential(t *testing.T) {
	base := mdflow.New()
	for _, sections := range []int{1, 5, 50, 200} {
		src := bigDoc(sections)
		want := base.HTML(src)
		for _, n := range []int{1, 2, 3, 4, 8, 16, 64} {
			if got := base.Workers(n).HTML(src); got != want {
				t.Fatalf("sections=%d workers=%d: output differs from sequential", sections, n)
			}
		}
		if got := base.HTMLParallel(src); got != want {
			t.Fatalf("sections=%d: HTMLParallel differs from sequential", sections)
		}
	}
}

func TestParallelRenderToWriter(t *testing.T) {
	src := bigDoc(100)
	base := mdflow.New()
	var b strings.Builder
	if err := base.RenderParallel(&b, src); err != nil {
		t.Fatal(err)
	}
	if b.String() != base.HTML(src) {
		t.Error("RenderParallel differs from sequential HTML")
	}
}

// A middleware chain cannot be partitioned safely, so the fan-out must decline
// rather than silently reorder or duplicate transformed events.
func TestParallelFallsBackWithPipeline(t *testing.T) {
	src := bigDoc(100)
	seq := mdflow.New().Transform(mdflow.ShiftHeadings(1))
	par := seq.Workers(8)
	if got, want := par.HTML(src), seq.HTML(src); got != want {
		t.Error("parallel parser with a pipeline diverged from the sequential one")
	}
	if !strings.Contains(par.HTML(src), "<h3>") {
		t.Error("middleware was skipped on the parallel parser")
	}
}

func TestWorkersIsImmutable(t *testing.T) {
	base := mdflow.New()
	_ = base.Workers(8)
	src := bigDoc(50)
	if got, want := base.HTML(src), base.Workers(1).HTML(src); got != want {
		t.Error("Workers mutated its receiver")
	}
}
