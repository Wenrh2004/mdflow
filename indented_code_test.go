package mdflow_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestCommonMarkIndentedCodeBlocks(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "official example 107 basic block",
			src:  "    a simple\n      indented code block\n",
			want: "<pre><code>a simple\n  indented code block\n</code></pre>\n",
		},
		{
			name: "official example 110 literal content",
			src:  "    <a/>\n    *hi*\n\n    - one\n",
			want: "<pre><code>&lt;a/&gt;\n*hi*\n\n- one\n</code></pre>\n",
		},
		{
			name: "official example 111 blank separators",
			src:  "    chunk1\n\n    chunk2\n  \n \n\n    chunk3\n",
			want: "<pre><code>chunk1\n\nchunk2\n\n\n\nchunk3\n</code></pre>\n",
		},
		{
			name: "official example 112 residual blank indentation",
			src:  "    chunk1\n      \n      chunk2\n",
			want: "<pre><code>chunk1\n  \n  chunk2\n</code></pre>\n",
		},
		{
			name: "official example 113 cannot interrupt paragraph",
			src:  "Foo\n    bar\n\n",
			want: "<p>Foo\nbar</p>\n",
		},
		{
			name: "official example 114 exit reclassifies line",
			src:  "    foo\nbar\n",
			want: "<pre><code>foo\n</code></pre>\n<p>bar</p>\n",
		},
		{
			name: "official example 115 after headings",
			src:  "# Heading\n    foo\nHeading\n------\n    foo\n----\n",
			want: "<h1>Heading</h1>\n<pre><code>foo\n</code></pre>\n<h2>Heading</h2>\n<pre><code>foo\n</code></pre>\n<hr />\n",
		},
		{
			name: "official example 116 removes exactly four columns",
			src:  "        foo\n    bar\n",
			want: "<pre><code>    foo\nbar\n</code></pre>\n",
		},
		{
			name: "official example 117 trims surrounding blank lines",
			src:  "\n    \n    foo\n    \n",
			want: "<pre><code>foo\n</code></pre>\n",
		},
		{
			name: "official example 118 preserves trailing spaces",
			src:  "    foo  \n",
			want: "<pre><code>foo  \n</code></pre>\n",
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

func TestCommonMarkTabsInIndentedCode(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "official tabs example 1",
			src:  "\tfoo\tbaz\t\tbim\n",
			want: "<pre><code>foo\tbaz\t\tbim\n</code></pre>\n",
		},
		{
			name: "official tabs example 2",
			src:  "  \tfoo\tbaz\t\tbim\n",
			want: "<pre><code>foo\tbaz\t\tbim\n</code></pre>\n",
		},
		{
			name: "official tabs example 3",
			src:  "    a\ta\n    ὐ\ta\n",
			want: "<pre><code>a\ta\nὐ\ta\n</code></pre>\n",
		},
		{
			name: "official tabs example 8",
			src:  "    foo\n\tbar\n",
			want: "<pre><code>foo\nbar\n</code></pre>\n",
		},
		{
			name: "official tabs example 6 in blockquote",
			src:  ">\t\tfoo\n",
			want: "<blockquote>\n<pre><code>  foo\n</code></pre>\n</blockquote>\n",
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
