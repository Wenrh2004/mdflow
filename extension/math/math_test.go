package math_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/math"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), math.Math)
}

func TestMathRenders(t *testing.T) {
	cases := map[string]string{
		"$E=mc^2$ here\n":     `<p><code class="language-math">E=mc^2</code> here</p>` + "\n",
		"costs $5 today\n":    "<p>costs $5 today</p>\n", // a lone dollar is text
		"$$\n\\frac ab\n$$\n": "<pre><code class=\"language-math\">\\frac ab\n</code></pre>\n",
		"$a$ and $b$\n":       `<p><code class="language-math">a</code> and <code class="language-math">b</code></p>` + "\n",
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// The math content is verbatim: an HTML-special byte in a formula is escaped,
// not interpreted.
func TestMathContentIsEscaped(t *testing.T) {
	if got, want := newParser().HTML("$a < b$\n"), `<p><code class="language-math">a &lt; b</code></p>`+"\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	math.Syntax(rules)
	// Inline math is atomic and carries its own text.
	if got, want := mdflow.NewWith(rules, html.NewRenderer()).HTML("$x$\n"), "<p>x</p>\n"; got != want {
		t.Errorf("syntax-only inline: got %q want %q", got, want)
	}
	// A math block is a literal leaf; its body survives as text.
	if got, want := mdflow.NewWith(rules, html.NewRenderer()).HTML("$$\nx\n$$\n"), "x\n"; got != want {
		t.Errorf("syntax-only block: got %q want %q", got, want)
	}
}

func TestIsMath(t *testing.T) {
	p := newParser()
	inline, block := 0, 0
	for e := range p.Events("$x$\n\n$$\ny\n$$\n") {
		if math.IsMath(e) && e.Type == mdflow.EnterEvent {
			if mdflow.IsLeafNode(e.Node) {
				block++
			} else {
				inline++
			}
		}
	}
	if inline != 1 || block != 1 {
		t.Errorf("IsMath: inline=%d block=%d, want 1 and 1", inline, block)
	}
}

func TestMathBlockSurvivesStreaming(t *testing.T) {
	src := "$$\na + b\nc + d\n$$\n"
	p := newParser()
	want := p.HTML(src)
	for _, chunk := range []int{1, 2, 5} {
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
