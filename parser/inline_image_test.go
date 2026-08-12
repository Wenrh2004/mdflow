package parser

import (
	"reflect"
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func TestImageBracketStackKeepsNestedDescriptionFlat(t *testing.T) {
	const src = `![foo ![bar](/inner) [link](/link)](/outer "title")`
	got := New().Inline().Parse(src)
	want := []token.Inline{
		{Node: token.Image, Dest: "/outer", Title: "title"},
		{Node: token.Text, Text: "foo "},
		{Node: token.Image, Dest: "/inner"},
		{Node: token.Text, Text: "bar"},
		{Node: token.Image, Close: true},
		{Node: token.Text, Text: " "},
		{Node: token.Link, Dest: "/link"},
		{Node: token.Text, Text: "link"},
		{Node: token.Link, Close: true},
		{Node: token.Image, Close: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse(%q)\n got: %#v\nwant: %#v", src, got, want)
	}
}
