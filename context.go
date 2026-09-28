package mdflow

import (
	"bufio"
	"context"
	"io"
	"iter"
	"strings"

	"github.com/Wenrh2004/mdflow/internal/drive"

	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// The context-aware twins of the terminal operations, for the one caller a
// streaming Markdown library exists to serve: an LLM feeding tokens into a
// render loop that a user can cancel. A plain parse of a small document is over
// in microseconds and needs none of this; a parse of a large or slowly arriving
// one wants a way out that does not involve tearing down the goroutine.
//
// Cancellation is cooperative and coarse-grained on purpose. Checking
// ctx.Err() per line would put an atomic load on the hottest loop in the
// library to serve a case that only matters across many thousands of lines, so
// the line-driven paths check once every ctxCheckLines lines. A cancelled parse
// stops promptly relative to a human's patience, which is the only clock that
// matters here.
//
// A cancelled operation returns whatever it had rendered so far together with
// ctx.Err(): the string and Render twins may have already written a prefix, and
// reporting the prefix rather than discarding it is what lets a UI keep the
// partial document it was showing.

// ctxCheckLines is how often the line-driven paths consult ctx.Err(). It is a
// power of two so the check is a mask, and large enough that the check is lost
// in the noise of the work between two consultations.
const ctxCheckLines = 1 << 10

// HTMLContext is [Parser.HTML] with cancellation. On cancellation it returns
// the HTML rendered up to that point together with ctx.Err().
func (p *Parser) HTMLContext(ctx context.Context, src string) (string, error) {
	var b strings.Builder
	b.Grow(len(src) + len(src)/2)
	err := p.renderToContext(ctx, &b, src)
	return b.String(), err
}

// RenderContext is [Parser.Render] with cancellation.
func (p *Parser) RenderContext(ctx context.Context, w io.Writer, src string) error {
	if bw, ok := w.(renderer.Writer); ok {
		return p.renderToContext(ctx, bw, src)
	}
	bw := bufio.NewWriter(w)
	if err := p.renderToContext(ctx, bw, src); err != nil {
		return err
	}
	return bw.Flush()
}

// TextContext is [Parser.Text] with cancellation.
func (p *Parser) TextContext(ctx context.Context, src string) (string, error) {
	out := collectText(p.EventsContext(ctx, src))
	return out, ctx.Err()
}

// HeadingsContext is [Parser.Headings] with cancellation.
func (p *Parser) HeadingsContext(ctx context.Context, src string) ([]Heading, error) {
	out := collectHeadings(p.EventsContext(ctx, src))
	return out, ctx.Err()
}

// EventsContext is [Parser.Events] with cancellation: once ctx is done the
// sequence stops yielding, so a range over it ends cooperatively. It carries no
// error itself — a consumer that needs the reason reads ctx.Err() after the
// range, the way TextContext and HeadingsContext do.
func (p *Parser) EventsContext(ctx context.Context, src string) iter.Seq[Event] {
	raw := p.rawEventsContext(ctx, src)
	if p.chain == nil {
		return raw
	}
	return p.chain(raw)
}

// renderToContext is [Parser.renderTo] with cancellation, mirroring its two
// paths so the context spelling makes the same fast/slow choices.
func (p *Parser) renderToContext(ctx context.Context, w renderer.Writer, src string) error {
	switch {
	case p.chain == nil:
		return p.renderDirectContext(ctx, w, src)
	default:
		p.renderEvents(w, p.EventsContext(ctx, src))
		return ctx.Err()
	}
}

// renderDirectContext is [Parser.renderDirect] with a periodic cancellation
// check on the line loop.
func (p *Parser) renderDirectContext(ctx context.Context, w renderer.Writer, src string) error {
	bp := p.borrow()
	defer p.release(bp)
	driver := newBatchDriver(bp, src)
	defer driver.Release()
	render := func(ev token.BlockEvent, inlines []token.Inline) bool {
		p.writeDocumentEvent(w, ev, inlines)
		return true
	}
	var (
		n    int
		cerr error
	)
	drive.EachLine(src, func(line string) bool {
		if n&(ctxCheckLines-1) == 0 {
			if err := ctx.Err(); err != nil {
				cerr = err
				return false
			}
		}
		n++
		return driver.FeedLine(line, render)
	})
	if cerr != nil {
		return cerr
	}
	driver.Close(render)
	return ctx.Err()
}

// rawEventsContext is [Parser.rawEvents] that stops feeding lines once ctx is
// done, so the sequence it drives ends cooperatively.
func (p *Parser) rawEventsContext(ctx context.Context, src string) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		bp := p.borrow()
		defer p.release(bp)
		driver := newBatchDriver(bp, src)
		defer driver.Release()
		emit := func(ev token.BlockEvent, inlines []token.Inline) bool {
			return p.emitDocumentEvent(ev, inlines, yield)
		}
		ok := true
		n := 0
		drive.EachLine(src, func(line string) bool {
			if n&(ctxCheckLines-1) == 0 && ctx.Err() != nil {
				ok = false
				return false
			}
			n++
			ok = driver.FeedLine(line, emit)
			return ok
		})
		if ok {
			driver.Close(emit)
		}
	}
}
