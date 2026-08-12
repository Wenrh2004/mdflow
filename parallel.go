package mdflow

import (
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/Wenrh2004/mdflow/renderer"
)

// Parallel rendering: fan the *inline* phase out across cores.
//
// CommonMark's appendix A splits parsing into two phases, and says something
// useful about their dependencies: block structure is strictly sequential (a
// line's meaning depends on the container stack above it), while inline parsing
// after reference definitions are sealed depends only on the leaf and that
// immutable resolver. So phase two is embarrassingly parallel, and phase one is
// not.
//
// Granularity is the whole game. Handing each block to a worker — one goroutine
// hop per block — costs more in scheduler traffic than a block's few
// microseconds of work; measured that way, the fan-out runs an order of
// magnitude *slower* than staying on one core. This implementation therefore
// partitions the block-event stream into a handful of large contiguous ranges,
// one per worker, so a single handoff amortises over thousands of blocks.
//
// The cost is memory: the block events for the whole document must exist at once
// (the sequential path streams them and keeps only the top of the stack). See
// Parser.Workers for the measured trade-off and when it is worth paying.

// parallelMinBytes is the input size below which fan-out is not worth its setup.
//
// Measured, not guessed, and re-derived by scaling *one* document shape across
// sizes so that density is held constant and only size varies — nine rounds per
// size, sequential and fan-out interleaved, median ratio (Apple M4 Pro, 14
// cores, dense mixed corpus):
//
//	0.9 KiB  0.53x        7.3 KiB  1.02x       29 KiB  1.10x
//	3.5 KiB  0.88x         14 KiB  1.17x       58 KiB  1.13x
//	5.3 KiB  0.91x         21 KiB  1.19x      234 KiB  1.63x
//
// Break-even is ~7.3 KiB. It used to be ~28 KiB, and it moved because the
// fan-out path materialises the whole document's []token.BlockEvent: shrinking
// BlockEvent from 96 to 72 bytes cut a quarter of that buffer's memory traffic,
// which helps fan-out more than it helps the sequential path.
//
// The threshold sits at 16 KiB rather than at break-even. Above it the win is
// consistent (1.16x-1.20x at 16-22 KiB), and the ~2x headroom is margin for
// machines with fewer cores than the one these numbers come from, where the
// same setup cost buys less. The price is leaving the modest 7-16 KiB wins on
// the table, which is the right way round: Workers is opt-in, so it must never
// make a document slower.
const parallelMinBytes = 16 << 10

// Workers derives a Parser whose HTML and Render fan the inline phase out across
// n goroutines. n <= 1 restores sequential behaviour; n <= 0 means
// runtime.GOMAXPROCS(0).
//
// Output is byte-identical to the sequential path: rendering a block event
// depends only on that event, so partitioning the stream cannot change the
// result. Two conditions silently fall back to sequential, because neither can
// be partitioned safely:
//
//   - inputs below parallelMinBytes, where fan-out costs more than it saves;
//   - a parser with a middleware chain installed, since a Middleware may carry
//     state across the whole event stream.
//
// Measured on an Apple M4 Pro (14 cores), dense mixed corpus, one document
// shape scaled by section count:
//
//	 14 KiB   1.15x    +5% memory
//	 43 KiB   1.04x    +7%
//	175 KiB   1.45x   +22%
//	511 KiB   1.89x   +17%
//	  2 MiB   1.98x   +23%
//
// The win survives full core saturation: under RunParallel at 175 KiB it is
// still 1.68x, because the smaller per-worker output buffers are friendlier to
// the allocator than one document-sized one.
//
// The ceiling is Amdahl's — only the inline phase distributes — so returns
// flatten a few workers in.
//
// These figures are lower than they once were, and so is the memory penalty:
// both moved when Leaf and BlockEvent shrank, which sped the sequential path up
// and made the fan-out's whole-document event buffer cheaper at the same time.
//
// This is off by default because it is not free: it holds the whole document's
// block events at once where the sequential path streams them, it costs memory,
// and it cannot be combined with a middleware chain.
func (p *Parser) Workers(n int) *Parser {
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}
	return newParser(p.cfg, p.pipeline, n)
}

// parallelEligible reports whether this call can take the fan-out path.
func (p *Parser) parallelEligible(src string) bool {
	return p.workers > 1 && p.chain == nil && len(src) >= parallelMinBytes
}

// renderParallel renders events in n contiguous partitions and concatenates the
// per-worker buffers in order.
func (p *Parser) renderParallel(w renderer.Writer, src string) {
	bp := p.borrow()
	// Held until every worker is done: the event slice is bp's backing array,
	// so releasing early would hand it to another goroutine mid-render.
	defer p.release(bp)

	events := bp.CollectAll(src)
	bp.SealReferences()
	n := min(p.workers, len(events))
	if n <= 1 {
		for i := range events {
			p.writeFinalEvent(w, bp, events[i])
		}
		return
	}

	bufs := make([]strings.Builder, n)
	per := (len(events) + n - 1) / n
	// Each worker's output is roughly its share of the final HTML; reserving up
	// front keeps the builders from doubling repeatedly under contention.
	reserve := len(src)/n + len(src)/(2*n) + 64

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		lo := i * per
		if lo >= len(events) {
			break
		}
		hi := min(lo+per, len(events))
		wg.Add(1)
		go func(i, lo, hi int) {
			defer wg.Done()
			b := &bufs[i]
			b.Grow(reserve)
			for j := lo; j < hi; j++ {
				p.writeFinalEvent(b, bp, events[j])
			}
		}(i, lo, hi)
	}
	wg.Wait()

	for i := range bufs {
		w.WriteString(bufs[i].String())
	}
}

// HTMLParallel renders src using the fan-out path regardless of the configured
// worker count, using runtime.GOMAXPROCS(0) goroutines. It is the explicit form
// of Workers(0).HTML; the size and pipeline fallbacks still apply.
func (p *Parser) HTMLParallel(src string) string {
	return p.Workers(0).HTML(src)
}

// RenderParallel is HTMLParallel streaming into w.
func (p *Parser) RenderParallel(w io.Writer, src string) error {
	return p.Workers(0).Render(w, src)
}

// renderParallelForced runs the fan-out path ignoring the size threshold. It
// exists so TestParallelCrossover can locate the crossover empirically rather
// than the threshold being a guess.
func (p *Parser) renderParallelForced(src string) string {
	var b strings.Builder
	b.Grow(len(src) + len(src)/2)
	p.renderParallel(&b, src)
	return b.String()
}
