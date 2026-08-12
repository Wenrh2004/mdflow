package mdflow_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

type contextRecorder struct {
	paragraphs []token.InlineContext
}

func (r *contextRecorder) RenderLeaf(_ renderer.Writer, leaf token.Leaf, _ []token.Inline) {
	if leaf.Node == token.Paragraph {
		r.paragraphs = append(r.paragraphs, leaf.Context)
	}
}
func (*contextRecorder) RenderContainer(renderer.Writer, token.BlockEvent) {}
func (*contextRecorder) RenderInlines(renderer.Writer, []token.Inline)     {}

func TestInlineContextSurvivesIdentityEventRoundTrip(t *testing.T) {
	const src = "- head\n\n  later\n\noutside\n"
	recorder := &contextRecorder{}
	p := mdflow.New(mdflow.WithOnly(), mdflow.WithRenderer(recorder))

	p.HTML(src)
	want := []token.InlineContext{token.InlineContextListItemHead, 0, 0}
	if !sameContexts(recorder.paragraphs, want) {
		t.Fatalf("fast-path paragraph contexts = %v, want %v", recorder.paragraphs, want)
	}

	recorder.paragraphs = recorder.paragraphs[:0]
	p.Map(func(event mdflow.Event) mdflow.Event { return event }).HTML(src)
	if !sameContexts(recorder.paragraphs, want) {
		t.Fatalf("event round-trip paragraph contexts = %v, want %v", recorder.paragraphs, want)
	}
}

func sameContexts(got, want []token.InlineContext) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
