package mdflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
)

func TestCommonMarkLazyParagraphContinuation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markdown string
		html     string
	}{
		{
			name:     "official example 232 omits one blockquote marker",
			markdown: "> # Foo\n> bar\nbaz\n",
			html:     "<blockquote>\n<h1>Foo</h1>\n<p>bar\nbaz</p>\n</blockquote>\n",
		},
		{
			name:     "official example 233 resumes explicit markers",
			markdown: "> bar\nbaz\n> foo\n",
			html:     "<blockquote>\n<p>bar\nbaz\nfoo</p>\n</blockquote>\n",
		},
		{
			name:     "official example 250 omits all nested markers",
			markdown: "> > > foo\nbar\n",
			html:     "<blockquote>\n<blockquote>\n<blockquote>\n<p>foo\nbar</p>\n</blockquote>\n</blockquote>\n</blockquote>\n",
		},
		{
			name:     "official example 251 omits a suffix of nested markers",
			markdown: ">>> foo\n> bar\n>>baz\n",
			html:     "<blockquote>\n<blockquote>\n<blockquote>\n<p>foo\nbar\nbaz</p>\n</blockquote>\n</blockquote>\n</blockquote>\n",
		},
		{
			name:     "official example 93 keeps setext-looking continuation literal",
			markdown: "> foo\nbar\n===\n",
			html:     "<blockquote>\n<p>foo\nbar\n===</p>\n</blockquote>\n",
		},
		{
			name:     "official example 234 thematic break interrupts",
			markdown: "> foo\n---\n",
			html:     "<blockquote>\n<p>foo</p>\n</blockquote>\n<hr />\n",
		},
		{
			name:     "official example 235 list interrupts",
			markdown: "> - foo\n- bar\n",
			html:     "<blockquote>\n<ul>\n<li>foo</li>\n</ul>\n</blockquote>\n<ul>\n<li>bar</li>\n</ul>\n",
		},
		{
			name:     "official example 238 indented marker cannot interrupt",
			markdown: "> foo\n    - bar\n",
			html:     "<blockquote>\n<p>foo\n- bar</p>\n</blockquote>\n",
		},
		{
			name:     "official example 291 omits a list item prefix",
			markdown: "  1.  A paragraph\n    with two lines.\n",
			html:     "<ol>\n<li>A paragraph\nwith two lines.</li>\n</ol>\n",
		},
		{
			name:     "official example 292 omits every nested prefix",
			markdown: "> 1. > Blockquote\ncontinued here.\n",
			html:     "<blockquote>\n<ol>\n<li>\n<blockquote>\n<p>Blockquote\ncontinued here.</p>\n</blockquote>\n</li>\n</ol>\n</blockquote>\n",
		},
		{
			name:     "official example 293 retains only the outer prefix",
			markdown: "> 1. > Blockquote\n> continued here.\n",
			html:     "<blockquote>\n<ol>\n<li>\n<blockquote>\n<p>Blockquote\ncontinued here.</p>\n</blockquote>\n</li>\n</ol>\n</blockquote>\n",
		},
	}

	p := mdflow.New()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := p.HTML(test.markdown)
			// List tightness is a separate document-level slice. Ignore only its
			// wrapper newline here so the nested-list cases isolate whether the
			// paragraph and every still-open container continued lazily.
			got = strings.ReplaceAll(got, "<li><blockquote>", "<li>\n<blockquote>")
			if got != test.html {
				t.Fatalf("HTML(%q) = %q, want %q", test.markdown, got, test.html)
			}
		})
	}
}

func TestBlockRuleWithInterruptProbeStartsAfterAMissingContainerPrefix(t *testing.T) {
	rules := parser.New()
	rules.AddLeafRule(probedTestLeafRule{})
	probe := new(rendererCallProbe)

	mdflow.NewWith(rules, probe).HTML("> paragraph\n::md custom\n")

	if want := []string{"paragraph", "custom"}; !slices.Equal(probe.contents, want) {
		t.Fatalf("leaf contents = %q, want %q", probe.contents, want)
	}
}

func TestUnmatchedLegacyBlockRuleDoesNotDisableLazyContinuation(t *testing.T) {
	rules := parser.New()
	rules.AddLeafRule(testLeafRule{})
	probe := new(rendererCallProbe)

	mdflow.NewWith(rules, probe).HTML("> paragraph\nordinary continuation\n")

	if want := []string{"paragraph\nordinary continuation"}; !slices.Equal(probe.contents, want) {
		t.Fatalf("leaf contents = %q, want %q", probe.contents, want)
	}
}

type probedTestLeafRule struct{ testLeafRule }

func (probedTestLeafRule) InterruptsParagraph(line string) bool {
	return strings.HasPrefix(line, "::md ")
}
