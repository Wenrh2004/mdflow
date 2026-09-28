package bench

import (
	"io"
	"strings"
	"testing"

	"github.com/usememos/gomark"
	gomarkhtml "github.com/usememos/gomark/renderer/html"

	"github.com/Wenrh2004/mdflow/all"
)

// Comparative benchmarks: mdflow vs github.com/usememos/gomark.
//
// Every pair below runs the *same* input through both libraries doing the
// *same* job, so the numbers compare architectures rather than convenience
// wrappers:
//
//	MarkdownToHTML   markdown in, HTML string out — the headline workload
//	ParseOnly        structure only, no rendering
//	Streaming        an LLM-style producer emitting 64-byte chunks
//	TextExtract      strip markup, keep the words (search indexing, embeddings)
//	Prose            markup-light input, exercising the fast path
//
// Run: go test -bench . -benchmem ./...

var parser = all.New()

// renderGomark is gomark's markdown -> HTML path.
func renderGomark(src string) string {
	doc, err := gomark.Parse(src)
	if err != nil {
		panic(err)
	}
	return gomarkhtml.NewHTMLRenderer().RenderDocument(doc)
}

func BenchmarkMarkdownToHTML(b *testing.B) {
	for _, sz := range Sizes {
		src := Doc(sz.Sections)
		b.Run(sz.Name+"/mdflow", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink(parser.HTML(src))
			}
		})
		b.Run(sz.Name+"/gomark", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink(renderGomark(src))
			}
		})
	}
}

// Rendering straight into an io.Writer is mdflow-only: it never builds a
// document-sized string. gomark's renderer returns a string by construction, so
// the closest equivalent is measured for reference.
func BenchmarkRenderToWriter(b *testing.B) {
	src := Doc(50)
	b.Run("mdflow", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := parser.Render(io.Discard, src); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("gomark", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = io.WriteString(io.Discard, renderGomark(src))
		}
	})
}

func BenchmarkParseOnly(b *testing.B) {
	src := Doc(50)
	b.Run("mdflow", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			n := 0
			for range parser.Blocks(src) {
				n++
			}
			sinkInt(n)
		}
	})
	b.Run("gomark", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			doc, err := gomark.Parse(src)
			if err != nil {
				b.Fatal(err)
			}
			sinkInt(len(doc.Children))
		}
	})
}

const chunkSize = 64 // roughly one LLM token batch

// Streaming is the workload the architectures actually differ on. mdflow feeds
// the incremental state machine, which never revisits a closed block: O(n).
// A parser without incremental state must re-parse the whole document on every
// chunk to refresh the view: O(n^2).
func BenchmarkStreaming(b *testing.B) {
	for _, sz := range []struct {
		Name     string
		Sections int
	}{{"small", 5}, {"medium", 50}} {
		src := Doc(sz.Sections)
		b.Run(sz.Name+"/mdflow-incremental", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s := parser.Stream()
				for off := 0; off < len(src); off += chunkSize {
					sink(s.Feed(src[off:min(off+chunkSize, len(src))]))
				}
				sink(s.Finish())
			}
		})
		// gomark has no incremental mode, so a streaming UI must re-parse. Only
		// the small document is measured: on the medium one the re-parse
		// strategy compounds gomark's own superlinear parse cost into minutes
		// per iteration, which says nothing new and makes the suite unusable.
		if sz.Sections <= 5 {
			b.Run(sz.Name+"/gomark-reparse", func(b *testing.B) {
				b.SetBytes(int64(len(src)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for off := 0; off < len(src); off += chunkSize {
						sink(renderGomark(src[:min(off+chunkSize, len(src))]))
					}
				}
			})
		}
		// The same naive strategy on mdflow, to separate the architectural win
		// from the raw single-pass speed win.
		b.Run(sz.Name+"/mdflow-reparse", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for off := 0; off < len(src); off += chunkSize {
					sink(parser.HTML(src[:min(off+chunkSize, len(src))]))
				}
			}
		})
	}
}

func BenchmarkTextExtract(b *testing.B) {
	src := Doc(50)
	b.Run("mdflow", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink(parser.Text(src))
		}
	})
	b.Run("gomark", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			doc, err := gomark.Parse(src)
			if err != nil {
				b.Fatal(err)
			}
			sink(gomark.Restore(doc))
		}
	})
}

func BenchmarkProse(b *testing.B) {
	src := ProseDoc(200)
	b.Run("mdflow", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink(parser.HTML(src))
		}
	})
	b.Run("gomark", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink(renderGomark(src))
		}
	})
}

// Parsers get shared across goroutines in any real server; this checks the
// pooled state scales rather than serialising.
func BenchmarkParallel(b *testing.B) {
	src := Doc(50)
	b.Run("mdflow", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sink(parser.HTML(src))
			}
		})
	})
	b.Run("gomark", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sink(renderGomark(src))
			}
		})
	})
}

// TestOutputsAreComparable guards the benchmark's honesty: if one library
// silently dropped a construct it would look artificially fast. The two emit
// different HTML conventions, so this asserts that the *content* survives both.
func TestOutputsAreComparable(t *testing.T) {
	src := Doc(2)
	mine, theirs := parser.HTML(src), renderGomark(src)
	for _, want := range []string{
		"Section 0", "emphasis", "strong text", "inline code",
		"https://example.com/page/0", "func handler0", "first item",
		"blockquote with", "alpha",
		"shipped item 0", "section0", "highlighted", "struck", "a_0 + b",
	} {
		if !strings.Contains(mine, want) {
			t.Errorf("mdflow output is missing %q", want)
		}
		if !strings.Contains(theirs, want) {
			t.Errorf("gomark output is missing %q", want)
		}
	}
	for _, tag := range []string{
		"<h2>", "<p>", "<pre>", "<code", "<ul>", "<li>", "<blockquote>", "<table>",
		"<em>", "<strong>", "<a href=", "<mark>", "<del>", "checkbox",
	} {
		if !strings.Contains(mine, tag) {
			t.Errorf("mdflow output is missing tag %q", tag)
		}
	}
}

// Package-level sinks keep the compiler from eliminating the work.
var (
	strSink string
	intSink int
)

func sink(s string) { strSink = s }
func sinkInt(n int) { intSink = n }
