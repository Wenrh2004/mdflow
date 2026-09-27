package text_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/text"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), text.NewRenderer())
}

func TestRendersPlainText(t *testing.T) {
	cases := map[string]string{
		"# Title\n\nA *para* with `code`.\n":            "Title\n\nA para with code.\n\n",
		"- one\n- two\n":                                "- one\n- two\n\n",
		"- [ ] todo\n- [x] done\n":                      "- [ ] todo\n- [x] done\n\n",
		"> quoted\n":                                    "quoted\n\n",
		"```go\nx := 1\n```\n":                          "x := 1\n\n",
		"para one\n\npara two\n":                        "para one\n\npara two\n\n",
		"a [link](http://x) and ![img](/i.png \"t\")\n": "a link and img\n\n",
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("text(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// Markup carries no glyphs into the text: emphasis, strong, links and code
// spans contribute only their words.
func TestMarkupIsStripped(t *testing.T) {
	got := newParser().HTML("***bold italic*** stays as [a](/b) with `code`\n")
	for _, glyph := range []string{"*", "[", "]", "(", ")", "<", ">", "`"} {
		if strings.Contains(got, glyph) {
			t.Errorf("text output kept markup glyph %q: %q", glyph, got)
		}
	}
	if !strings.Contains(got, "bold italic") || !strings.Contains(got, "code") {
		t.Errorf("text output dropped words: %q", got)
	}
}

// A parser wired to the text renderer streams identically to a single-shot
// render, the same contract the HTML renderer honours.
func TestTextSurvivesStreaming(t *testing.T) {
	src := "# H\n\nprose *here*\n\n- a\n- b\n"
	p := newParser()
	want := p.HTML(src)
	for _, chunk := range []int{1, 3, 8} {
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
