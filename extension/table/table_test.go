package table_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/table"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), table.Table)
}

func TestTableRenders(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"aligned columns",
			"| a | b |\n| --- | ---: |\n| 1 | 2 |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n<th align=\"right\">b</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td>1</td>\n<td align=\"right\">2</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"centre alignment",
			"| a |\n| :-: |\n| 1 |\n",
			"<table>\n<thead>\n<tr>\n<th align=\"center\">a</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td align=\"center\">1</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"inline markup in a cell parses",
			"| a |\n| - |\n| *x* |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td><em>x</em></td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"header only, no body",
			"| a | b |\n| - | - |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n</table>\n",
		},
		{
			"short row is padded",
			"| a | b |\n| - | - |\n| 1 |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td>1</td>\n<td></td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"no delimiter row stays a paragraph",
			"| a | b |\nnot a table\n",
			"<p>| a | b |\nnot a table</p>\n",
		},
	}
	p := newParser()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.HTML(tc.in); got != tc.want {
				t.Errorf("HTML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// A table ends at the first line without a pipe; what follows is an ordinary
// block, proving the continuation closes the leaf cleanly.
func TestTableEndsAtNonRow(t *testing.T) {
	got := newParser().HTML("| a |\n| - |\n| 1 |\nafter\n")
	if !strings.Contains(got, "</table>\n<p>after</p>\n") {
		t.Errorf("table did not close before the following paragraph: %q", got)
	}
}

// Syntax without Output surfaces the structure but registers no markup: cells
// still emit their text, never nothing.
func TestSyntaxOnlyKeepsText(t *testing.T) {
	rules := parser.New()
	table.Syntax(rules)
	got := mdflow.NewWith(rules, html.NewRenderer()).HTML("| a |\n| - |\n| 1 |\n")
	if want := "a1"; got != want {
		t.Errorf("syntax-only table: got %q want %q", got, want)
	}
}

func TestIsTable(t *testing.T) {
	p := newParser()
	n := 0
	for e := range p.Events("| a |\n| - |\n| 1 |\n") {
		if table.IsTable(e) {
			n++
		}
	}
	// table+thead+row (open/close ×3) = 6, its one cell (open/close) = 2,
	// tbody+row (open/close ×2) = 4, its one cell (open/close) = 2 → 14 events
	// touch the table.
	if n != 14 {
		t.Errorf("IsTable matched %d events, want 14", n)
	}
}

// Streaming a table byte by byte must match a single-shot parse: the leaf
// accumulates across chunk boundaries.
func TestTableSurvivesStreaming(t *testing.T) {
	src := "| a | b |\n| - | -: |\n| 1 | 2 |\n| 3 | 4 |\n"
	p := newParser()
	want := p.HTML(src)
	for _, chunk := range []int{1, 3, 7, 64} {
		var got strings.Builder
		s := p.Stream()
		for i := 0; i < len(src); i += chunk {
			got.WriteString(s.Feed(src[i:min(i+chunk, len(src))]))
		}
		got.WriteString(s.Finish())
		if got.String() != want {
			t.Errorf("chunk=%d\n got: %q\nwant: %q", chunk, got.String(), want)
		}
	}
}

// A wide header over many one-cell rows is padded to the header's width. That
// padding is bounded per table, as in cmark-gfm, so output stays linear.
func TestAutocompletedCellsAreBounded(t *testing.T) {
	const n = 3_000
	src := strings.Repeat("|a", n) + "|\n" + strings.Repeat("|-", n) + "|\n" + strings.Repeat("|x\n", n)
	out := newParser().HTML(src)
	if limit := 20*len(src) + 12*0x80000; len(out) > limit {
		t.Fatalf("output %d bytes from %d bytes of input; want <= %d", len(out), len(src), limit)
	}
	if !strings.Contains(out, "</table>\n<p>|x\n|x") {
		t.Fatal("rows past the padding budget must end the table and stay paragraph text")
	}
}

// A reference definition leading the paragraph that becomes a table header is
// registered with the document rather than swallowed into the table, and the
// line below it is still recognised as the header (cmark-gfm and goldmark agree).
func TestPromotedHeaderRegistersLeadingDefinitions(t *testing.T) {
	src := "[a]: /u\n| x | y |\n|---|---|\n| 1 | 2 |\n\n[a]\n"
	want := "<table>\n<thead>\n<tr>\n<th>x</th>\n<th>y</th>\n</tr>\n</thead>\n" +
		"<tbody>\n<tr>\n<td>1</td>\n<td>2</td>\n</tr>\n</tbody>\n</table>\n" +
		"<p><a href=\"/u\">a</a></p>\n"
	if got := newParser().HTML(src); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	// Streaming must agree at every split point.
	for i := 0; i <= len(src); i++ {
		s := newParser().Stream()
		if got := s.Feed(src[:i]) + s.Feed(src[i:]) + s.Finish(); got != want {
			t.Fatalf("split at %d: got %q", i, got)
		}
	}
}
