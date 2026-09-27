package mdflow_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/iterx"
)

// corpus is a document exercising every code path the three input spellings
// share: containers, a fenced block and a range of inline markup. If string /
// []byte / context ever diverge, one of them renders this differently.
const corpus = `# Title

Some *emphasis*, **strong**, ` + "`code`" + ` and a [link](https://x.dev).

- one
- two

> quoted

` + "```go\nfunc f() {}\n```" + `

1. first
2. second

Autolinked <https://go.dev> and an image ![alt](/a.png).
`

func TestBytesMatchesString(t *testing.T) {
	p := mdflow.New()
	src := corpus
	b := []byte(corpus)

	if got, want := p.HTMLBytes(b), p.HTML(src); got != want {
		t.Errorf("HTMLBytes != HTML\n got: %q\nwant: %q", got, want)
	}

	var wb, ws bytes.Buffer
	if err := p.RenderBytes(&wb, b); err != nil {
		t.Fatalf("RenderBytes: %v", err)
	}
	if err := p.Render(&ws, src); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if wb.String() != ws.String() {
		t.Errorf("RenderBytes != Render\n got: %q\nwant: %q", wb.String(), ws.String())
	}

	if got, want := p.TextBytes(b), p.Text(src); got != want {
		t.Errorf("TextBytes != Text\n got: %q\nwant: %q", got, want)
	}

	gotH, wantH := p.HeadingsBytes(b), p.Headings(src)
	if len(gotH) != len(wantH) {
		t.Fatalf("HeadingsBytes len %d != Headings len %d", len(gotH), len(wantH))
	}
	for i := range gotH {
		if gotH[i] != wantH[i] {
			t.Errorf("heading %d: %+v != %+v", i, gotH[i], wantH[i])
		}
	}

	// The lazy sequences must agree event for event.
	gotE := slices.Collect(p.EventsBytes(b))
	wantE := slices.Collect(p.Events(src))
	if len(gotE) != len(wantE) {
		t.Fatalf("EventsBytes len %d != Events len %d", len(gotE), len(wantE))
	}
	for i := range gotE {
		if gotE[i] != wantE[i] {
			t.Errorf("event %d: %+v != %+v", i, gotE[i], wantE[i])
		}
	}

	gotB := slices.Collect(p.BlocksBytes(b))
	wantB := slices.Collect(p.Blocks(src))
	if len(gotB) != len(wantB) {
		t.Fatalf("BlocksBytes len %d != Blocks len %d", len(gotB), len(wantB))
	}
}

func TestBytesEmpty(t *testing.T) {
	p := mdflow.New()
	if got := p.HTMLBytes(nil); got != "" {
		t.Errorf("HTMLBytes(nil) = %q, want empty", got)
	}
	if got := p.HTMLBytes([]byte{}); got != "" {
		t.Errorf("HTMLBytes([]byte{}) = %q, want empty", got)
	}
}

func TestContextMatchesPlain(t *testing.T) {
	p := mdflow.New()
	ctx := context.Background()

	got, err := p.HTMLContext(ctx, corpus)
	if err != nil {
		t.Fatalf("HTMLContext: %v", err)
	}
	if want := p.HTML(corpus); got != want {
		t.Errorf("HTMLContext != HTML\n got: %q\nwant: %q", got, want)
	}

	var w bytes.Buffer
	if err := p.RenderContext(ctx, &w, corpus); err != nil {
		t.Fatalf("RenderContext: %v", err)
	}
	if want := p.HTML(corpus); w.String() != want {
		t.Errorf("RenderContext != HTML\n got: %q\nwant: %q", w.String(), want)
	}

	txt, err := p.TextContext(ctx, corpus)
	if err != nil {
		t.Fatalf("TextContext: %v", err)
	}
	if want := p.Text(corpus); txt != want {
		t.Errorf("TextContext != Text\n got: %q\nwant: %q", txt, want)
	}

	hs, err := p.HeadingsContext(ctx, corpus)
	if err != nil {
		t.Fatalf("HeadingsContext: %v", err)
	}
	if want := p.Headings(corpus); len(hs) != len(want) {
		t.Errorf("HeadingsContext len %d != Headings len %d", len(hs), len(want))
	}

	got2 := slices.Collect(p.EventsContext(ctx, corpus))
	want2 := slices.Collect(p.Events(corpus))
	if len(got2) != len(want2) {
		t.Fatalf("EventsContext len %d != Events len %d", len(got2), len(want2))
	}
}

// bigDoc (shared with parallel_test.go) builds a document large enough that a
// mid-parse cancel has lines left to skip, so cancellation cuts work short.

func TestContextCancelledUpFront(t *testing.T) {
	p := mdflow.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done before the first line

	src := bigDoc(5000)
	got, err := p.HTMLContext(ctx, src)
	if err == nil {
		t.Fatal("HTMLContext returned nil error for a cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	// A cancel before the first check yields no output, and must never render
	// the whole document.
	if full := p.HTML(src); got == full {
		t.Error("cancelled HTMLContext rendered the whole document")
	}
}

func TestContextCancelledMidParse(t *testing.T) {
	p := mdflow.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel from inside the stream once enough events have flowed to prove the
	// parse had started but not finished.
	src := bigDoc(20000)
	n := 0
	seen := 0
	for range p.EventsContext(ctx, src) {
		seen++
		if n++; n == 500 {
			cancel()
		}
	}
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("ctx.Err() = %v, want context.Canceled", err)
	}
	full := iterx.Count(p.Events(src))
	if seen >= full {
		t.Errorf("cancelled stream yielded %d events; full document has %d — cancel did not stop it", seen, full)
	}
}

func TestContextCancelParallel(t *testing.T) {
	p := mdflow.New().Workers(4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := bigDoc(20000) // well over parallelMinBytes
	got, err := p.HTMLContext(ctx, src)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got != "" {
		t.Errorf("cancelled parallel HTMLContext returned %d bytes, want none", len(got))
	}
}
