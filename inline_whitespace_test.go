package mdflow_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestCommonMarkInlineWhitespaceStaysContextSensitive(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "official code span example 335",
			src:  "``\nfoo\nbar  \nbaz\n``\n",
			want: "<p><code>foo bar   baz</code></p>\n",
		},
		{
			name: "official code span example 336",
			src:  "``\nfoo \n``\n",
			want: "<p><code>foo </code></p>\n",
		},
		{
			name: "official code span example 337",
			src:  "`foo   bar \nbaz`\n",
			want: "<p><code>foo   bar  baz</code></p>\n",
		},
		{
			name: "official hard break example 640 stays inside code",
			src:  "`code  \nspan`\n",
			want: "<p><code>code   span</code></p>\n",
		},
		{
			name: "official soft break example 649 drops one trailing space",
			src:  "foo \n baz\n",
			want: "<p>foo\nbaz</p>\n",
		},
		{
			name: "two trailing spaces remain a hard break",
			src:  "foo  \nbaz\n",
			want: "<p>foo<br />\nbaz</p>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.HTML(tt.src); got != tt.want {
				t.Fatalf("HTML(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
