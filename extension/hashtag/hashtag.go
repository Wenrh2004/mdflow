// Package hashtag implements `#tag` as an mdflow extension.
//
// A `#` followed by a space is an ATX heading and never reaches the inline
// layer, so the rule only ever sees a genuine hashtag; it stops at whitespace,
// another `#` or a backslash, matching github.com/usememos/gomark.
package hashtag

import (
	"html"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// hashtagTag is atomic: a hashtag is one self-contained token, not an open/close
// pair.
var hashtagTag = token.NewAtomicTag("github.com/Wenrh2004/mdflow/extension/hashtag.hashtag")

// Hashtag is the `#tag` capability.
var Hashtag = extension.Capability{
	Name:   "hashtag",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers the hashtag inline rule on p, independent of any renderer.
func Syntax(p *parser.RuleSet) { p.AddInlineRule(tagRule{}) }

// IsHashtag reports whether e is a `#hashtag`.
func IsHashtag(e mdflow.Event) bool {
	return e.Node == token.Custom && e.Tag == hashtagTag
}

// tagRule handles `#tag`, stopping at whitespace, another `#` or a backslash.
type tagRule struct{}

func (tagRule) Name() string     { return "hashtag" }
func (tagRule) Triggers() []byte { return []byte{'#'} }
func (tagRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	j := i + 1
	for j < len(src) {
		c := src[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '#' || c == '\\' {
			break
		}
		j++
	}
	if j == i+1 {
		return false
	}
	s.Emit(token.Inline{Node: token.Custom, Tag: hashtagTag, Text: src[i+1 : j]})
	s.Advance(j - i)
	return true
}

func output(r renderer.Renderer) {
	renderer.RegisterCustom(r, hashtagTag, func(w renderer.Writer, t token.Inline) {
		w.WriteString(`<span class="tag">#`)
		w.WriteString(html.EscapeString(t.Text))
		w.WriteString("</span>")
	})
}
