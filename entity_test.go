package mdflow_test

import (
	"slices"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/token"
)

func TestCommonMarkInlineCharacterReferences(t *testing.T) {
	cases := []struct {
		name     string
		markdown string
		html     string
	}{
		{
			name: "named references",
			markdown: "&nbsp; &amp; &copy; &AElig; &Dcaron;\n" +
				"&frac34; &HilbertSpace; &DifferentialD;\n" +
				"&ClockwiseContourIntegral; &ngE;\n",
			html: "<p>\u00a0 &amp; © Æ Ď\n" +
				"¾ ℋ ⅆ\n" +
				"∲ ≧̸</p>\n",
		},
		{
			name:     "decimal references",
			markdown: "&#35; &#1234; &#992; &#0;\n",
			html:     "<p># Ӓ Ϡ �</p>\n",
		},
		{
			name:     "hexadecimal references",
			markdown: "&#X22; &#XD06; &#xcab;\n",
			html:     "<p>&quot; ആ ಫ</p>\n",
		},
		{
			name: "malformed references stay literal",
			markdown: "&nbsp &x; &#; &#x;\n" +
				"&#87654321;\n" +
				"&#abcdef0;\n" +
				"&ThisIsNotDefined; &hi?;\n",
			html: "<p>&amp;nbsp &amp;x; &amp;#; &amp;#x;\n" +
				"&amp;#87654321;\n" +
				"&amp;#abcdef0;\n" +
				"&amp;ThisIsNotDefined; &amp;hi?;</p>\n",
		},
		{
			name:     "named reference requires semicolon at EOF",
			markdown: "&copy",
			html:     "<p>&amp;copy</p>\n",
		},
	}

	p := mdflow.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.HTML(tc.markdown); got != tc.html {
				t.Fatalf("HTML(%q) = %q, want %q", tc.markdown, got, tc.html)
			}
		})
	}
}

func TestInlineCharacterReferenceBoundaries(t *testing.T) {
	const src = "a &amp; b &#9; c &#10; d &#x10ffff; e &#0; f &#xD800; g &#x110000; " +
		"h &#12345678; i &#x1234567; j &amp k\n"
	wantText := []string{
		"a & b \t c \n d \U0010ffff e � f � g � h " +
			"&#12345678; i &#x1234567; j &amp k",
	}

	gotText := slices.Collect(iterx.FilterMap(mdflow.New().Events(src), func(e mdflow.Event) (string, bool) {
		return e.Text, e.Type == mdflow.TextEvent
	}))
	if !slices.Equal(gotText, wantText) {
		t.Fatalf("text events = %q, want %q", gotText, wantText)
	}
}

func TestDecodedCharacterReferencesAreNotReparsedAsMarkup(t *testing.T) {
	const src = "&#42;foo&#42;\n"
	if got, want := mdflow.New().HTML(src), "<p>*foo*</p>\n"; got != want {
		t.Fatalf("HTML(%q) = %q, want %q", src, got, want)
	}
}

func TestCommonMarkExample32DecodesLinkDestinationAndTitle(t *testing.T) {
	const src = "[foo](/f&ouml;&ouml; \"f&ouml;&ouml;\")\n"
	links := slices.Collect(iterx.Filter(mdflow.New().Events(src), func(e mdflow.Event) bool {
		return e.Type == mdflow.EnterEvent && e.Node == token.Link
	}))
	if len(links) != 1 {
		t.Fatalf("link events = %#v, want one link", links)
	}
	if got, want := links[0].Dest, "/föö"; got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}
	if got, want := links[0].Title, "föö"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestCommonMarkExample34DecodesFencedCodeInfo(t *testing.T) {
	const src = "``` f&ouml;&ouml;\nfoo\n```\n"
	const want = "<pre><code class=\"language-föö\">foo\n</code></pre>\n"
	if got := mdflow.New().HTML(src); got != want {
		t.Fatalf("HTML(%q) = %q, want %q", src, got, want)
	}
}
