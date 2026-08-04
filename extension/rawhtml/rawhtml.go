// Package rawhtml recognises inline HTML tags as an mdflow extension.
//
// Recognising is not emitting: the default output *escapes* every tag, because a
// parser that passes user-authored `<script>` straight through is an XSS vector.
// The tags surface as raw-HTML nodes regardless, so a caller can sanitise them
// itself, and [WithUnsafeHTML] replaces the escaping renderer with a verbatim
// one for content you control.
//
// The rule is registered after the core autolink rule, which shares the `<`
// trigger, so `<https://x>` still parses as an autolink.
package rawhtml

import (
	"html"
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// rawHTMLTag is atomic: each tag is its own token, so open/close pairs fall out
// of the flat stream with no nesting bookkeeping.
var rawHTMLTag = token.NewAtomicTag("raw_html")

// RawHTML is the inline raw-HTML capability. Its output half escapes.
var RawHTML = extension.Capability{
	Name:   "raw_html",
	Syntax: Syntax,
	Output: func(r renderer.Renderer) {
		renderer.RegisterCustom(r, rawHTMLTag, func(w renderer.Writer, t token.Inline) {
			w.WriteString(html.EscapeString(t.Text))
		})
	},
}

// Syntax registers the raw-HTML inline rule on p, independent of any renderer.
func Syntax(p *parser.RuleSet) { p.AddInlineRule(rawHTMLRule{}) }

// IsRawHTML reports whether e is a raw HTML tag.
func IsRawHTML(e mdflow.Event) bool {
	return e.Node == token.Custom && e.Tag == rawHTMLTag
}

// WithUnsafeHTML passes recognised raw HTML tags through to the output verbatim
// instead of escaping them.
//
// It is a renderer tweak layered after [RawHTML]'s escaping registration, so it
// only takes effect when raw HTML is also enabled. Off by default: enable it
// only for content you control.
func WithUnsafeHTML() mdflow.Option {
	return mdflow.WithExtensions(extension.Capability{
		Name: "raw_html_unsafe",
		Output: func(r renderer.Renderer) {
			renderer.RegisterCustom(r, rawHTMLTag,
				func(w renderer.Writer, t token.Inline) { w.WriteString(t.Text) })
		},
	})
}

// rawHTMLRule recognises one inline HTML tag: `<u>`, `</u>`, `<br>`,
// `<img src="...">`. Each tag is its own token.
type rawHTMLRule struct{}

func (rawHTMLRule) Name() string     { return "raw_html" }
func (rawHTMLRule) Triggers() []byte { return []byte{'<'} }
func (rawHTMLRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	j := i + 1
	if j < len(src) && src[j] == '/' {
		j++
	}
	start := j
	for j < len(src) && isTagNameByte(src[j]) {
		j++
	}
	if j == start {
		return false
	}
	end := strings.IndexByte(src[j:], '>')
	if end < 0 {
		return false
	}
	end += j
	if strings.IndexByte(src[i:end], '\n') >= 0 {
		return false
	}
	s.Emit(token.Inline{Node: token.Custom, Tag: rawHTMLTag, Text: src[i : end+1]})
	s.Advance(end + 1 - i)
	return true
}

func isTagNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}
