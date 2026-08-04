package bench

import (
	"runtime"
	"testing"

	"github.com/Wenrh2004/mdflow/all"
)

// Does fanning the inline phase out across cores actually pay?
//
// The claim under test is narrow: block parsing is inherently sequential, but
// inline parsing of a closed leaf is independent of every other leaf, so phase
// two can be distributed. Amdahl bounds the win by whatever fraction of the work
// is inline — measured at roughly 78% on this corpus — so the ceiling is around
// 4x no matter how many cores are thrown at it.
//
// These benchmarks report wall-clock *and* bytes allocated, because the fan-out
// buys speed with memory: it materialises every block event at once, where the
// sequential path streams them.

func BenchmarkSequentialVsParallel(b *testing.B) {
	for _, sz := range []struct {
		Name     string
		Sections int
	}{{"8KiB", 16}, {"32KiB", 64}, {"128KiB", 256}, {"512KiB", 1024}, {"2MiB", 4096}} {
		src := Doc(sz.Sections)
		seq := all.New()
		par := seq.Workers(0)

		b.Run(sz.Name+"/sequential", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink(seq.HTML(src))
			}
		})
		b.Run(sz.Name+"/parallel", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink(par.HTML(src))
			}
		})
	}
}

// How does the win scale with worker count? If it plateaus early, the extra
// goroutines are pure cost.
func BenchmarkWorkerScaling(b *testing.B) {
	src := Doc(1024) // ~512 KiB
	for _, n := range []int{1, 2, 4, 8, 14} {
		if n > runtime.GOMAXPROCS(0) {
			continue
		}
		p := all.New().Workers(n)
		b.Run("workers="+itoa(n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink(p.HTML(src))
			}
		})
	}
}

// The fan-out helps one big document on an idle machine. A server rendering many
// documents at once already saturates its cores with request-level parallelism,
// so the per-document fan-out has nothing left to win and only adds contention.
// This is the case that decides whether it should ever be a default.
func BenchmarkParallelUnderLoad(b *testing.B) {
	src := Doc(256) // ~128 KiB
	seq := all.New()
	par := seq.Workers(0)

	b.Run("sequential", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sink(seq.HTML(src))
			}
		})
	})
	b.Run("parallel", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sink(par.HTML(src))
			}
		})
	})
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
