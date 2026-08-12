package mdflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

type rendererCallProbe struct {
	leaves     []token.Node
	contents   []string
	infos      []string
	containers []token.BlockEvent
}

func (r *rendererCallProbe) RenderLeaf(_ renderer.Writer, leaf token.Leaf, _ []token.Inline) {
	r.leaves = append(r.leaves, leaf.Node)
	r.contents = append(r.contents, leaf.Content)
	r.infos = append(r.infos, leaf.Info)
}

func TestEventRoundTripPreservesRawLeafContentAndContainerMetadata(t *testing.T) {
	rules := parser.New()
	rules.AddLeafRule(metadataLeafRule{})
	const src = "paragraph\n\n# heading\n\n::meta custom\n\n> quote\n"

	direct := new(rendererCallProbe)
	mdflow.NewWith(rules, direct).HTML(src)
	piped := new(rendererCallProbe)
	mdflow.NewWith(rules, piped).
		Map(func(event mdflow.Event) mdflow.Event { return event }).
		HTML(src)

	if !slices.Equal(piped.contents, direct.contents) {
		t.Fatalf("identity leaf contents = %q, direct = %q", piped.contents, direct.contents)
	}
	if !slices.Equal(piped.infos, direct.infos) {
		t.Fatalf("identity leaf info = %q, direct = %q", piped.infos, direct.infos)
	}
	if !slices.Equal(piped.containers, direct.containers) {
		t.Fatalf("identity containers = %+v, direct = %+v", piped.containers, direct.containers)
	}
}

type metadataLeafRule struct{}

func (metadataLeafRule) Name() string { return "metadata_leaf" }
func (metadataLeafRule) Open(s *parser.BlockState, line string) bool {
	content, ok := strings.CutPrefix(line, "::meta ")
	if !ok {
		return false
	}
	s.EmitLeaf(token.Leaf{
		Node: token.CustomLeaf, Tag: testLeafTag, Content: content, Info: "metadata",
	})
	return true
}

func (r *rendererCallProbe) RenderContainer(_ renderer.Writer, event token.BlockEvent) {
	r.containers = append(r.containers, event)
}

func (*rendererCallProbe) RenderInlines(renderer.Writer, []token.Inline) {}

func TestThematicBreakEventRoundTripDoesNotForgeAContainer(t *testing.T) {
	probe := new(rendererCallProbe)
	p := mdflow.NewWith(parser.New(), probe).
		Map(func(event mdflow.Event) mdflow.Event { return event })
	p.HTML("---\n")

	if got, want := probe.leaves, []token.Node{token.ThematicBreak}; !equalNodes(got, want) {
		t.Fatalf("leaf calls = %v, want %v", got, want)
	}
	if len(probe.containers) != 0 {
		t.Fatalf("thematic-break event pair forged container calls: %+v", probe.containers)
	}
}

func TestUnwrapUsesTheOpeningEventDecisionForItsMatchingLeave(t *testing.T) {
	probe := new(rendererCallProbe)
	p := mdflow.NewWith(parser.New(), probe).
		Transform(mdflow.Unwrap(mdflow.AtLevel(2)))
	p.HTML("## unwrapped\n\nkept\n")

	if len(probe.containers) != 0 {
		t.Fatalf("Unwrap forged container calls: %+v", probe.containers)
	}
	if got, want := probe.contents, []string{"kept"}; !slices.Equal(got, want) {
		t.Fatalf("remaining leaf contents = %q, want %q", got, want)
	}
}

func equalNodes(a, b []token.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
