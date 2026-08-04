package strikethrough_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/strikethrough"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), strikethrough.Strikethrough)
}

func TestStrikethroughRenders(t *testing.T) {
	cases := map[string]string{
		"~~gone~~\n":        "<p><del>gone</del></p>\n",
		"~~*a* b~~\n":       "<p><del><em>a</em> b</del></p>\n", // content parses recursively
		"a ~~b~~ c ~~d~~\n": "<p>a <del>b</del> c <del>d</del></p>\n",
		"~~ open only\n":    "<p>~~ open only</p>\n", // no close: literal
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// Syntax without Output surfaces the node but renders no markup: the content
// survives as text, proving the two halves are genuinely independent.
func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	strikethrough.Syntax(rules)
	if got, want := mdflow.NewWith(rules, html.NewRenderer()).HTML("~~gone~~\n"), "<p>gone</p>\n"; got != want {
		t.Errorf("syntax-only: got %q want %q", got, want)
	}
}

func TestIsStrikethrough(t *testing.T) {
	p := newParser()
	n := 0
	for e := range p.Events("~~a~~ plain ~~b~~\n") {
		if strikethrough.IsStrikethrough(e) {
			n++
		}
	}
	if n != 4 { // two runs, enter + leave each
		t.Errorf("IsStrikethrough matched %d events, want 4", n)
	}
}
