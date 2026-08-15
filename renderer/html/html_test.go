package html_test

import (
	"strings"
	"testing"

	mdhtml "github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/token"
)

func TestCommonMarkEscapesDoubleQuoteAndPreservesApostrophe(t *testing.T) {
	tokens := []token.Inline{
		{Node: token.Text, Text: `say "go" and 'stay' `},
		{Node: token.Link, Dest: `a"b`, Title: `c"d`},
		{Node: token.Text, Text: "link"},
		{Node: token.Link, Close: true},
	}

	var got strings.Builder
	mdhtml.NewRenderer().RenderInlines(&got, tokens)
	const want = `say &quot;go&quot; and 'stay' <a href="a%22b" title="c&quot;d">link</a>`
	if got.String() != want {
		t.Fatalf("RenderInlines() = %q, want %q", got.String(), want)
	}
}

func TestCommonMarkURLEscaping(t *testing.T) {
	cases := []struct {
		name string
		dest string
		want string
	}{
		{
			name: "pulldown ASCII allowlist",
			dest: "!#$%()*+,-./0123456789:;=?@AZ^_az~",
			want: "!#$%()*+,-./0123456789:;=?@AZ^_az~",
		},
		{
			name: "all percent spellings stay verbatim",
			dest: "%20%2f%GZ%",
			want: "%20%2f%GZ%",
		},
		{
			name: "HTML entities in href",
			dest: "&'",
			want: "&amp;&#x27;",
		},
		{
			name: "unsafe ASCII uses uppercase percent escapes",
			dest: " \"<>[]\\`{}|\x7f",
			want: "%20%22%3C%3E%5B%5D%5C%60%7B%7D%7C%7F",
		},
		{
			name: "UTF-8 is encoded by byte",
			dest: "föö",
			want: "f%C3%B6%C3%B6",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := []token.Inline{
				{Node: token.Link, Dest: tc.dest},
				{Node: token.Text, Text: "x"},
				{Node: token.Link, Close: true},
			}
			var got strings.Builder
			mdhtml.NewRenderer().RenderInlines(&got, tokens)
			want := `<a href="` + tc.want + `">x</a>`
			if got.String() != want {
				t.Fatalf("RenderInlines(Dest: %q) = %q, want %q", tc.dest, got.String(), want)
			}
		})
	}
}
