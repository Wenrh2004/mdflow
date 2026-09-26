package bench

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/usememos/gomark"
	gomarkhtml "github.com/usememos/gomark/renderer/html"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
)

// The dimensions beyond throughput and allocations on which the two libraries
// are compared. Throughput lives in bench_test.go; everything here is about
// what a production user feels besides raw speed:
//
//	ChunkLatency      the per-chunk cost a streaming chat UI pays, p50 and p99
//	Pathological      adversarial input: the worst case, not the average
//	Construct         setting a parser up
//	TestConformance   CommonMark 0.31.2 examples rendered correctly
//	TestSafety        attacker HTML reaching the output unescaped
//	TestBinarySize    what importing the library adds to a program

// BenchmarkChunkLatency streams a document in 64-byte chunks and reports the
// per-chunk latency distribution of refreshing the view: mdflow's
// Feed+Provisional versus the re-parse a library without incremental state is
// limited to. p99 is what a user perceives as a stutter.
func BenchmarkChunkLatency(b *testing.B) {
	src := Doc(10)
	run := func(b *testing.B, step func(prefix, chunk string) string, reset func()) {
		var lat []time.Duration
		for b.Loop() {
			reset()
			for off := 0; off < len(src); off += chunkSize {
				end := min(off+chunkSize, len(src))
				t := time.Now()
				sink(step(src[:end], src[off:end]))
				lat = append(lat, time.Since(t))
			}
		}
		slices.Sort(lat)
		b.ReportMetric(float64(lat[len(lat)/2].Nanoseconds()), "p50-ns/chunk")
		b.ReportMetric(float64(lat[len(lat)*99/100].Nanoseconds()), "p99-ns/chunk")
	}
	b.Run("mdflow", func(b *testing.B) {
		var s *mdflow.Stream
		run(b, func(_, chunk string) string {
			return s.Feed(chunk) + s.Provisional()
		}, func() { s = parser.Stream() })
	})
	b.Run("gomark", func(b *testing.B) {
		run(b, func(prefix, _ string) string { return renderGomark(prefix) }, func() {})
	})
}

// pathological inputs are the shapes that historically drive Markdown parsers
// quadratic. Sizes are kept small enough that a superlinear parser still
// finishes; the point is the ratio, not the absolute time.
var pathological = []struct{ name, src string }{
	{"nested-lists", strings.Repeat("- ", 1_000) + "x\n"},
	{"nested-quotes", strings.Repeat(">", 1_000) + " x\n"},
	{"open-brackets", strings.Repeat("[", 1_000) + "\n"},
	{"open-emphasis", strings.Repeat("*a ", 1_000) + "\n"},
	{"backticks", strings.Repeat("`a", 1_000) + "\n"},
	{"wiki-links", strings.Repeat("[[a", 1_000) + "\n"},
	{"many-lines", strings.Repeat("word\n", 1_000)},
}

func BenchmarkPathological(b *testing.B) {
	for _, p := range pathological {
		b.Run(p.name+"/mdflow", func(b *testing.B) {
			b.SetBytes(int64(len(p.src)))
			b.ReportAllocs()
			for b.Loop() {
				sink(parser.HTML(p.src))
			}
		})
		b.Run(p.name+"/gomark", func(b *testing.B) {
			b.SetBytes(int64(len(p.src)))
			b.ReportAllocs()
			for b.Loop() {
				sink(renderGomark(p.src))
			}
		})
	}
}

// BenchmarkConstruct is the cost of making a ready parser. gomark has no
// parser value to build — each Parse starts from nothing — so its row is the
// smallest document it can render, the fixed cost it pays per call instead.
func BenchmarkConstruct(b *testing.B) {
	b.Run("mdflow", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink(all.New().HTML("x"))
		}
	})
	b.Run("gomark", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink(renderGomark("x"))
		}
	})
}

// TestConformance renders every CommonMark 0.31.2 spec example with both
// libraries and reports how many each gets right. Output is compared after
// normalizeHTML, the same leniency the spec's own test runner applies, so
// cosmetic differences — newlines between tags, `<br />` versus `<br>` — do not
// count against gomark; only different structure or text does.
func TestConformance(t *testing.T) {
	raw, err := os.ReadFile("../testdata/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples []struct{ Markdown, HTML string }
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	trusted := mdflow.New(rawhtml.WithUnsafeHTML())
	var md, gm int
	for _, e := range examples {
		want := normalizeHTML(e.HTML)
		if normalizeHTML(trusted.HTML(e.Markdown)) == want {
			md++
		}
		if out, ok := tryGomark(e.Markdown); ok && normalizeHTML(out) == want {
			gm++
		}
	}
	t.Logf("CommonMark 0.31.2 examples passed: mdflow %d/%d, gomark %d/%d", md, len(examples), gm, len(examples))
	if md <= gm {
		t.Errorf("mdflow passes %d examples, gomark %d; mdflow must lead", md, gm)
	}
}

// TestSafety feeds attacker-shaped input through both libraries' default
// profiles and counts outputs that still carry live markup.
func TestSafety(t *testing.T) {
	attacks := []string{
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		"<iframe src=//evil></iframe>",
		"a <svg onload=alert(1)> b",
		"<style>body{display:none}</style>",
	}
	live := func(out string) bool {
		out = strings.ToLower(out)
		for _, tag := range []string{"<script", "<img src=x", "<iframe", "<svg", "<style"} {
			if strings.Contains(out, tag) {
				return true
			}
		}
		return false
	}
	var md, gm int
	for _, a := range attacks {
		if live(parser.HTML(a)) {
			md++
		}
		if out, ok := tryGomark(a); ok && live(out) {
			gm++
		}
	}
	t.Logf("attacks passed through unescaped: mdflow %d/%d, gomark %d/%d", md, len(attacks), gm, len(attacks))
	if md != 0 {
		t.Errorf("mdflow let %d attacks through its default profile", md)
	}
}

// TestBinarySize builds the same one-line program against each library (see
// cmd/size-*) and reports what the import adds to a stripped binary.
func TestBinarySize(t *testing.T) {
	if testing.Short() {
		t.Skip("builds four binaries")
	}
	sizes := map[string]int64{}
	for _, name := range []string{"baseline", "mdflow", "mdflow-all", "gomark"} {
		bin := filepath.Join(t.TempDir(), name)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", bin, "./cmd/size-"+name)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, out)
		}
		fi, err := os.Stat(bin)
		if err != nil {
			t.Fatal(err)
		}
		sizes[name] = fi.Size()
	}
	for _, name := range []string{"mdflow", "mdflow-all", "gomark"} {
		t.Logf("%-10s adds %5d KiB to a stripped binary", name, (sizes[name]-sizes["baseline"])/1024)
	}
}

// tryGomark renders with gomark, treating a panic or error as a failed render.
func tryGomark(src string) (out string, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	doc, err := gomark.Parse(src)
	if err != nil {
		return "", false
	}
	return gomarkhtml.NewHTMLRenderer().RenderDocument(doc), true
}

// normalizeHTML removes differences a browser ignores: whitespace between tags,
// and the XHTML self-closing slash.
func normalizeHTML(s string) string {
	s = strings.ReplaceAll(s, " />", ">")
	s = strings.ReplaceAll(s, "/>", ">")
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimSpace(line))
	}
	s = b.String()
	for {
		t := strings.ReplaceAll(strings.ReplaceAll(s, "> <", "><"), ">  <", "><")
		if t == s {
			return s
		}
		s = t
	}
}
