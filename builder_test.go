package mdflow_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	internalrawhtml "github.com/Wenrh2004/mdflow/internal/rawhtml"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/token"
)

// testCap is a self-contained capability — one inline rule and its paired
// markup — used to exercise the facade's wiring without importing any extension
// module. It reuses testInlineRule / testInlineTag from capability_test.go.
var testCap = extension.Capability{
	Name:   "test",
	Syntax: func(p *parser.RuleSet) { p.AddInlineRule(testInlineRule{}) },
	Output: func(r renderer.Renderer) { extension.Paired(r, testInlineTag, "<test>", "</test>") },
}

// Handing New a renderer must not orphan a capability's output. Previously the
// default set was applied to the renderer New had constructed, and WithRenderer
// then replaced that renderer — so every registered tag was silently discarded.
// Capabilities now apply after the renderer settles, whatever the option order.
func TestWithRendererKeepsCapabilityOutput(t *testing.T) {
	got := mdflow.New(mdflow.WithExtensions(testCap), mdflow.WithRenderer(html.NewRenderer())).HTML("@@gone@@\n")
	if want := "<p><test>gone</test></p>\n"; got != want {
		t.Errorf("custom tag lost when a renderer was supplied\n got: %q\nwant: %q", got, want)
	}
}

// Functional options must commute. They configure; they do not race.
func TestOptionsAreOrderIndependent(t *testing.T) {
	cases := []struct {
		name string
		a, b *mdflow.Parser
		in   string
	}{
		{
			"renderer then html5",
			mdflow.New(mdflow.WithRenderer(html.NewRenderer()), mdflow.WithHTML5()),
			mdflow.New(mdflow.WithHTML5(), mdflow.WithRenderer(html.NewRenderer())),
			"---\n",
		},
		{
			"extensions then renderer",
			mdflow.New(mdflow.WithExtensions(testCap), mdflow.WithRenderer(html.NewRenderer())),
			mdflow.New(mdflow.WithRenderer(html.NewRenderer()), mdflow.WithExtensions(testCap)),
			"@@gone@@\n",
		},
		{
			"unsafe output then only raw HTML",
			mdflow.New(mdflow.WithOutput(internalrawhtml.UnsafeHTML), mdflow.WithOnly(internalrawhtml.RawHTML)),
			mdflow.New(mdflow.WithOnly(internalrawhtml.RawHTML), mdflow.WithOutput(internalrawhtml.UnsafeHTML)),
			"<i>trusted</i>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if x, y := tc.a.HTML(tc.in), tc.b.HTML(tc.in); x != y {
				t.Errorf("option order changed the result\n one order: %q\nthe other: %q", x, y)
			}
		})
	}
}

// WithHTML5 has to reach the renderer that actually ends up in use.
func TestWithHTML5Applies(t *testing.T) {
	if got, want := mdflow.New(mdflow.WithHTML5()).HTML("---\n"), "<hr>\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got, want := mdflow.New().HTML("---\n"), "<hr />\n"; got != want {
		t.Errorf("default should stay XHTML: got %q want %q", got, want)
	}
}

// The builder is the same wiring by another spelling, so it must agree with New
// call for call.
func TestBuilderMatchesNew(t *testing.T) {
	src := "# t\n\npara *x* `c` [l](/u)\n\n- a\n- b\n"
	if got, want := mdflow.NewBuilder().Build().HTML(src), mdflow.New().HTML(src); got != want {
		t.Errorf("NewBuilder().Build() differs from New()\n got: %q\nwant: %q", got, want)
	}
	viaBuilder := mdflow.NewBuilder().Renderer(html.NewRenderer()).With(mdflow.WithHTML5()).Build()
	viaNew := mdflow.New(mdflow.WithRenderer(html.NewRenderer()), mdflow.WithHTML5())
	if got, want := viaBuilder.HTML("---\n"), viaNew.HTML("---\n"); got != want {
		t.Errorf("builder and options disagree: %q vs %q", got, want)
	}
}

// Only replaces the default capability set instead of adding to it. With no base
// set, Only(testCap) turns the one capability on and nothing else.
func TestBuilderOnlyReplacesDefaults(t *testing.T) {
	p := mdflow.NewBuilder().Use(testCap).Only().Build()
	// Only() with no arguments discards testCap, so the node stays literal text.
	if got, want := p.HTML("@@gone@@\n"), "<p>@@gone@@</p>\n"; got != want {
		t.Errorf("Only() should discard added capabilities: got %q want %q", got, want)
	}
	q := mdflow.NewBuilder().Only(testCap).Build()
	if got, want := q.HTML("@@gone@@\n"), "<p><test>gone</test></p>\n"; got != want {
		t.Errorf("Only(testCap) should turn it on: got %q want %q", got, want)
	}
}

// A Builder configures copies of caller-supplied components. Otherwise a
// trusted parser can overwrite the raw-HTML handlers of an already-built safe
// parser, and a rule set once given the default profile can make a later
// WithOnly() parser retain raw-HTML syntax it did not request.
func TestBuildDoesNotMutateSuppliedComponents(t *testing.T) {
	rules := parser.New()
	renderer := html.NewRenderer()

	safe := mdflow.New(mdflow.WithRules(rules), mdflow.WithRenderer(renderer))
	_ = mdflow.New(
		mdflow.WithRules(rules),
		mdflow.WithRenderer(renderer),
		mdflow.WithOutput(internalrawhtml.UnsafeHTML),
	)

	const src = "<script>alert(1)</script>\n"
	if got, want := safe.HTML(src), "&lt;script&gt;alert(1)&lt;/script&gt;\n"; got != want {
		t.Fatalf("a later trusted parser changed an existing safe parser\n got: %q\nwant: %q", got, want)
	}

	only := mdflow.New(
		mdflow.WithRules(rules),
		mdflow.WithRenderer(renderer),
		mdflow.WithOnly(),
	)
	if got, want := only.HTML("<div>\nx\n\n"), "<p>&lt;div&gt;\nx</p>\n"; got != want {
		t.Fatalf("caller-owned state leaked through WithOnly()\n got: %q\nwant: %q", got, want)
	}
}

// Deriving a parser must never reach back into the receiver — including when
// the renderer is not the HTML one. This used to hold only for *html.Renderer,
// because the deep copy was behind a concrete type assertion.
func TestWithExtensionsDoesNotLeakIntoReceiver(t *testing.T) {
	base := mdflow.New()
	derived := base.WithExtensions(extension.Capability{
		Name: "italic-emphasis",
		Output: func(r renderer.Renderer) {
			renderer.OverrideNode(r, token.Emph, func(w renderer.Writer, n token.Inline) {
				w.WriteString(map[bool]string{true: "</i>", false: "<i>"}[n.Close])
			})
		},
	})

	if got, want := derived.HTML("*a*\n"), "<p><i>a</i></p>\n"; got != want {
		t.Errorf("derived: got %q want %q", got, want)
	}
	if got, want := base.HTML("*a*\n"), "<p><em>a</em></p>\n"; got != want {
		t.Errorf("receiver was mutated by deriving: got %q want %q", got, want)
	}
}

// The same guarantee for a renderer that is not html.Renderer. A renderer with
// no Cloner is shared by contract, so use one that has state worth protecting.
func TestDerivationIsolatesRuleSetForAnyRenderer(t *testing.T) {
	base := mdflow.NewWith(parser.New(), &coreRenderer{})
	_ = base.WithExtensions(testCap)

	// base never had the test rule; deriving must not have added it. coreRenderer
	// emits inline text only, so a leaked rule would drop the `@@` delimiters.
	if got := base.HTML("@@gone@@\n"); got != "@@gone@@\n" {
		t.Errorf("deriving leaked syntax into the receiver's rule set: %q", got)
	}
}
