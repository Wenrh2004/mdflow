package bench

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/gfm"
)

// goldmark is the de-facto fast Go Markdown library, measured as a reference
// point for throughput: a comparison against gomark alone would say more about
// gomark's superlinear parser than about mdflow's speed. Both sides run
// CommonMark plus GFM tables, strikethrough and task lists in XHTML output.
var (
	goldmarkGFM = goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList),
		goldmark.WithRendererOptions(gmhtml.WithXHTML()),
	)
	mdflowGFM = mdflow.New(mdflow.WithExtensions(gfm.GFM))
)

func BenchmarkGoldmarkReference(b *testing.B) {
	docs := []struct{ name, src string }{
		{"mixed-200KB", Doc(500)},
		{"prose-46KB", ProseDoc(150)},
	}
	for _, d := range docs {
		b.Run(d.name+"/mdflow", func(b *testing.B) {
			b.SetBytes(int64(len(d.src)))
			b.ReportAllocs()
			for b.Loop() {
				sink(mdflowGFM.HTML(d.src))
			}
		})
		b.Run(d.name+"/goldmark", func(b *testing.B) {
			b.SetBytes(int64(len(d.src)))
			b.ReportAllocs()
			src := []byte(d.src)
			for b.Loop() {
				var buf bytes.Buffer
				_ = goldmarkGFM.Convert(src, &buf)
				sink(buf.String())
			}
		})
	}
}
