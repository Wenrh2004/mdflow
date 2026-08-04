package resource_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/resource"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), resource.Resource)
}

func TestResourceRenders(t *testing.T) {
	cases := map[string]string{
		"see [[note]]\n":    `<p>see <span class="reference" data-resource="note">note</span></p>` + "\n",
		"[[note?size=2]]\n": `<p><span class="reference" data-resource="note" data-params="size=2">note</span></p>` + "\n",
		"![[image.png]]\n":  `<div class="embed" data-resource="image.png">image.png</div>` + "\n",
		"![[a.png?w=1]]\n":  `<div class="embed" data-resource="a.png" data-params="w=1">a.png</div>` + "\n",
		// An embed must own its line: with trailing text the line is an
		// ordinary paragraph, where the inline reference rule still matches.
		"![[x]] and text\n": `<p>!<span class="reference" data-resource="x">x</span> and text</p>` + "\n",
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// A reference must beat the core link rule on the shared `[` trigger.
func TestReferenceBeatsLink(t *testing.T) {
	got := newParser().HTML("[[note]](not-a-link)\n")
	if !strings.Contains(got, `<span class="reference" data-resource="note">note</span>`) {
		t.Errorf("reference lost the `[` trigger to the link rule: %q", got)
	}
}

// Content is escaped, not interpreted, on output.
func TestResourceIsEscaped(t *testing.T) {
	if got, want := newParser().HTML("[[a&b]]\n"), `<p><span class="reference" data-resource="a&amp;b">a&amp;b</span></p>`+"\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// Syntax without Output surfaces the nodes but renders no markup: reference text
// and embed name survive as text, proving the halves are independent.
func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	resource.Syntax(rules)
	p := mdflow.NewWith(rules, html.NewRenderer())
	if got, want := p.HTML("see [[note]]\n"), "<p>see note</p>\n"; got != want {
		t.Errorf("syntax-only reference: got %q want %q", got, want)
	}
	if got, want := p.HTML("![[image.png]]\n"), "image.png"; got != want {
		t.Errorf("syntax-only embed: got %q want %q", got, want)
	}
}

func TestIsResource(t *testing.T) {
	p := newParser()
	inline, block := 0, 0
	for e := range p.Events("[[note]]\n\n![[image.png]]\n") {
		if resource.IsResource(e) && e.Type == mdflow.EnterEvent {
			if mdflow.IsLeafNode(e.Node) {
				block++
			} else {
				inline++
			}
		}
	}
	if inline != 1 || block != 1 {
		t.Errorf("IsResource: inline=%d block=%d, want 1 and 1", inline, block)
	}
}
