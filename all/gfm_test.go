package all_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
)

// The GFM constructs — tables, strikethrough and task lists — exercised through
// the full-syntax bundle. They live outside the CommonMark profile in their own
// extension modules.
func TestGFMSyntax(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strikethrough", "~~gone~~\n", "<p><del>gone</del></p>\n"},
		{
			"table",
			"| a | b |\n| --- | ---: |\n| 1 | 2 |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n<th align=\"right\">b</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td>1</td>\n<td align=\"right\">2</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"table with inline markup",
			"| a |\n| - |\n| *x* |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td><em>x</em></td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			"task list",
			"- [ ] todo\n- [x] done\n",
			"<ul>\n<li><input type=\"checkbox\" disabled /> todo</li>\n" +
				"<li><input type=\"checkbox\" checked disabled /> done</li>\n</ul>\n",
		},
		{
			"pipe without delimiter row stays a paragraph",
			"| a | b |\nnot a table\n",
			"<p>| a | b |\nnot a table</p>\n",
		},
	}

	p := all.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.HTML(tc.in); got != tc.want {
				t.Errorf("HTML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// The whole point of the streaming design is that chunk boundaries are
// invisible. A table is the hardest case — it accumulates lines across chunks —
// so feeding it byte by byte must still match a single-shot parse.
func TestStreamMatchesBatchWithTable(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"Some *emphasis* and `code` and a [link](https://go.dev).",
		"",
		"| a | b |",
		"| - | - |",
		"| 1 | 2 |",
		"",
		"final paragraph",
	}, "\n") + "\n"

	p := all.New()
	want := p.HTML(src)

	for _, chunk := range []int{1, 2, 3, 7, 16, 64, 4096} {
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

func TestStreamProvisionalTable(t *testing.T) {
	// A header + delimiter row with no body yet: the table is a multi-event
	// accumulating leaf, so the provisional view must finalise a snapshot to
	// show it, without disturbing the real parse.
	p := all.New()
	s := p.Stream()
	s.Feed("| a | b |\n| - | - |\n")
	prov := s.Provisional()
	for _, want := range []string{"<table>", "<thead>", "<th>a</th>", "<th>b</th>"} {
		if !strings.Contains(prov, want) {
			t.Errorf("provisional table missing %q: %q", want, prov)
		}
	}
	// The probe must not disturb the real state: a body row still lands.
	final := s.Feed("| 1 | 2 |\n") + s.Close()
	if !strings.Contains(final, "<td>1</td>") || !strings.Contains(final, "<td>2</td>") {
		t.Errorf("real parse lost the body row after Provisional: %q", final)
	}
}

// A pipeline that leaves every event alone must be byte-identical to the fast
// path — including the multi-event table stream, otherwise the two rendering
// routes have drifted apart.
func TestIdentityPipelineMatchesFastPathWithTable(t *testing.T) {
	src := "# T\n\npara *x* `c` [l](/u) ![i](/p)\n\n- a\n- b\n\n> q\n\n```go\nz\n```\n\n| a | b |\n| - | - |\n| 1 | 2 |\n"
	base := all.New()
	piped := base.Map(func(e mdflow.Event) mdflow.Event { return e })
	if got, want := piped.HTML(src), base.HTML(src); got != want {
		t.Errorf("pipeline route differs from fast path:\n got: %q\nwant: %q", got, want)
	}
}
