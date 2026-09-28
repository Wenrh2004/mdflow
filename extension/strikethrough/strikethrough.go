// Package strikethrough implements GFM `~~text~~` as an mdflow extension.
//
// It is deliberately the smallest possible capability — one inline rule and one
// paired render target — and so serves as the reference for writing an inline
// extension entirely against the public seam: an [parser.InlineRule] whose Match
// is written in terms of Src/Pos/Parse/Emit/Advance, with no access to
// unexported parser state.
package strikethrough

import (
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// strikeTag is allocated by name, so the rule and the renderer agree without a
// shared constant. It is paired (open/close), which is the default for NewTag.
var strikeTag = token.NewTag("github.com/Wenrh2004/mdflow/extension/strikethrough.strikethrough")

// Strikethrough is the GFM strikethrough capability.
var Strikethrough = extension.Capability{
	Name:   "strikethrough",
	Syntax: Syntax,
	Output: func(r renderer.Renderer) { extension.Paired(r, strikeTag, "<del>", "</del>") },
}

// Syntax registers the strikethrough inline rule on p, independent of any
// renderer.
func Syntax(p *parser.RuleSet) { p.AddInlineRule(strikeRule{}) }

// IsStrikethrough reports whether e enters or leaves a `~~strike~~` run.
func IsStrikethrough(e mdflow.Event) bool {
	return e.Node == token.Custom && e.Tag == strikeTag
}

// strikeRule handles `~~text~~`, parsing its content recursively so emphasis
// and other inline syntax inside a strike run still applies.
type strikeRule struct{}

func (strikeRule) Name() string     { return "strikethrough" }
func (strikeRule) Triggers() []byte { return []byte{'~'} }
func (strikeRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if !strings.HasPrefix(src[i:], "~~") {
		return false
	}
	closeIdx := strings.Index(src[i+2:], "~~")
	if closeIdx < 0 {
		return false
	}
	inner := s.Parse(src[i+2 : i+2+closeIdx])
	s.Emit(token.Inline{Node: token.Custom, Tag: strikeTag})
	s.EmitAll(inner)
	s.Emit(token.Inline{Node: token.Custom, Tag: strikeTag, Close: true})
	s.Advance(2 + closeIdx + 2)
	return true
}
