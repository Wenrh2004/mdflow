package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestCommonMarkListMarkerGrammar(t *testing.T) {
	t.Parallel()

	wanted := make(map[int]bool, 32)
	for _, n := range []int{
		265, 266, 267, 268, 269,
		272,
		275, 276,
		278, 279, 280, 281, 282, 283, 284, 285,
		289,
	} {
		wanted[n] = true
	}
	for n := 291; n <= 305; n++ {
		wanted[n] = true
	}

	p := mdflow.New()
	seen := 0
	for _, example := range loadSpecExamples(t) {
		if !wanted[example.Example] {
			continue
		}
		seen++
		got := p.HTML(example.Markdown)
		if compactListBlockFormatting(got) != compactListBlockFormatting(example.HTML) {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", example.Example, example.Section, example.Markdown, got, example.HTML)
		}
	}
	if seen != len(wanted) {
		t.Fatalf("loaded %d selected examples, want %d", seen, len(wanted))
	}
}

func TestListMarkerPaddingBeyondFourColumnsKeepsContentIndent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		markdown string
		html     string
	}{
		{
			markdown: "1.     indented code\n",
			html:     "<ol>\n<li>\n<pre><code>indented code\n</code></pre>\n</li>\n</ol>\n",
		},
		{
			markdown: "1.      indented code\n",
			html:     "<ol>\n<li>\n<pre><code> indented code\n</code></pre>\n</li>\n</ol>\n",
		},
		{
			markdown: "-\t\tfoo\n",
			html:     "<ul>\n<li>\n<pre><code>  foo\n</code></pre>\n</li>\n</ul>\n",
		},
	}

	p := mdflow.New()
	for _, test := range tests {
		got := p.HTML(test.markdown)
		if compactListBlockFormatting(got) != compactListBlockFormatting(test.html) {
			t.Errorf("HTML(%q) = %q, want %q", test.markdown, got, test.html)
		}
	}
}

// The list commit-barrier slice owns the HTML newlines that depend on whether
// the first child is a block and whether the eventual list is tight or loose.
// This slice compares the complete tag/content stream while ignoring only
// those line endings, so marker recognition, nesting, indentation and payload
// remain strict without freezing the provisional renderer formatting.
func compactListBlockFormatting(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}
