package hashtag_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/hashtag"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), hashtag.Hashtag)
}

func TestHashtagRenders(t *testing.T) {
	cases := map[string]string{
		"#todo now\n":    `<p><span class="tag">#todo</span> now</p>` + "\n",
		"a #b and #c\n":  `<p>a <span class="tag">#b</span> and <span class="tag">#c</span></p>` + "\n",
		"# heading\n":    "<h1>heading</h1>\n", // '# ' is an ATX heading, never a hashtag
		"bare # alone\n": "<p>bare # alone</p>\n",
		"#a#b\n":         `<p><span class="tag">#a</span><span class="tag">#b</span></p>` + "\n",
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// A hashtag's text is escaped, not interpreted, on output.
func TestHashtagIsEscaped(t *testing.T) {
	if got, want := newParser().HTML("#a&b\n"), `<p><span class="tag">#a&amp;b</span></p>`+"\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// Syntax without Output surfaces the node but renders no markup: the tag body
// survives as text, proving the two halves are genuinely independent.
func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	hashtag.Syntax(rules)
	if got, want := mdflow.NewWith(rules, html.NewRenderer()).HTML("#todo now\n"), "<p>todo now</p>\n"; got != want {
		t.Errorf("syntax-only: got %q want %q", got, want)
	}
}

func TestIsHashtag(t *testing.T) {
	p := newParser()
	n := 0
	for e := range p.Events("#a plain #b\n") {
		if hashtag.IsHashtag(e) {
			if !e.IsAtomic() {
				t.Errorf("hashtag event should be atomic: %+v", e)
			}
			n++
		}
	}
	if n != 2 { // atomic: one event per tag, no matching Leave
		t.Errorf("IsHashtag matched %d events, want 2", n)
	}
}
