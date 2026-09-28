package parser

import (
	"reflect"
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func referenceLeaf(src string) token.Leaf {
	return token.Leaf{Node: token.Paragraph, Content: src}
}

func TestInlineReferenceFormsUseDocumentDefinitions(t *testing.T) {
	state := newBlockState(New())
	state.references.define("foo", referenceDefinition{destination: "/url", title: "title"})
	state.references.define("bar", referenceDefinition{destination: "/bar"})
	state.sealReferences()

	tests := []struct {
		name string
		src  string
		want []token.Inline
	}{
		{
			name: "shortcut",
			src:  "[foo]",
			want: []token.Inline{
				{Node: token.Link, Dest: "/url", Title: "title"},
				{Node: token.Text, Text: "foo"},
				{Node: token.Link, Close: true},
			},
		},
		{
			name: "collapsed",
			src:  "[foo][]",
			want: []token.Inline{
				{Node: token.Link, Dest: "/url", Title: "title"},
				{Node: token.Text, Text: "foo"},
				{Node: token.Link, Close: true},
			},
		},
		{
			name: "full",
			src:  "[foo][bar]",
			want: []token.Inline{
				{Node: token.Link, Dest: "/bar"},
				{Node: token.Text, Text: "foo"},
				{Node: token.Link, Close: true},
			},
		},
		{
			name: "image",
			src:  "![foo]",
			want: []token.Inline{
				{Node: token.Image, Dest: "/url", Title: "title"},
				{Node: token.Text, Text: "foo"},
				{Node: token.Image, Close: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := state.parseInlineFinal(referenceLeaf(test.src))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("tokens for %q\n got: %#v\nwant: %#v", test.src, got, test.want)
			}
		})
	}
}

func TestInlineReferenceCursorResumesWithoutRescanningTheLeaf(t *testing.T) {
	state := newBlockState(New())
	_, cursor := state.startInline(referenceLeaf("[foo] and [bar]"))
	if cursor == nil {
		t.Fatal("unknown shortcut reference did not pause")
	}
	if got, complete := cursor.Resume(); complete || got != nil {
		t.Fatalf("unrelated resume = %#v, %v; want blocked", got, complete)
	}

	state.references.define("foo", referenceDefinition{destination: "/foo"})
	if got, complete := cursor.Resume(); complete || got != nil {
		t.Fatalf("cursor should advance to the second unresolved label, got %#v, %v", got, complete)
	}
	if cursor.state.pending.key != "bar" {
		t.Fatalf("pending key = %q, want bar", cursor.state.pending.key)
	}

	state.references.define("bar", referenceDefinition{destination: "/bar"})
	got, complete := cursor.Resume()
	if !complete {
		t.Fatal("both definitions are known, cursor stayed blocked")
	}
	want := []token.Inline{
		{Node: token.Link, Dest: "/foo"},
		{Node: token.Text, Text: "foo"},
		{Node: token.Link, Close: true},
		{Node: token.Text, Text: " and "},
		{Node: token.Link, Dest: "/bar"},
		{Node: token.Text, Text: "bar"},
		{Node: token.Link, Close: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resumed tokens\n got: %#v\nwant: %#v", got, want)
	}
}

func TestInlineReferenceCursorFallsBackOnlyWhenDefinitionsSeal(t *testing.T) {
	state := newBlockState(New())
	_, cursor := state.startInline(referenceLeaf("before [missing] after"))
	if cursor == nil {
		t.Fatal("unknown reference did not pause")
	}
	state.sealReferences()
	got, complete := cursor.Resume()
	if !complete {
		t.Fatal("sealed missing reference stayed blocked")
	}
	want := []token.Inline{{Node: token.Text, Text: "before [missing] after"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("literal fallback = %#v, want %#v", got, want)
	}
}

func TestUndefinedFullReferenceSuppressesShortcut(t *testing.T) {
	state := newBlockState(New())
	state.references.define("foo", referenceDefinition{destination: "/shortcut"})
	state.sealReferences()
	got := state.parseInlineFinal(referenceLeaf("[foo][missing]"))
	want := []token.Inline{{Node: token.Text, Text: "[foo][missing]"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("undefined full reference fell back to shortcut\n got: %#v\nwant: %#v", got, want)
	}
}

func TestInlineReferenceCursorCloneIsSpeculative(t *testing.T) {
	live := newBlockState(New())
	_, cursor := live.startInline(referenceLeaf("[foo]"))
	if cursor == nil {
		t.Fatal("unknown reference did not pause")
	}

	snapshot := live.clone()
	clone := cursor.cloneFor(snapshot)
	snapshot.references.define("foo", referenceDefinition{destination: "/snapshot"})
	got, complete := clone.Resume()
	if !complete || len(got) == 0 || got[0].Dest != "/snapshot" {
		t.Fatalf("snapshot cursor = %#v, %v", got, complete)
	}
	if got, complete := cursor.Resume(); complete || got != nil {
		t.Fatalf("snapshot definition polluted live cursor: %#v, %v", got, complete)
	}
	cursor.Release()
}
