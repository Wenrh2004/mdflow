package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/token"
)

func TestCommonMarkURINormalization(t *testing.T) {
	cases := []struct {
		name     string
		markdown string
		html     string
	}{
		{
			name:     "example 20 autolink backslash is literal",
			markdown: "<https://example.com?find=\\*>\n",
			html:     "<p><a href=\"https://example.com?find=%5C*\">https://example.com?find=\\*</a></p>\n",
		},
		{
			name:     "example 22 link destination and title escapes",
			markdown: "[foo](/bar\\* \"ti\\*tle\")\n",
			html:     "<p><a href=\"/bar*\" title=\"ti*tle\">foo</a></p>\n",
		},
		{
			name:     "example 24 fenced info escape",
			markdown: "``` foo\\+bar\nfoo\n```\n",
			html:     "<pre><code class=\"language-foo+bar\">foo\n</code></pre>\n",
		},
		{
			name:     "example 32 entities in destination and title",
			markdown: "[foo](/f&ouml;&ouml; \"f&ouml;&ouml;\")\n",
			html:     "<p><a href=\"/f%C3%B6%C3%B6\" title=\"föö\">foo</a></p>\n",
		},
		{
			name:     "example 489 space in pointy destination",
			markdown: "[link](</my uri>)\n",
			html:     "<p><a href=\"/my%20uri\">link</a></p>\n",
		},
		{
			name:     "example 495 escaped parentheses",
			markdown: "[link](\\(foo\\))\n",
			html:     "<p><a href=\"(foo)\">link</a></p>\n",
		},
		{
			name:     "example 500 escaped close and colon",
			markdown: "[link](foo\\)\\:)\n",
			html:     "<p><a href=\"foo):\">link</a></p>\n",
		},
		{
			name:     "example 502 non-escape backslash",
			markdown: "[link](foo\\bar)\n",
			html:     "<p><a href=\"foo%5Cbar\">link</a></p>\n",
		},
		{
			name:     "example 503 existing escape and decoded UTF-8",
			markdown: "[link](foo%20b&auml;)\n",
			html:     "<p><a href=\"foo%20b%C3%A4\">link</a></p>\n",
		},
		{
			name:     "example 506 title source escapes but is not URL encoded",
			markdown: "[link](/url \"title \\\"&quot;\")\n",
			html:     "<p><a href=\"/url\" title=\"title &quot;&quot;\">link</a></p>\n",
		},
		{
			name:     "example 595 autolink ampersands",
			markdown: "<https://foo.bar.baz/test?q=hello&id=22&boolean>\n",
			html:     "<p><a href=\"https://foo.bar.baz/test?q=hello&amp;id=22&amp;boolean\">https://foo.bar.baz/test?q=hello&amp;id=22&amp;boolean</a></p>\n",
		},
		{
			name:     "example 603 autolink does not source-unescape",
			markdown: "<https://example.com/\\[\\>\n",
			html:     "<p><a href=\"https://example.com/%5C%5B%5C\">https://example.com/\\[\\</a></p>\n",
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

func TestLinkSourceUnescapeIsSinglePassAndEventsStayDecoded(t *testing.T) {
	cases := []struct {
		name      string
		markdown  string
		wantDest  string
		wantTitle string
		wantHTML  string
	}{
		{
			name:      "backslash escape cannot expose an entity",
			markdown:  "[x](\\&ouml; \"&bsol;*\")\n",
			wantDest:  "&ouml;",
			wantTitle: "\\*",
			wantHTML:  "<p><a href=\"&amp;ouml;\" title=\"\\*\">x</a></p>\n",
		},
		{
			name:      "decoded unicode remains in event destination",
			markdown:  "[x](foo\\*%20b&auml; \"t\\*&quot;\")\n",
			wantDest:  "foo*%20bä",
			wantTitle: "t*\"",
			wantHTML:  "<p><a href=\"foo*%20b%C3%A4\" title=\"t*&quot;\">x</a></p>\n",
		},
	}

	p := mdflow.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links := iterx.Collect(iterx.Filter(p.Events(tc.markdown), func(e mdflow.Event) bool {
				return e.Type == mdflow.EnterEvent && e.Node == token.Link
			}))
			if len(links) != 1 {
				t.Fatalf("link events = %#v, want one link", links)
			}
			if got := links[0].Dest; got != tc.wantDest {
				t.Errorf("destination = %q, want %q", got, tc.wantDest)
			}
			if got := links[0].Title; got != tc.wantTitle {
				t.Errorf("title = %q, want %q", got, tc.wantTitle)
			}
			if got := p.HTML(tc.markdown); got != tc.wantHTML {
				t.Errorf("HTML(%q) = %q, want %q", tc.markdown, got, tc.wantHTML)
			}
		})
	}
}

func TestSourceNormalizationReplacesNULAndInvalidUTF8(t *testing.T) {
	p := mdflow.New()

	const withNUL = "[x](<a\x00b> \"a\x00b\")\n"
	const wantNUL = "<p><a href=\"a%EF%BF%BDb\" title=\"a�b\">x</a></p>\n"
	if got := p.HTML(withNUL); got != wantNUL {
		t.Fatalf("HTML with NUL = %q, want %q", got, wantNUL)
	}

	invalid := []byte{'a', 0xff, 0xfe, 'b', '\n'}
	const wantInvalid = "<p>a��b</p>\n"
	if got := p.HTMLBytes(invalid); got != wantInvalid {
		t.Fatalf("HTMLBytes with invalid UTF-8 = %q, want %q", got, wantInvalid)
	}
}

func TestStreamNormalizesAfterRejoiningSplitUTF8(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()
	src := "[x](<café>)\n"
	split := strings.Index(src, "é") + 1

	var got strings.Builder
	got.WriteString(s.Feed(src[:split]))
	got.WriteString(s.Feed(src[split:]))
	got.WriteString(s.Close())

	const want = "<p><a href=\"caf%C3%A9\">x</a></p>\n"
	if got.String() != want {
		t.Fatalf("split UTF-8 stream = %q, want %q", got.String(), want)
	}
}
