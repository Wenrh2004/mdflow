package rawhtml_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

func newParser() *mdflow.Parser {
	return mdflow.NewWith(parser.New(), html.NewRenderer(), rawhtml.RawHTML)
}

// By default a recognised tag is escaped, not emitted: passing user-authored
// HTML straight through would be an XSS vector.
func TestRawHTMLEscapedByDefault(t *testing.T) {
	cases := map[string]string{
		"a <b>c</b>\n":                "<p>a &lt;b&gt;c&lt;/b&gt;</p>\n",
		"<script>alert(1)</script>\n": "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>\n",
		"<br>\n":                      "<p>&lt;br&gt;</p>\n",
	}
	p := newParser()
	for in, want := range cases {
		if got := p.HTML(in); got != want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// WithUnsafeHTML passes recognised tags through verbatim. It layers after the
// escaping registration, so it needs RawHTML enabled to have anything to affect.
func TestWithUnsafeHTML(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(rawhtml.RawHTML), rawhtml.WithUnsafeHTML())
	if got, want := p.HTML("a <b>c</b>\n"), "<p>a <b>c</b></p>\n"; got != want {
		t.Errorf("HTML(%q)\n got: %q\nwant: %q", "a <b>c</b>\n", got, want)
	}
}

// The rule is registered after the core autolink rule, which shares the `<`
// trigger, so an autolink still parses as a link rather than a raw tag.
func TestAutolinkStillWins(t *testing.T) {
	got := newParser().HTML("<https://example.com>\n")
	if want := `<p><a href="https://example.com">https://example.com</a></p>` + "\n"; got != want {
		t.Errorf("autolink lost the `<` trigger: got %q want %q", got, want)
	}
}

func TestIsRawHTML(t *testing.T) {
	p := newParser()
	n := 0
	for e := range p.Events("a <b>c</b>\n") {
		if rawhtml.IsRawHTML(e) {
			if !e.IsAtomic() {
				t.Errorf("raw-HTML event should be atomic: %+v", e)
			}
			n++
		}
	}
	if n != 2 { // <b> and </b>, each its own atomic token
		t.Errorf("IsRawHTML matched %d events, want 2", n)
	}
}
