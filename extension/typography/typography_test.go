package typography_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/typography"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), typography.Typography)
}

func TestTypographyRenders(t *testing.T) {
	cases := map[string]string{
		"==hi==\n":      "<p><mark>hi</mark></p>\n",
		"==a **b**==\n": "<p><mark>a <strong>b</strong></mark></p>\n", // content nests
		"H~2~O\n":       "<p>H<sub>2</sub>O</p>\n",
		"x^2^\n":        "<p>x<sup>2</sup></p>\n",
		"||secret||\n":  "<p><details><summary>secret</summary></details></p>\n",
		"==open only\n": "<p>==open only</p>\n", // no close: literal
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// Syntax without Output surfaces the four wrappers but renders no markup: each
// wrapper's content survives as text, proving the halves are independent.
func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	typography.Syntax(rules)
	p := mdflow.NewWith(rules, html.NewRenderer())
	cases := map[string]string{
		"==hi==\n":     "<p>hi</p>\n",
		"H~2~O\n":      "<p>H2O</p>\n",
		"x^2^\n":       "<p>x2</p>\n",
		"||secret||\n": "<p>secret</p>\n",
	}
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("syntax-only HTML(%q): got %q want %q", in, got, want)
		}
	}
}

func TestPredicates(t *testing.T) {
	p := newParser()
	got := struct{ hi, sub, sup, spoil int }{}
	for e := range p.Events("==a== H~2~O x^2^ ||s||\n") {
		switch {
		case typography.IsHighlight(e):
			got.hi++
		case typography.IsSubscript(e):
			got.sub++
		case typography.IsSuperscript(e):
			got.sup++
		case typography.IsSpoiler(e):
			got.spoil++
		}
	}
	// Each wrapper is paired: one enter + one leave = 2 events apiece.
	if got.hi != 2 || got.sub != 2 || got.sup != 2 || got.spoil != 2 {
		t.Errorf("predicate counts = %+v, want 2 each", got)
	}
}
