package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/token"
)

// coreRenderer implements renderer.Renderer and nothing else: no Cloner, no
// CustomRegistrar. It stands in for any output format that is not HTML, and it
// is the reason the capability interfaces exist — the facade and the extensions
// used to reach for *html.Renderer directly, so everything they configured was
// silently discarded for a renderer like this one.
type coreRenderer struct{}

func (c *coreRenderer) RenderLeaf(w renderer.Writer, leaf token.Leaf, inlines []token.Inline) {
	if leaf.Node == token.CodeBlock {
		w.WriteString(leaf.Content)
		return
	}
	c.RenderInlines(w, inlines)
	w.WriteString("\n")
}

func (c *coreRenderer) RenderContainer(renderer.Writer, token.BlockEvent) {}

func (c *coreRenderer) RenderInlines(w renderer.Writer, toks []token.Inline) {
	for _, t := range toks {
		if t.Text != "" {
			w.WriteString(t.Text)
		}
	}
}

// The tags and rules below are the smallest thing that exercises the core's
// custom-node seam without importing any extension module: an inline pair, a
// markdown leaf and a literal leaf. They let the capability mechanism be tested
// against the public parser seam alone, the way an out-of-tree extension writes
// its own.
var (
	testInlineTag = token.NewTag("test_inline")
	testLeafTag   = token.NewTag("test_leaf")
	testLitTag    = token.NewTag("test_lit")
)

// testInlineRule matches `@@text@@` and emits a paired custom node, parsing its
// content recursively — the shape strikethrough and the typography wrappers use.
type testInlineRule struct{}

func (testInlineRule) Name() string     { return "test_inline" }
func (testInlineRule) Triggers() []byte { return []byte{'@'} }
func (testInlineRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if !strings.HasPrefix(src[i:], "@@") {
		return false
	}
	closeIdx := strings.Index(src[i+2:], "@@")
	if closeIdx < 0 {
		return false
	}
	inner := s.Parse(src[i+2 : i+2+closeIdx])
	s.Emit(token.Inline{Node: token.Custom, Tag: testInlineTag})
	s.EmitAll(inner)
	s.Emit(token.Inline{Node: token.Custom, Tag: testInlineTag, Close: true})
	s.Advance(2 + closeIdx + 2)
	return true
}

// testLeafRule opens a markdown custom leaf on a `::md ` line: its content is
// inline-parsed, so it carries the same text in both Content and its parsed
// inlines — the case a naive fallback double-emits.
type testLeafRule struct{}

func (testLeafRule) Name() string { return "test_leaf" }
func (testLeafRule) Open(s *parser.BlockState, line string) bool {
	rest, ok := strings.CutPrefix(line, "::md ")
	if !ok {
		return false
	}
	s.EmitLeaf(token.Leaf{Node: token.CustomLeaf, Tag: testLeafTag, Content: rest})
	return true
}

// testLitRule opens a literal custom leaf on a `::lit ` line: its body is
// verbatim, held in Content with no inlines, the way a math block is.
type testLitRule struct{}

func (testLitRule) Name() string { return "test_lit" }
func (testLitRule) Open(s *parser.BlockState, line string) bool {
	rest, ok := strings.CutPrefix(line, "::lit ")
	if !ok {
		return false
	}
	s.EmitLeaf(token.Leaf{Node: token.CustomLeaf, Tag: testLitTag, Content: rest, Literal: true})
	return true
}

// A renderer that cannot host custom inline nodes must still render the
// document. Extensions whose output half cannot apply degrade to no output
// wiring — never to a panic and never to lost prose.
func TestRendererWithoutCapabilitiesStillRenders(t *testing.T) {
	p := mdflow.NewWith(parser.New(), &coreRenderer{})

	got := p.HTML("hello *world*\n")
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("core-only renderer lost prose: %q", got)
	}
}

// The registration helpers report whether the renderer could take the
// registration. That boolean is the whole point: a renderer that cannot host a
// custom node is a detectable condition, where the old *html.Renderer type
// assertion made it an invisible one.
func TestRegistrationReportsCapability(t *testing.T) {
	if ok := renderer.RegisterCustom(&coreRenderer{}, testInlineTag, func(renderer.Writer, token.Inline) {}); ok {
		t.Error("core-only renderer reported that it accepted a custom inline registration")
	}
	if ok := renderer.RegisterCustom(html.NewRenderer(), testInlineTag, func(renderer.Writer, token.Inline) {}); !ok {
		t.Error("html renderer reported that it could not accept a custom inline registration")
	}
}

func TestVoidElementClosingIsAFunctionalCapability(t *testing.T) {
	var out strings.Builder
	if renderer.CloseVoidElement(&coreRenderer{}, &out) {
		t.Fatal("core-only renderer reported a void-element convention")
	}

	h := html.NewRenderer()
	if !renderer.CloseVoidElement(h, &out) || out.String() != " />" {
		t.Fatalf("XHTML void close = %q", out.String())
	}
	out.Reset()
	h.XHTML = false
	if !renderer.CloseVoidElement(h, &out) || out.String() != ">" {
		t.Fatalf("HTML5 void close = %q", out.String())
	}
}

// Content preservation: a custom inline node whose rendering was never
// registered must still emit its text. Dropping the node entirely would delete
// the author's prose to punish a configuration mistake.
func TestUnregisteredCustomTagKeepsItsText(t *testing.T) {
	rules := parser.New()
	rules.AddInlineRule(testInlineRule{}) // syntax on, rendering deliberately absent

	got := mdflow.NewWith(rules, html.NewRenderer()).HTML("@@gone@@\n")
	if !strings.Contains(got, "gone") {
		t.Errorf("unregistered custom tag swallowed its content: %q", got)
	}
}

// Cloner is what lets a derived parser keep its own renderer. Without it, Use
// shared one pointer and mutations leaked back into the receiver.
func TestClonerIsHonoured(t *testing.T) {
	if _, ok := any(html.NewRenderer()).(renderer.Cloner); !ok {
		t.Error("html.Renderer does not implement renderer.Cloner")
	}
}

// A custom leaf whose rendering was never registered emits its content once —
// not twice. A markdown leaf carries the same text in both Content (unparsed)
// and inlines (parsed), so a fallback that wrote both duplicated every cell.
func TestUnregisteredCustomLeafDoesNotDuplicate(t *testing.T) {
	rules := parser.New()
	rules.AddLeafRule(testLeafRule{}) // syntax on, rendering deliberately absent

	got := mdflow.NewWith(rules, html.NewRenderer()).HTML("::md hello\n")
	if want := "hello"; got != want {
		t.Errorf("content emitted more than once\n got: %q\nwant: %q", got, want)
	}

	// A literal leaf keeps its body in Content and has no inlines.
	literal := parser.New()
	literal.AddLeafRule(testLitRule{})
	if got, want := mdflow.NewWith(literal, html.NewRenderer()).HTML("::lit x\n"), "x"; got != want {
		t.Errorf("literal leaf: got %q want %q", got, want)
	}
}
