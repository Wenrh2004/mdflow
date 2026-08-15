package parser

import (
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func collectReferenceBlockEvents(t *testing.T, src string) (*BlockState, []token.BlockEvent) {
	t.Helper()
	state := NewBlockState(New())
	var events []token.BlockEvent
	EachLine(src, func(line string) bool {
		events = append(events, state.FeedLine(line)...)
		return true
	})
	events = append(events, state.CloseAll()...)
	return state, events
}

func TestReferenceDefinitionsAreRemovedFromParagraphBlocks(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		wantLeaves  []token.Leaf
		wantRef     string
		wantRefDest string
	}{
		{
			name:    "definition only",
			src:     "[foo]: /url\n",
			wantRef: "foo", wantRefDest: "/url",
		},
		{
			name:       "multiline label followed by visible content",
			src:        "[\nfoo\n]: /url\nbar\n",
			wantLeaves: []token.Leaf{{Node: token.Paragraph, Content: "bar"}},
			wantRef:    "foo", wantRefDest: "/url",
		},
		{
			name:       "same-line garbage rejects the whole definition",
			src:        "[foo]: /url \"title\" ok\n",
			wantLeaves: []token.Leaf{{Node: token.Paragraph, Content: "[foo]: /url \"title\" ok"}},
		},
		{
			name:       "definition cannot interrupt paragraph",
			src:        "Foo\n[bar]: /baz\n",
			wantLeaves: []token.Leaf{{Node: token.Paragraph, Content: "Foo\n[bar]: /baz"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, events := collectReferenceBlockEvents(t, test.src)
			var leaves []token.Leaf
			for _, event := range events {
				if event.Type == token.LeafBlock {
					leaves = append(leaves, event.Leaf)
				}
			}
			if !sameReferenceLeaves(leaves, test.wantLeaves) {
				t.Fatalf("leaves = %#v, want %#v", leaves, test.wantLeaves)
			}
			if test.wantRef == "" {
				if len(state.references.definitions) != 0 {
					t.Fatalf("unexpected definitions: %#v", state.references.definitions)
				}
				return
			}
			definition, ok := state.references.lookup(test.wantRef)
			if !ok || definition.destination != test.wantRefDest {
				t.Fatalf("lookup(%q) = %#v, %v; want destination %q", test.wantRef, definition, ok, test.wantRefDest)
			}
		})
	}
}

func TestReferenceDefinitionsAreRemovedBeforeSetextClassification(t *testing.T) {
	t.Run("visible remainder becomes heading", func(t *testing.T) {
		state, events := collectReferenceBlockEvents(t, "[foo]: /url\nbar\n===\n[foo]\n")
		want := []token.Leaf{
			{Node: token.Heading, Level: 1, Content: "bar"},
			{Node: token.Paragraph, Content: "[foo]"},
		}
		var leaves []token.Leaf
		for _, event := range events {
			if event.Type == token.LeafBlock {
				leaves = append(leaves, event.Leaf)
			}
		}
		if !sameReferenceLeaves(leaves, want) {
			t.Fatalf("leaves = %#v, want %#v", leaves, want)
		}
		if _, ok := state.references.lookup("foo"); !ok {
			t.Fatal("setext path did not register definition")
		}
	})

	t.Run("definition-only paragraph leaves underline as text", func(t *testing.T) {
		_, events := collectReferenceBlockEvents(t, "[foo]: /url\n===\n[foo]\n")
		want := []token.Leaf{{Node: token.Paragraph, Content: "===\n[foo]"}}
		var leaves []token.Leaf
		for _, event := range events {
			if event.Type == token.LeafBlock {
				leaves = append(leaves, event.Leaf)
			}
		}
		if !sameReferenceLeaves(leaves, want) {
			t.Fatalf("leaves = %#v, want %#v", leaves, want)
		}
	})
}

func TestReferenceDefinitionInsideBlockquoteIsDocumentGlobal(t *testing.T) {
	state, events := collectReferenceBlockEvents(t, "[foo]\n\n> [foo]: /url\n")
	definition, ok := state.references.lookup("foo")
	if !ok || definition.destination != "/url" {
		t.Fatalf("global lookup = %#v, %v", definition, ok)
	}
	for _, event := range events {
		if event.Type == token.LeafBlock && event.Leaf.Content == "[foo]: /url" {
			t.Fatal("definition inside blockquote leaked as visible paragraph")
		}
	}
}

func TestInvisibleReferenceDefinitionStillConsumesListItemHeadContext(t *testing.T) {
	_, events := collectReferenceBlockEvents(t, "- [ref]: /url\n\n  [x] later\n")
	for _, event := range events {
		if event.Type != token.LeafBlock || event.Leaf.Node != token.Paragraph {
			continue
		}
		if event.Leaf.Context&token.InlineContextListItemHead != 0 {
			t.Fatalf("later paragraph inherited list-item-head context after a definition: %#v", event.Leaf)
		}
		return
	}
	t.Fatal("visible paragraph not emitted")
}

func sameReferenceLeaves(got, want []token.Leaf) bool {
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
