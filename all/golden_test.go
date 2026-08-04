package all_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/renderer/text"
)

// corpusForRenderers exercises CommonMark plus every bundled extension, so both
// renderers below are driven by the identical syntax half — the point of the
// parser/renderer split.
const corpusForRenderers = "# Report\n\n" +
	"A paragraph with *emphasis*, **strong**, `code`, a #tag and inline math $a+b$.\n\n" +
	"- one\n- two with ~~struck~~ text\n\n" +
	"| name | value |\n| --- | ---: |\n| alpha | 1 |\n\n" +
	"> a quote with ==mark==\n\n" +
	"```go\nfmt.Println(1)\n```\n"

// TestSameSyntaxTwoRenderers renders one corpus through the HTML renderer and
// the plain-text renderer using the *same* extension syntax, and pins both. It
// is the golden proof that Phase 3's syntax/output split holds: a second output
// format reuses every extension's Syntax half unchanged and only swaps the
// renderer.
func TestSameSyntaxTwoRenderers(t *testing.T) {
	rules := parser.New()
	all.All.Rules(rules)

	htmlParser := mdflow.NewWith(rules, html.NewRenderer(), all.All)
	textParser := mdflow.NewWith(rules, text.NewRenderer(), all.All)

	const wantHTML = "<h1>Report</h1>\n" +
		`<p>A paragraph with <em>emphasis</em>, <strong>strong</strong>, <code>code</code>, ` +
		`a <span class="tag">#tag</span> and inline math <code class="language-math">a+b</code>.</p>` + "\n" +
		"<ul>\n<li>one</li>\n<li>two with <del>struck</del> text</li>\n</ul>\n" +
		"<table>\n<thead>\n<tr>\n<th>name</th>\n<th align=\"right\">value</th>\n</tr>\n</thead>\n" +
		"<tbody>\n<tr>\n<td>alpha</td>\n<td align=\"right\">1</td>\n</tr>\n</tbody>\n</table>\n" +
		"<blockquote>\n<p>a quote with <mark>mark</mark></p>\n</blockquote>\n" +
		`<pre><code class="language-go">fmt.Println(1)` + "\n</code></pre>\n"

	if got := htmlParser.HTML(corpusForRenderers); got != wantHTML {
		t.Errorf("HTML render mismatch\n got: %q\nwant: %q", got, wantHTML)
	}

	// The text renderer hosts no custom-node output, so extension nodes fall
	// back to their own text: the #tag becomes "tag", the math "a+b", the struck
	// run keeps its words, table cells each land on their own line, and every
	// markup glyph is gone.
	const wantText = "Report\n\n" +
		"A paragraph with emphasis, strong, code, a tag and inline math a+b.\n\n" +
		"- one\n- two with struck text\n\n" +
		"name\nvalue\nalpha\n1\n" +
		"a quote with mark\n\n" +
		"fmt.Println(1)\n\n"

	if got := textParser.HTML(corpusForRenderers); got != wantText {
		t.Errorf("text render mismatch\n got: %q\nwant: %q", got, wantText)
	}
}
