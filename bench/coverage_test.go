package bench

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
)

// TestFeatureCoverage backs the README's claim that mdflow's syntax coverage is
// a superset of gomark's — one case per gomark AST node type. Without it, the
// benchmark numbers would be unfalsifiable: a parser looks fast when it silently
// skips syntax it does not implement.
//
// The assertion is on *structure recognised*, not on identical bytes: the two
// libraries pick different markup for the same construct (a bare `<span>` vs
// `<span class="tag">`, and so on). What matters is that neither drops the
// construct on the floor.
func TestFeatureCoverage(t *testing.T) {
	// want is a substring proving mdflow recognised the construct rather than
	// echoing it as literal text.
	cases := []struct {
		node string // the gomark ast node type this covers
		in   string
		want string
	}{
		{"Paragraph", "text\n", "<p>text</p>"},
		{"Heading", "## h\n", "<h2>h</h2>"},
		{"CodeBlock", "```go\nx\n```\n", `<code class="language-go">`},
		{"HorizontalRule", "---\n", "<hr />"},
		{"Blockquote", "> q\n", "<blockquote>"},
		{"List/UnorderedListItem", "- a\n", "<ul>\n<li>a</li>"},
		{"OrderedListItem", "1. a\n", "<ol>\n<li>a</li>"},
		{"TaskListItem", "- [x] a\n", `type="checkbox" checked`},
		{"MathBlock", "$$\nx\n$$\n", `<pre><code class="language-math">`},
		{"Table", "| a |\n| --- |\n| 1 |\n", "<table>"},
		{"EmbeddedContent", "![[r]]\n", `class="embed"`},
		{"Text", "plain\n", "plain"},
		{"Bold", "**b**\n", "<strong>b</strong>"},
		{"Italic", "*i*\n", "<em>i</em>"},
		{"BoldItalic", "***bi***\n", "<em><strong>bi</strong></em>"},
		{"Code", "`c`\n", "<code>c</code>"},
		{"Image", "![a](/b.png)\n", `<img src="/b.png" alt="a"`},
		{"Link", "[a](/b)\n", `<a href="/b">a</a>`},
		{"AutoLink", "<https://go.dev>\n", `<a href="https://go.dev">`},
		{"Tag", "#t\n", `<span class="tag">#t</span>`},
		{"Strikethrough", "~~s~~\n", "<del>s</del>"},
		{"EscapingCharacter", `\*x\*` + "\n", "*x*"},
		{"Math", "$x$\n", `<code class="language-math">x</code>`},
		{"Highlight", "==h==\n", "<mark>h</mark>"},
		{"Subscript", "~s~\n", "<sub>s</sub>"},
		{"Superscript", "^p^\n", "<sup>p</sup>"},
		{"ReferencedContent", "[[r]]\n", `class="reference"`},
		{"Spoiler", "||s||\n", "<details><summary>s</summary></details>"},
		{"HTMLElement", "<kbd>C</kbd>\n", "<kbd>C</kbd>"},
		{"LineBreak (hard)", "a  \nb\n", "<br />"},
		{"Document", "a\n\nb\n", "<p>a</p>\n<p>b</p>"},
	}

	// HTMLElement needs the unsafe renderer; everything else uses the default.
	safe, unsafe := all.New(), all.New(rawhtml.WithUnsafeHTML())
	for _, tc := range cases {
		t.Run(tc.node, func(t *testing.T) {
			p := safe
			if strings.Contains(tc.node, "HTMLElement") {
				p = unsafe
			}
			if got := p.HTML(tc.in); !strings.Contains(got, tc.want) {
				t.Errorf("gomark node %s has no mdflow equivalent\n input: %q\n  want substring: %q\n     got: %q",
					tc.node, tc.in, tc.want, got)
			}
		})
	}

	if len(cases) < 31 {
		t.Errorf("gomark declares 31 AST node types; only %d are covered here", len(cases))
	}
}

// TestDisagreements pins the inputs where the two libraries differ, so the
// README's comparison table stays true as either side changes.
func TestDisagreements(t *testing.T) {
	p := all.New()
	cases := []struct {
		in         string
		mdflowWant string
		note       string
	}{
		{"Title\n=====\n", "<h1>Title</h1>", "setext heading; gomark has no support"},
		{"[a](/b \"t\")\n", `title="t"`, "link title; gomark renders it literally"},
		{"<!-- c -->\n", "&lt;!--", "HTML comment; gomark turns it into an autolink"},
		{"***bi***\n", "<em><strong>", "CommonMark nesting order; gomark inverts it"},
		{"| a |\n| - |\n| 1 |\n", "<table>", "1-dash delimiter; gomark needs 3+"},
	}
	for _, tc := range cases {
		if got := p.HTML(tc.in); !strings.Contains(got, tc.mdflowWant) {
			t.Errorf("%s\n input: %q\n  want substring: %q\n     got: %q",
				tc.note, tc.in, tc.mdflowWant, got)
		}
	}
}
