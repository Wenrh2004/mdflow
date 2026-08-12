package mdflow_test

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/token"
)

func TestRenderHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"atx heading", "# Title\n", "<h1>Title</h1>\n"},
		{"closed atx", "## Title ##\n", "<h2>Title</h2>\n"},
		{"setext h1", "Title\n=====\n", "<h1>Title</h1>\n"},
		{"setext h2", "Title\n-----\n", "<h2>Title</h2>\n"},
		{"paragraph", "hello world\n", "<p>hello world</p>\n"},
		{"emphasis", "*a* **b**\n", "<p><em>a</em> <strong>b</strong></p>\n"},
		{"nested emphasis", "**a *b* c**\n", "<p><strong>a <em>b</em> c</strong></p>\n"},
		{"code span", "use `x + y` here\n", "<p>use <code>x + y</code> here</p>\n"},
		{"escape", `\*not emph\*` + "\n", "<p>*not emph*</p>\n"},
		{"link", "[go](https://go.dev)\n", `<p><a href="https://go.dev">go</a></p>` + "\n"},
		{"link title", `[go](https://go.dev "Go")` + "\n", `<p><a href="https://go.dev" title="Go">go</a></p>` + "\n"},
		{"image", "![alt](/a.png)\n", `<p><img src="/a.png" alt="alt" /></p>` + "\n"},
		{"autolink", "<https://go.dev>\n", `<p><a href="https://go.dev">https://go.dev</a></p>` + "\n"},
		{"email autolink", "<a@b.com>\n", `<p><a href="mailto:a@b.com">a@b.com</a></p>` + "\n"},
		{"thematic break", "---\n", "<hr />\n"},
		{"escaping", "a < b & c\n", "<p>a &lt; b &amp; c</p>\n"},
		{"hard break backslash", "a\\\nb\n", "<p>a<br />\nb</p>\n"},
		{"hard break spaces", "a  \nb\n", "<p>a<br />\nb</p>\n"},
		{
			"fenced code",
			"```go\nfunc f() {}\n```\n",
			"<pre><code class=\"language-go\">func f() {}\n</code></pre>\n",
		},
		{
			"code is literal",
			"```\n*not* <em>\n```\n",
			"<pre><code>*not* &lt;em&gt;\n</code></pre>\n",
		},
		{
			"unordered list",
			"- a\n- b\n",
			"<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n",
		},
		{
			"ordered list with start",
			"3. a\n4. b\n",
			"<ol start=\"3\">\n<li>a</li>\n<li>b</li>\n</ol>\n",
		},
		{
			"GFM task marker is CommonMark text",
			"- [ ] todo\n- [x] done\n",
			"<ul>\n<li>[ ] todo</li>\n<li>[x] done</li>\n</ul>\n",
		},
		{
			"blockquote",
			"> quoted\n",
			"<blockquote>\n<p>quoted</p>\n</blockquote>\n",
		},
		{
			"nested blockquote list",
			"> - a\n",
			"<blockquote>\n<ul>\n<li>a</li>\n</ul>\n</blockquote>\n",
		},
	}

	p := mdflow.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.HTML(tc.in); got != tc.want {
				t.Errorf("HTML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCRLFEqualsLF(t *testing.T) {
	src := "# Title\n\n- a\n- b\n\n> q\n"
	p := mdflow.New()
	if got, want := p.HTML(strings.ReplaceAll(src, "\n", "\r\n")), p.HTML(src); got != want {
		t.Errorf("CRLF differs from LF:\n got: %q\nwant: %q", got, want)
	}
}

func TestNoTrailingNewline(t *testing.T) {
	p := mdflow.New()
	if got, want := p.HTML("# Title"), "<h1>Title</h1>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// The whole point of the streaming design is that chunk boundaries are
// invisible. Feeding one byte at a time must produce byte-identical output to
// parsing the document in one go.
func TestStreamMatchesBatch(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"Some *emphasis* and `code` and a [link](https://go.dev).",
		"",
		"```go",
		"func main() {}",
		"```",
		"",
		"- one",
		"- two",
		"",
		"> quoted **text**",
		"",
		"final paragraph",
	}, "\n") + "\n"

	p := mdflow.New()
	want := p.HTML(src)

	for _, chunk := range []int{1, 2, 3, 7, 16, 64, 4096} {
		var got strings.Builder
		s := p.Stream()
		for i := 0; i < len(src); i += chunk {
			got.WriteString(s.Feed(src[i:min(i+chunk, len(src))]))
		}
		got.WriteString(s.Close())
		if got.String() != want {
			t.Errorf("chunk=%d\n got: %q\nwant: %q", chunk, got.String(), want)
		}
	}
}

func TestStreamProvisional(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()
	s.Feed("> a partial quo")
	// Nothing is final yet, but the tail must still be displayable.
	if got := s.Provisional(); !strings.Contains(got, "<blockquote>") || !strings.Contains(got, "a partial quo") {
		t.Errorf("provisional view lost the open tail: %q", got)
	}
	// The probe must not disturb the real state.
	final := s.Feed("te\n") + s.Close()
	if want := "<blockquote>\n<p>a partial quote</p>\n</blockquote>\n"; final != want {
		t.Errorf("got %q want %q", final, want)
	}
}

func TestStreamCloseIsIdempotent(t *testing.T) {
	s := mdflow.New().Stream()
	s.Feed("# hi")
	if first := s.Close(); first == "" {
		t.Fatal("first Close returned nothing")
	}
	if second := s.Close(); second != "" {
		t.Errorf("second Close returned %q, want empty", second)
	}
}

func TestChainingIsImmutable(t *testing.T) {
	base := mdflow.New()
	shifted := base.Transform(mdflow.ShiftHeadings(2))

	if got, want := base.HTML("# a\n"), "<h1>a</h1>\n"; got != want {
		t.Errorf("base parser was mutated: got %q want %q", got, want)
	}
	if got, want := shifted.HTML("# a\n"), "<h3>a</h3>\n"; got != want {
		t.Errorf("derived parser: got %q want %q", got, want)
	}
}

func TestTransforms(t *testing.T) {
	cases := []struct {
		name string
		p    *mdflow.Parser
		in   string
		want string
	}{
		{
			"drop code blocks",
			mdflow.New().Transform(mdflow.Drop(mdflow.IsCodeBlock)),
			"a\n\n```\nx\n```\n\nb\n",
			"<p>a</p>\n<p>b</p>\n",
		},
		{
			"atomic match nested inside dropped span",
			mdflow.New().Transform(mdflow.Drop(func(e mdflow.Event) bool {
				return e.Node == token.Blockquote || e.Node == token.SoftBreak
			})),
			"> a\n> b\n\nafter\n",
			"<p>after</p>\n",
		},
		{
			"drop heading at level keeps following sibling",
			mdflow.New().Transform(mdflow.Drop(mdflow.AtLevel(2))),
			"## gone\n\nkeep\n",
			"<p>keep</p>\n",
		},
		{
			"unwrap links keeps text",
			mdflow.New().Transform(mdflow.Unwrap(mdflow.IsLink)),
			"see [docs](https://go.dev)\n",
			"<p>see docs</p>\n",
		},
		{
			"rewrite links",
			mdflow.New().Transform(mdflow.RewriteLinks(func(d string) string {
				return strings.Replace(d, "http://", "https://", 1)
			})),
			"[a](http://x.dev)\n",
			`<p><a href="https://x.dev">a</a></p>` + "\n",
		},
		{
			"map text only, not code",
			mdflow.New().Transform(mdflow.MapText(strings.ToUpper)),
			"ab `cd`\n",
			"<p>AB <code>cd</code></p>\n",
		},
		{
			"shift clamps at 6",
			mdflow.New().Transform(mdflow.ShiftHeadings(9)),
			"# a\n",
			"<h6>a</h6>\n",
		},
		{
			"composed chain",
			mdflow.New().
				Transform(mdflow.ShiftHeadings(1)).
				Transform(mdflow.Unwrap(mdflow.IsLink)),
			"# [t](/x)\n",
			"<h2>t</h2>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.HTML(tc.in); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// A pipeline that leaves every event alone must be byte-identical to the fast
// path — otherwise the two rendering routes have drifted apart.
func TestIdentityPipelineMatchesFastPath(t *testing.T) {
	src := "# T\n\npara *x* `c` [l](/u) ![i](/p)\n\n- a\n- b\n\n> q\n\n```go\nz\n```\n"
	base := mdflow.New()
	piped := base.Map(func(e mdflow.Event) mdflow.Event { return e })
	if got, want := piped.HTML(src), base.HTML(src); got != want {
		t.Errorf("pipeline route differs from fast path:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextAndHeadings(t *testing.T) {
	src := "# Title\n\nSome **bold** text.\n\n```go\nignored := 1\n```\n\n## Sub\n"
	p := mdflow.New()

	if got, want := p.Text(src), "Title\n\nSome bold text.\n\nSub"; got != want {
		t.Errorf("Text: got %q want %q", got, want)
	}

	hs := p.Headings(src)
	if len(hs) != 2 || hs[0].Level != 1 || hs[0].Text != "Title" || hs[1].Level != 2 || hs[1].Text != "Sub" {
		t.Errorf("Headings: got %+v", hs)
	}

	if got, want := p.Text("a\\\nb\n"), "a\nb"; got != want {
		t.Errorf("Text hard break: got %q want %q", got, want)
	}
	if got, want := p.Headings("a\\\nb\n===\n"), []mdflow.Heading{{Level: 1, Text: "a\nb"}}; !slices.Equal(got, want) {
		t.Errorf("Headings hard break: got %+v want %+v", got, want)
	}
}

func TestEventsAreLazy(t *testing.T) {
	// Breaking out of the range must stop the parser, not just the consumer.
	src := strings.Repeat("# h\n\npara\n\n", 10000)
	n := 0
	for range mdflow.New().Events(src) {
		n++
		if n == 3 {
			break
		}
	}
	if n != 3 {
		t.Errorf("consumed %d events, want 3", n)
	}
}

func TestSeqCombinators(t *testing.T) {
	src := "# a\n\n## b\n\n### c\n"
	p := mdflow.New()

	levels := iterx.Collect(iterx.FilterMap(p.Events(src), func(e mdflow.Event) (int, bool) {
		if e.Type == mdflow.EnterEvent && e.Node == token.Heading {
			return e.Level, true
		}
		return 0, false
	}))
	if len(levels) != 3 || levels[0] != 1 || levels[2] != 3 {
		t.Errorf("levels = %v", levels)
	}

	words := iterx.Reduce(p.Events(src), 0, func(acc int, e mdflow.Event) int {
		if e.Type == mdflow.TextEvent {
			return acc + len(strings.Fields(e.Text))
		}
		return acc
	})
	if words != 3 {
		t.Errorf("words = %d, want 3", words)
	}

	if got := iterx.Count(iterx.Take(p.Events(src), 4)); got != 4 {
		t.Errorf("Take(4) yielded %d", got)
	}
}

// Derivation isolation lives in builder_test.go as
// TestWithExtensionsDoesNotLeakIntoReceiver, with these same two assertions
// plus the non-HTML-renderer case this one could not reach.

// A Parser is documented as shareable; pooled state must not break that.
func TestParserIsConcurrencySafe(t *testing.T) {
	p := mdflow.New()
	src := "# T\n\n- a\n- b\n\n> q *x*\n\n```go\nz\n```\n"
	want := p.HTML(src)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if got := p.HTML(src); got != want {
					t.Errorf("concurrent mismatch: %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestRenderToWriter(t *testing.T) {
	var b strings.Builder
	if err := mdflow.Render(&b, "# hi\n"); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), "<h1>hi</h1>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":   "hello-world",
		"  Trim Me  ":   "trim-me",
		"C++ & Rust!":   "c-rust",
		"中文 标题":         "中文-标题",
		"multi   space": "multi-space",
	}
	for in, want := range cases {
		if got := mdflow.Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
