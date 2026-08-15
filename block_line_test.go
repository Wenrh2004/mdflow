package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	mdhtml "github.com/Wenrh2004/mdflow/renderer/html"
)

type stripSectionMarkerRule struct{}

func (stripSectionMarkerRule) Name() string { return "strip_section_marker" }

func (stripSectionMarkerRule) Open(_ *parser.BlockState, line string) (string, bool) {
	if strings.HasPrefix(line, "§") {
		return strings.TrimPrefix(line, "§"), true
	}
	return line, false
}

func TestCommonMarkTabsInBlockMarkers(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "official example 10 atx heading",
			src:  "#\tFoo\n",
			want: "<h1>Foo</h1>\n",
		},
		{
			name: "official example 11 thematic break",
			src:  "*\t*\t*\t\n",
			want: "<hr />\n",
		},
		{
			name: "blockquote marker",
			src:  ">\tfoo\n",
			want: "<blockquote>\n<p>foo</p>\n</blockquote>\n",
		},
		{
			name: "list marker",
			src:  "-\tfoo\n",
			want: "<ul>\n<li>foo</li>\n</ul>\n",
		},
		{
			name: "closing fence trailing tab",
			src:  "```\nx\n```\t\n",
			want: "<pre><code>x\n</code></pre>\n",
		},
		{
			name: "content tab is not expanded",
			src:  "foo\tbar\n",
			want: "<p>foo\tbar</p>\n",
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

func TestCommonMarkLineEndingsAreEquivalent(t *testing.T) {
	p := mdflow.New()
	lf := "Foo\n---\n\n> bar\n\n- baz\n"
	want := p.HTML(lf)

	for name, src := range map[string]string{
		"LF":    lf,
		"CRLF":  strings.ReplaceAll(lf, "\n", "\r\n"),
		"CR":    strings.ReplaceAll(lf, "\n", "\r"),
		"mixed": "Foo\r\n---\r\n\r> bar\n\r- baz\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := p.HTML(src); got != want {
				t.Fatalf("HTML with %s endings = %q, want %q", name, got, want)
			}
		})
	}
}

func TestStreamLineEndingsAcrossChunks(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "CRLF split after CR", chunks: []string{"Foo\r", "\n---\r", "\n"}},
		{name: "CRLF byte split", chunks: []string{"Foo", "\r", "\n", "---", "\r", "\n"}},
		{name: "CR only", chunks: []string{"Foo\r", "---\r"}},
		{name: "mixed fenced code", chunks: []string{"```\r", "\na\r", "\nb\n", "```\r", "\n"}},
		{name: "empty chunk preserves pending CRLF", chunks: []string{"Foo\r", "", "\n---\r", "\n"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := p.Stream()
			var src, committed strings.Builder
			for i, chunk := range tt.chunks {
				src.WriteString(chunk)
				committed.WriteString(s.Feed(chunk))
				if got, want := committed.String()+s.Provisional(), p.HTML(src.String()); got != want {
					t.Fatalf("after chunk %d %q: committed + provisional = %q, want batch prefix %q", i, chunk, got, want)
				}
			}
			committed.WriteString(s.Close())
			if got, want := committed.String(), p.HTML(src.String()); got != want {
				t.Fatalf("closed stream = %q, want batch %q", got, want)
			}
		})
	}
}

func TestStreamLineEndingBoundaries(t *testing.T) {
	p := mdflow.New()
	for name, chunks := range map[string][]string{
		"one CRLF":              {"\r", "\n"},
		"CR then CRLF":          {"\r", "\r", "\n"},
		"trailing CR":           {"a", "\r"},
		"unterminated final":    {"a"},
		"CRLF after empty feed": {"a\r", "", "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			s := p.Stream()
			var src, got strings.Builder
			for _, chunk := range chunks {
				src.WriteString(chunk)
				got.WriteString(s.Feed(chunk))
			}
			got.WriteString(s.Close())
			if want := p.HTML(src.String()); got.String() != want {
				t.Fatalf("stream = %q, want batch %q", got.String(), want)
			}
		})
	}
}

func TestStreamProvisionalDoesNotRepeatCommittedContainers(t *testing.T) {
	p := mdflow.New()
	for name, src := range map[string]string{
		"blockquote": "> foo\n",
		"list":       "- foo\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := p.Stream()
			committed := s.Feed(src)
			if got, want := committed+s.Provisional(), p.HTML(src); got != want {
				t.Fatalf("committed + provisional = %q, want %q", got, want)
			}
		})
	}
}

func TestExtensionRemainderCountsUTF8ByRune(t *testing.T) {
	rules := parser.New()
	rules.AddContainerRule(stripSectionMarkerRule{})
	p := mdflow.NewWith(rules, mdhtml.NewRenderer())

	const src = "§\t # h\n"
	// The marker is one visual column, so the tab contributes three and the
	// following space one: the remainder is an indented code block. Counting the
	// UTF-8 bytes as columns would instead leave only three columns and parse an
	// ATX heading.
	const want = "<pre><code># h\n</code></pre>\n"
	if got := p.HTML(src); got != want {
		t.Fatalf("HTML(%q) = %q, want %q", src, got, want)
	}
}
