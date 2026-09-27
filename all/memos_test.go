package all_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/extension/hashtag"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
	"github.com/Wenrh2004/mdflow/iterx"
)

// The constructs added for parity with github.com/usememos/gomark, exercised
// through the full-syntax bundle in this module.
func TestMemosFlavourSyntax(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"inline math", "$E=mc^2$ here\n", `<p><code class="language-math">E=mc^2</code> here</p>` + "\n"},
		{"lone dollar is text", "costs $5 today\n", "<p>costs $5 today</p>\n"},
		{
			"math block",
			"$$\n\\frac{a}{b}\n$$\n",
			"<pre><code class=\"language-math\">\\frac{a}{b}\n</code></pre>\n",
		},
		{"tag", "see #golang here\n", `<p>see <span class="tag">#golang</span> here</p>` + "\n"},
		{"tag at line start", "#todo item\n", `<p><span class="tag">#todo</span> item</p>` + "\n"},
		{"heading still wins", "# Heading\n", "<h1>Heading</h1>\n"},
		{"highlight", "==important==\n", "<p><mark>important</mark></p>\n"},
		{"subscript", "H~2~O\n", "<p>H<sub>2</sub>O</p>\n"},
		{"superscript", "x^2^\n", "<p>x<sup>2</sup></p>\n"},
		{"strikethrough still wins over subscript", "~~gone~~\n", "<p><del>gone</del></p>\n"},
		{"spoiler", "||secret||\n", "<p><details><summary>secret</summary></details></p>\n"},
		{
			"reference",
			"see [[my-note]]\n",
			`<p>see <span class="reference" data-resource="my-note">my-note</span></p>` + "\n",
		},
		{
			"reference with params",
			"[[note?align=center]]\n",
			`<p><span class="reference" data-resource="note" data-params="align=center">note</span></p>` + "\n",
		},
		{
			"embed block",
			"![[photo.png]]\n",
			`<div class="embed" data-resource="photo.png">photo.png</div>` + "\n",
		},
		{
			"embed with params",
			"![[photo.png?w=100]]\n",
			`<div class="embed" data-resource="photo.png" data-params="w=100">photo.png</div>` + "\n",
		},
		{"link still wins over reference", "[a](/b)\n", `<p><a href="/b">a</a></p>` + "\n"},
		{"autolink still wins over raw html", "<https://go.dev>\n", `<p><a href="https://go.dev">https://go.dev</a></p>` + "\n"},
	}

	p := all.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.HTML(tc.in); got != tc.want {
				t.Errorf("HTML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// mdflow parses wrapper content recursively where gomark stringifies it.
func TestWrappersNestRecursively(t *testing.T) {
	cases := map[string]string{
		"==a **b**==\n": "<p><mark>a <strong>b</strong></mark></p>\n",
		"||a `c`||\n":   "<p><details><summary>a <code>c</code></summary></details></p>\n",
		"^*x*^\n":       "<p><sup><em>x</em></sup></p>\n",
	}
	p := all.New()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// Raw HTML is always recognised as structure; only its rendering is gated.
func TestRawHTMLIsEscapedByDefault(t *testing.T) {
	src := "press <kbd>Ctrl</kbd> now\n"

	safe := all.New()
	if got, want := safe.HTML(src), "<p>press &lt;kbd&gt;Ctrl&lt;/kbd&gt; now</p>\n"; got != want {
		t.Errorf("default should escape:\n got: %q\nwant: %q", got, want)
	}

	unsafe := all.New(rawhtml.WithUnsafeHTML())
	if got, want := unsafe.HTML(src), "<p>press <kbd>Ctrl</kbd> now</p>\n"; got != want {
		t.Errorf("WithUnsafeHTML should pass through:\n got: %q\nwant: %q", got, want)
	}

	// Either way the parser reports the tags, so callers can sanitise themselves.
	n := iterx.Count(iterx.Filter(safe.Events(src), rawhtml.IsRawHTML))
	if n != 2 {
		t.Errorf("expected 2 raw HTML events, got %d", n)
	}
}

func TestScriptTagIsNotExecutable(t *testing.T) {
	got := all.New().HTML("<script>alert(1)</script>\n")
	if strings.Contains(got, "<script>") {
		t.Errorf("default renderer emitted a live script tag: %q", got)
	}
}

// Atomic inline nodes are a single EnterEvent with no LeaveEvent; Drop must
// handle them without falling into an unterminated skip.
func TestDropHandlesAtomicNodes(t *testing.T) {
	p := all.New().Transform(mdflow.Drop(hashtag.IsHashtag))
	if got, want := p.HTML("a #tag b\n"), "<p>a  b</p>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	// A following paired node must still render — proof the skip terminated.
	if got, want := p.HTML("#t and *em*\n"), "<p> and <em>em</em></p>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNewConstructsSurviveStreaming(t *testing.T) {
	src := "# T\n\n$E=mc^2$ and #tag and ==hi== and ~s~ and ^p^ and ||x||\n\n" +
		"$$\na+b\n$$\n\n![[res.png]]\n\n[[ref]]\n"
	p := all.New()
	want := p.HTML(src)
	for _, chunk := range []int{1, 3, 17, 512} {
		var got strings.Builder
		s := p.Stream()
		for i := 0; i < len(src); i += chunk {
			got.WriteString(s.Feed(src[i:min(i+chunk, len(src))]))
		}
		got.WriteString(s.Finish())
		if got.String() != want {
			t.Errorf("chunk=%d\n got: %q\nwant: %q", chunk, got.String(), want)
		}
	}
}

func TestNewConstructsRoundTripThroughPipeline(t *testing.T) {
	src := "$x$ #t ==h== ~s~ ^p^ ||sp|| [[r]]\n\n$$\nm\n$$\n\n![[e]]\n"
	base := all.New()
	piped := base.Map(func(e mdflow.Event) mdflow.Event { return e })
	if got, want := piped.HTML(src), base.HTML(src); got != want {
		t.Errorf("pipeline route differs from fast path:\n got: %q\nwant: %q", got, want)
	}
}
