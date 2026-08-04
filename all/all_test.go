package all_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/gfm"
	"github.com/Wenrh2004/mdflow/extension/hashtag"
	"github.com/Wenrh2004/mdflow/extension/math"
	"github.com/Wenrh2004/mdflow/extension/memos"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
	"github.com/Wenrh2004/mdflow/extension/resource"
	"github.com/Wenrh2004/mdflow/extension/strikethrough"
	"github.com/Wenrh2004/mdflow/extension/table"
	"github.com/Wenrh2004/mdflow/extension/typography"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

// Every bundled capability and bundle satisfies both halves of the interface.
var (
	_ extension.Extension = table.Table
	_ extension.Extension = strikethrough.Strikethrough
	_ extension.Extension = math.Math
	_ extension.Extension = hashtag.Hashtag
	_ extension.Extension = typography.Typography
	_ extension.Extension = resource.Resource
	_ extension.Extension = rawhtml.RawHTML
	_ extension.Extension = gfm.GFM
	_ extension.Extension = memos.Memos
	_ extension.Extension = all.All
)

// A capability is a rule *and* its rendering. Enabling one on an otherwise bare
// CommonMark parser must produce that capability's markup — which fails if
// either half is missing, and is the guarantee that replaced keeping rules and
// rendering in two packages that had to be kept in agreement by hand.
func TestEveryCapabilityShipsBothHalves(t *testing.T) {
	cases := []struct {
		name string
		ext  extension.Extension
		in   string
		want string
	}{
		{"Strikethrough", strikethrough.Strikethrough, "~~gone~~\n", "<p><del>gone</del></p>\n"},
		{"Hashtag", hashtag.Hashtag, "#note\n", "<p><span class=\"tag\">#note</span></p>\n"},
		{
			"Table", table.Table,
			"| a |\n| - |\n| 1 |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td>1</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"Math inline", math.Math, "$x$\n",
			"<p><code class=\"language-math\">x</code></p>\n",
		},
		{
			"Math block", math.Math, "$$\nx\n$$\n",
			"<pre><code class=\"language-math\">x\n</code></pre>\n",
		},
		{"Typography highlight", typography.Typography, "==hi==\n", "<p><mark>hi</mark></p>\n"},
		{"Typography superscript", typography.Typography, "^up^\n", "<p><sup>up</sup></p>\n"},
		{
			"Resource reference", resource.Resource, "[[doc]]\n",
			"<p><span class=\"reference\" data-resource=\"doc\">doc</span></p>\n",
		},
		{"RawHTML escapes", rawhtml.RawHTML, "<b>x</b>\n", "<p>&lt;b&gt;x&lt;/b&gt;</p>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mdflow.NewWith(parser.New(), html.NewRenderer(), tc.ext)
			if got := p.HTML(tc.in); got != tc.want {
				t.Errorf("capability did not deliver both halves\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// The syntax half alone must not render the capability's markup. This is the
// negative control for the test above: without it, a capability whose rendering
// silently came from somewhere else would still pass.
func TestSyntaxWithoutOutputRendersNoMarkup(t *testing.T) {
	rules := parser.New()
	strikethrough.Strikethrough.Rules(rules) // syntax only — Render deliberately not called

	got := mdflow.NewWith(rules, html.NewRenderer()).HTML("~~gone~~\n")
	if got == "<p><del>gone</del></p>\n" {
		t.Error("markup appeared without the capability's Render half ever being applied")
	}
	if want := "<p>gone</p>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// all.New enables the full bundle in the load-bearing order. A document that
// touches every flavour must render each construct, proving the bundle wires
// the whole set and its ordering holds (strikethrough before subscript on `~`,
// raw HTML after autolink on `<`).
func TestAllRendersEveryFlavour(t *testing.T) {
	src := "# T\n\n~~strike~~ and $x$ and #tag and ==mark== and ~sub~ and ^sup^ and ||spoiler||\n\n" +
		"[[ref]] and <https://go.dev>\n\n| a | b |\n| - | - |\n| 1 | 2 |\n"
	got := all.New().HTML(src)
	for _, want := range []string{
		"<del>strike</del>",
		`<code class="language-math">x</code>`,
		`<span class="tag">#tag</span>`,
		"<mark>mark</mark>",
		"<sub>sub</sub>",
		"<sup>sup</sup>",
		"<details><summary>spoiler</summary></details>",
		`<span class="reference" data-resource="ref">ref</span>`,
		`<a href="https://go.dev">https://go.dev</a>`,
		"<table>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("all.New() output missing %q\n got: %q", want, got)
		}
	}
}

// The builder path must agree with all.New call for call over the full syntax.
func TestBuilderMatchesAllNew(t *testing.T) {
	src := "# t\n\n~~x~~ and $y$ and #tag\n\n| a |\n| - |\n| 1 |\n"
	viaBuilder := mdflow.NewBuilder().Only(all.All).Build()
	if got, want := viaBuilder.HTML(src), all.New().HTML(src); got != want {
		t.Errorf("NewBuilder().Only(all.All) differs from all.New()\n got: %q\nwant: %q", got, want)
	}
}

// GFM is a strict subset: tables and strikethrough on, Memos syntax off.
func TestGFMBundleIsSubset(t *testing.T) {
	p := mdflow.New(mdflow.WithOnly(gfm.GFM))
	if got, want := p.HTML("~~gone~~\n"), "<p><del>gone</del></p>\n"; got != want {
		t.Errorf("GFM should be on: got %q want %q", got, want)
	}
	// A hashtag is Memos syntax, not GFM, so it stays literal text. (No space
	// after the '#', so CommonMark does not read it as a heading either.)
	if got, want := p.HTML("#tag\n"), "<p>#tag</p>\n"; got != want {
		t.Errorf("non-GFM syntax should be off: got %q want %q", got, want)
	}
}
