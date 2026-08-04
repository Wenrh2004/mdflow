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
		got.WriteString(s.Close())
		if got.String() != want {
			t.Errorf("chunk=%d\n got: %q\nwant: %q", chunk, got.String(), want)
		}
	}
}
