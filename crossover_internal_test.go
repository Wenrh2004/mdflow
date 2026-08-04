package mdflow

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestParallelCrossover guards the only claim that justifies shipping the
// fan-out at all: above parallelMinBytes it is meaningfully faster than the
// sequential path. If a future kernel optimisation shrinks the inline share of
// the work, that claim stops holding and this test says so — at which point the
// honest move is to delete Workers rather than keep an option that does nothing.
//
// It deliberately does *not* assert where the crossover sits. Near break-even
// the ratio is dominated by scheduler noise — repeated runs put it anywhere
// between 3 KiB and 30 KiB — so a threshold derived from one run would be
// fiction. parallelMinBytes comes from the far more stable `go test -bench`
// means recorded in the Workers doc comment; this test only checks the
// conclusion those numbers support.
func TestParallelCrossover(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive")
	}
	if raceEnabled {
		// The race detector serialises the fan-out's goroutines, so the speedup
		// this guard measures collapses to noise under it. The claim is about
		// production timing, which -race does not model.
		t.Skip("race detector distorts the parallel timing this guard measures")
	}
	seq := New()
	par := seq.Workers(0)

	// Mean, not best-of-N. Best-of-N systematically flatters the fan-out: it
	// samples the run where all cores happened to be free and every cache was
	// warm, which is exactly the condition a real server does not enjoy. The
	// mean is what `go test -bench` reports and what the threshold is set from.
	mean := func(f func(string) string, src string) time.Duration {
		const runs = 50
		for i := 0; i < 20; i++ {
			_ = f(src) // warm caches and the state pool
		}
		t0 := time.Now()
		for i := 0; i < runs; i++ {
			_ = f(src)
		}
		return time.Since(t0) / runs
	}

	var (
		report    strings.Builder
		bigRatio  float64
		bigTested bool
	)
	for _, sections := range []int{4, 16, 64, 256, 1024} {
		src := crossoverDoc(sections)
		ratio := float64(mean(seq.HTML, src)) / float64(mean(par.renderParallelForced, src))
		fmt.Fprintf(&report, "  %8d bytes  %.2fx\n", len(src), ratio)
		// Judge on a document several times the threshold, where the signal is
		// well clear of the noise floor.
		if len(src) >= 4*parallelMinBytes {
			bigRatio, bigTested = ratio, true
		}
	}
	t.Logf("fan-out speedup by input size:\n%s", report.String())

	if !bigTested {
		t.Fatal("corpus never reached 4x parallelMinBytes; the guard measured nothing")
	}
	// The guard is set from measurement, not from the benchmark table: sampled
	// 21 times at 240 KiB it ranged 1.42x-1.63x, median 1.50x. A threshold of
	// 1.5x therefore tripped on roughly a quarter of runs — it was sitting on
	// the median, not below the floor.
	//
	// The ratio fell (it was ~1.69x) because the *sequential* path got faster
	// when Tag became an integer and Leaf and BlockEvent shrank; the fan-out
	// win compressed accordingly. That is an improvement wearing a regression's
	// clothes, and the reason to re-derive this number rather than relax it
	// until the test passes.
	//
	// 1.25x sits about 12% below the observed floor — this ratio compares two
	// measurements from the same run, so it is far steadier than either timing
	// alone — while staying well clear of 1.0x, where parallelism genuinely
	// stops paying and this test is meant to fire.
	if bigRatio < 1.25 {
		t.Errorf("fan-out gave only %.2fx on a document 4x the threshold; "+
			"Workers no longer earns its complexity and should be reconsidered\n%s",
			bigRatio, report.String())
	}
}

// crossoverDoc mirrors the dense mixed corpus in bench/corpus.go, restricted to
// the CommonMark core the module ships. The threshold has to hold for the
// *worst* case, and density is what decides the crossover: the fan-out
// distributes inline work, so a markup-heavy document breaks even sooner than
// plain prose.
func crossoverDoc(sections int) string {
	var b strings.Builder
	for i := 0; i < sections; i++ {
		s := strconv.Itoa(i)
		b.WriteString("## Section " + s + "\n\n")
		b.WriteString("Prose with *emphasis*, **strong**, `code`, [a link](https://x.dev/" + s +
			") and an autolink <https://go.dev/" + s + ">.\n\n")
		b.WriteString("```go\nfunc f" + s + "() {}\n```\n\n- one\n- two\n\n> quoted\n\n")
		b.WriteString("Dense inline: *a* **b** `c` *d* **e** `f` with [more](https://x.dev/" + s + ").\n\n")
		b.WriteString("1. first\n2. second\n\n")
	}
	return b.String()
}
