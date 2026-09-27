package rawhtml_test

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
	"github.com/Wenrh2004/mdflow/parser"
	mdhtml "github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/token"
)

func explicitSafeParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), mdhtml.NewRenderer(), rawhtml.RawHTML)
}

func unsafeParser() *mdflow.Parser {
	return mdflow.New(rawhtml.WithUnsafeHTML())
}

func TestNewEnablesSafeRawHTMLByDefault(t *testing.T) {
	cases := []struct {
		name, markdown, want string
	}{
		{"inline", "a <b>c</b>\n", "<p>a &lt;b&gt;c&lt;/b&gt;</p>\n"},
		{"block", "<script>alert(1)</script>\n", "&lt;script&gt;alert(1)&lt;/script&gt;\n"},
		{"block-tag", "<br>\n", "&lt;br&gt;\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mdflow.New().HTML(tc.markdown); got != tc.want {
				t.Fatalf("HTML(%q)\n got: %q\nwant: %q", tc.markdown, got, tc.want)
			}
		})
	}
}

func TestExplicitRawHTMLIsSafe(t *testing.T) {
	if got, want := explicitSafeParser().HTML("x <img src=x onerror=alert(1)> y\n"),
		"<p>x &lt;img src=x onerror=alert(1)&gt; y</p>\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSafeRawHTMLEscapesEveryForm(t *testing.T) {
	cases := []struct {
		name, markdown, want string
	}{
		{
			"open-tag", "x <img src=\"&\" onerror=\"alert(1)\"> y\n",
			"<p>x &lt;img src=&quot;&amp;&quot; onerror=&quot;alert(1)&quot;&gt; y</p>\n",
		},
		{"comment", "x <!-- &<>\" --> y\n", "<p>x &lt;!-- &amp;&lt;&gt;&quot; --&gt; y</p>\n"},
		{"instruction", "x <?target &<>\" ?> y\n", "<p>x &lt;?target &amp;&lt;&gt;&quot; ?&gt; y</p>\n"},
		{"declaration", "x <!ELEMENT &<>\"> y\n", "<p>x &lt;!ELEMENT &amp;&lt;&gt;&quot;&gt; y</p>\n"},
		{"cdata", "x <![CDATA[&<>\"]]> y\n", "<p>x &lt;![CDATA[&amp;&lt;&gt;&quot;]]&gt; y</p>\n"},
		{
			"block", "<script>if (a < b && c > d) alert(\"x\")</script>\n",
			"&lt;script&gt;if (a &lt; b &amp;&amp; c &gt; d) alert(&quot;x&quot;)&lt;/script&gt;\n",
		},
		{
			"malformed", "x <img src=x onerror=alert(1)\n",
			"<p>x &lt;img src=x onerror=alert(1)</p>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mdflow.New().HTML(tc.markdown); got != tc.want {
				t.Fatalf("HTML(%q)\n got: %q\nwant: %q", tc.markdown, got, tc.want)
			}
		})
	}
}

func TestWithUnsafeHTMLUsesTheDefaultSyntax(t *testing.T) {
	p := unsafeParser()
	if got, want := p.HTML("a <b>c</b>\n"), "<p>a <b>c</b></p>\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := p.HTML("<script>alert(1)</script>\n"), "<script>alert(1)</script>\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestUnsafeOutputCommutesWithSyntaxSelection(t *testing.T) {
	const src = "<i>trusted</i>\n"
	const wantOutput = "<p><i>trusted</i></p>\n"
	first := mdflow.New(rawhtml.WithUnsafeHTML(), mdflow.WithOnly(rawhtml.RawHTML))
	last := mdflow.New(mdflow.WithOnly(rawhtml.RawHTML), rawhtml.WithUnsafeHTML())
	if got, want := first.HTML(src), last.HTML(src); got != want || got != wantOutput {
		t.Fatalf("option order changed trusted output: first=%q last=%q", got, want)
	}
}

func TestTypeOneBlockEndsAtAnyRawTextClosingTag(t *testing.T) {
	names := []string{"pre", "script", "style", "textarea"}
	for _, open := range names {
		for _, close := range names {
			t.Run(open+"/"+close, func(t *testing.T) {
				src := "<" + open + ">\nbody\n</" + close + ">\nafter\n"
				want := "<" + open + ">\nbody\n</" + close + ">\n<p>after</p>\n"
				if got := unsafeParser().HTML(src); got != want {
					t.Fatalf("mismatched raw-text closer did not end type 1 block\n got: %q\nwant: %q", got, want)
				}
			})
		}
	}
}

func TestTypeSevenExcludesSpecialOpenTags(t *testing.T) {
	for _, name := range []string{"pre", "script", "style", "textarea"} {
		t.Run(name, func(t *testing.T) {
			src := "<" + name + "/>\n\nafter\n"
			want := "<p><" + name + "/></p>\n<p>after</p>\n"
			if got := unsafeParser().HTML(src); got != want {
				t.Fatalf("special self-closing tag was classified as a type 7 block\n got: %q\nwant: %q", got, want)
			}
		})
	}

	// The exclusion applies to open tags only; a complete closing tag is still
	// a type 7 block.
	if got, want := unsafeParser().HTML("</pre>\n\nafter\n"), "</pre>\n<p>after</p>\n"; got != want {
		t.Fatalf("special closing tag stopped being type 7\n got: %q\nwant: %q", got, want)
	}
}

func TestRawHTMLBlockIndentUsesContainerColumn(t *testing.T) {
	cases := []struct {
		name, markdown, safe, unsafe string
	}{
		{
			name:     "blockquote",
			markdown: "> \t<div>\n",
			safe:     "<blockquote>\n\t&lt;div&gt;\n</blockquote>\n",
			unsafe:   "<blockquote>\n\t<div>\n</blockquote>\n",
		},
		{
			name:     "lazy-nested-blockquote",
			markdown: "> > para\n> \t<div>\n",
			safe:     "<blockquote>\n<blockquote>\n<p>para</p>\n</blockquote>\n\t&lt;div&gt;\n</blockquote>\n",
			unsafe:   "<blockquote>\n<blockquote>\n<p>para</p>\n</blockquote>\n\t<div>\n</blockquote>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/safe", func(t *testing.T) {
			if got := mdflow.New().HTML(tc.markdown); got != tc.safe {
				t.Fatalf("safe container-column render\n got: %q\nwant: %q", got, tc.safe)
			}
		})
		t.Run(tc.name+"/unsafe", func(t *testing.T) {
			if got := unsafeParser().HTML(tc.markdown); got != tc.unsafe {
				t.Fatalf("unsafe container-column render\n got: %q\nwant: %q", got, tc.unsafe)
			}
		})
	}
}

func TestRawHTMLBlockIndentPreservesNestedPartialTab(t *testing.T) {
	const markdown = ">   - x\n> \t\t<div>\n"
	wantBody := false
	found := false
	for event := range unsafeParser().Events(markdown) {
		if event.Type == mdflow.EnterEvent && event.Node == token.CustomLeaf && rawhtml.IsRawHTML(event) {
			wantBody = true
			continue
		}
		if wantBody && event.Type == mdflow.TextEvent {
			if event.Text != "  <div>\n" {
				t.Fatalf("partial tab remainder changed: got %q want %q", event.Text, "  <div>\n")
			}
			found = true
			wantBody = false
		}
	}
	if !found {
		t.Fatal("nested blockquote/list partial tab did not produce a raw HTML block")
	}
}

func TestCoreRuleSetRemainsRawHTMLBlind(t *testing.T) {
	for name, p := range map[string]*mdflow.Parser{
		"NewWith":  mdflow.NewWith(parser.New(), mdhtml.NewRenderer()),
		"WithOnly": mdflow.New(mdflow.WithOnly()),
	} {
		t.Run(name, func(t *testing.T) {
			if got, want := p.HTML("<div>\nx\n\n"), "<p>&lt;div&gt;\nx</p>\n"; got != want {
				t.Fatalf("raw-HTML semantics leaked into the core profile: got %q want %q", got, want)
			}
		})
	}
}

func TestAutolinkStillWins(t *testing.T) {
	got := mdflow.New().HTML("<https://example.com>\n")
	if want := `<p><a href="https://example.com">https://example.com</a></p>` + "\n"; got != want {
		t.Fatalf("autolink lost the `<` trigger: got %q want %q", got, want)
	}
}

func TestRawHTMLEvents(t *testing.T) {
	t.Run("inline-is-atomic", func(t *testing.T) {
		var got []mdflow.Event
		for e := range mdflow.New().Events("a <b>c</b>\n") {
			if rawhtml.IsRawHTML(e) {
				got = append(got, e)
			}
		}
		if len(got) != 2 {
			t.Fatalf("IsRawHTML matched %d events, want 2", len(got))
		}
		for _, e := range got {
			if !e.IsAtomic() || e.Node != token.Custom || e.Type != mdflow.EnterEvent {
				t.Errorf("raw inline event is not atomic: %+v", e)
			}
		}
	})

	t.Run("block-is-literal-custom-leaf", func(t *testing.T) {
		events := slices.Collect(mdflow.New().Events("<div>\nx\n\n"))
		if len(events) != 3 {
			t.Fatalf("got %d events, want 3: %+v", len(events), events)
		}
		if e := events[0]; e.Type != mdflow.EnterEvent || e.Node != token.CustomLeaf || !e.Literal || !rawhtml.IsRawHTML(e) {
			t.Errorf("unexpected raw block open: %+v", e)
		}
		if e := events[1]; e.Type != mdflow.TextEvent || e.Text != "<div>\nx\n" {
			t.Errorf("unexpected raw block body: %+v", e)
		}
		if e := events[2]; e.Type != mdflow.LeaveEvent || e.Node != token.CustomLeaf || !rawhtml.IsRawHTML(e) {
			t.Errorf("unexpected raw block close: %+v", e)
		}
	})
}

type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func rawHTMLSpecExamples(t *testing.T) []specExample {
	t.Helper()
	b, err := os.ReadFile("../../testdata/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var all []specExample
	if err := json.Unmarshal(b, &all); err != nil {
		t.Fatal(err)
	}
	out := make([]specExample, 0, 64)
	for _, e := range all {
		if e.Section == "HTML blocks" || e.Section == "Raw HTML" {
			out = append(out, e)
		}
	}
	if len(out) != 64 {
		t.Fatalf("loaded %d raw-HTML examples, want 64", len(out))
	}
	return out
}

func TestRawHTMLCommonMark0312(t *testing.T) {
	p := unsafeParser()
	for _, e := range rawHTMLSpecExamples(t) {
		t.Run(e.Section+"/"+strconv.Itoa(e.Example), func(t *testing.T) {
			if got := p.HTML(e.Markdown); got != e.HTML {
				t.Fatalf("example %d\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Markdown, got, e.HTML)
			}
		})
	}
}

func TestRawHTMLIdentityMiddleware(t *testing.T) {
	sources := []string{
		"<script>\nif (a < b) alert(\"x\")\n</script>\n\na <b>x</b>\n",
		"> \t<div>\n",
		"> > para\n> \t<div>\n",
		">   - x\n> \t\t<div>\n",
	}
	for _, e := range rawHTMLSpecExamples(t) {
		sources = append(sources, e.Markdown)
	}
	for _, p := range []*mdflow.Parser{mdflow.New(), unsafeParser()} {
		for _, src := range sources {
			want := p.HTML(src)
			got := p.Map(func(e mdflow.Event) mdflow.Event { return e }).HTML(src)
			if got != want {
				t.Fatalf("identity middleware changed output for %q\n got: %q\nwant: %q", src, got, want)
			}
		}
	}
}

func TestRawHTMLStreamMatchesBatch(t *testing.T) {
	cases := []string{
		"a <b data-x=\"1\">c</b>\n",
		"<script>\nx < y && y > z\n</script>\nnext\n",
		"<div>\n*x*\n\nnext\n",
		"foo <!-- comment\ncontinued --> bar\n",
		"> \t<div>\n",
		"> > para\n> \t<div>\n",
		">   - x\n> \t\t<div>\n",
	}
	for _, e := range rawHTMLSpecExamples(t) {
		cases = append(cases, e.Markdown)
	}
	for _, p := range []*mdflow.Parser{mdflow.New(), unsafeParser()} {
		for _, src := range cases {
			want := p.HTML(src)
			for split := 0; split <= len(src); split++ {
				s := p.Stream()
				got := s.Feed(src[:split]) + s.Feed(src[split:]) + s.Finish()
				if got != want {
					t.Fatalf("split %d of %q\n got: %q\nwant: %q", split, src, got, want)
				}
			}

			s := p.Stream()
			committed := ""
			for i := range len(src) {
				committed += s.Feed(src[i : i+1])
				if got, wantPrefix := committed+s.Provisional(), p.HTML(src[:i+1]); got != wantPrefix {
					t.Fatalf("prefix %d of %q\n got: %q\nwant: %q", i+1, src, got, wantPrefix)
				}
			}
			if got := committed + s.Finish(); got != want {
				t.Fatalf("byte stream close for %q\n got: %q\nwant: %q", src, got, want)
			}
		}
	}
}

func TestFilteredHTMLEscapesDisallowedTags(t *testing.T) {
	md := mdflow.New(rawhtml.WithFilteredHTML())
	cases := []struct{ in, want string }{
		// GFM spec example 652 (tagfilter).
		{"<strong> <title> <style> <em>\n\n<blockquote>\n  <xmp> is disallowed.  <XMP> is also disallowed.\n</blockquote>\n",
			"<p><strong> &lt;title> &lt;style> <em></p>\n<blockquote>\n  &lt;xmp> is disallowed.  &lt;XMP> is also disallowed.\n</blockquote>\n"},
		{"<script>alert(1)</script>\n", "&lt;script>alert(1)&lt;/script>\n"},
		{"a <iframe src=x> b\n", "<p>a &lt;iframe src=x> b</p>\n"},
		{"<scripts>\n", "<scripts>\n"},
		{"<div>ok</div>\n", "<div>ok</div>\n"},
	}
	for _, c := range cases {
		if got := md.HTML(c.in); got != c.want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", c.in, got, c.want)
		}
	}
}
