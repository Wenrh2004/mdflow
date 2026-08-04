// Package typography implements the Memos-flavoured wrappers `==highlight==`,
// `~subscript~`, `^superscript^` and `||spoiler||` as an mdflow extension.
//
// Each wrapper parses its content recursively, so `==a **b**==` nests properly.
// Subscript shares the `~` trigger with strikethrough and must be registered
// after it, so a parser with typography but not strikethrough reads `~~x~~` as
// an empty subscript. The gfm+memos bundling in the `all` module registers
// strikethrough first, which is the supported pairing.
package typography

import (
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// The four wrapper tags, each paired (open/close). Allocated by name.
var (
	highlightTag   = token.NewTag("highlight")
	subscriptTag   = token.NewTag("subscript")
	superscriptTag = token.NewTag("superscript")
	spoilerTag     = token.NewTag("spoiler")
)

// Typography is the wrapper capability: highlight, sub/superscript and spoiler.
var Typography = extension.Capability{
	Name:   "typography",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers the four wrapper rules on p, independent of any renderer.
// Subscript is registered first to match the original ordering; within the `~`
// trigger a strikethrough rule registered ahead of it still wins.
func Syntax(p *parser.RuleSet) {
	p.AddInlineRule(wrapRule{"subscript", "~", subscriptTag})
	p.AddInlineRule(wrapRule{"highlight", "==", highlightTag})
	p.AddInlineRule(wrapRule{"superscript", "^", superscriptTag})
	p.AddInlineRule(wrapRule{"spoiler", "||", spoilerTag})
}

// IsHighlight reports whether e enters or leaves a `==highlight==` run.
func IsHighlight(e mdflow.Event) bool { return e.Node == token.Custom && e.Tag == highlightTag }

// IsSubscript reports whether e enters or leaves a `~subscript~` run.
func IsSubscript(e mdflow.Event) bool { return e.Node == token.Custom && e.Tag == subscriptTag }

// IsSuperscript reports whether e enters or leaves a `^superscript^` run.
func IsSuperscript(e mdflow.Event) bool { return e.Node == token.Custom && e.Tag == superscriptTag }

// IsSpoiler reports whether e enters or leaves a `||spoiler||` run.
func IsSpoiler(e mdflow.Event) bool { return e.Node == token.Custom && e.Tag == spoilerTag }

// wrapRule matches `<open>content<close>` and emits a paired node whose content
// is parsed with the same rule set. One implementation serves all four wrappers;
// they differ only in delimiter and tag.
type wrapRule struct {
	name  string
	delim string
	tag   token.Tag
}

func (r wrapRule) Name() string     { return r.name }
func (r wrapRule) Triggers() []byte { return []byte{r.delim[0]} }
func (r wrapRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if !strings.HasPrefix(src[i:], r.delim) {
		return false
	}
	rest := src[i+len(r.delim):]
	end := strings.Index(rest, r.delim)
	if end <= 0 {
		return false
	}
	body := rest[:end]
	if strings.IndexByte(body, '\n') >= 0 {
		return false // these constructs never span lines
	}
	inner := s.Parse(body)
	s.Emit(token.Inline{Node: token.Custom, Tag: r.tag})
	s.EmitAll(inner)
	s.Emit(token.Inline{Node: token.Custom, Tag: r.tag, Close: true})
	s.Advance(len(r.delim)*2 + end)
	return true
}

func output(r renderer.Renderer) {
	extension.Paired(r, highlightTag, "<mark>", "</mark>")
	extension.Paired(r, subscriptTag, "<sub>", "</sub>")
	extension.Paired(r, superscriptTag, "<sup>", "</sup>")
	extension.Paired(r, spoilerTag, "<details><summary>", "</summary></details>")
}
