// Package resource implements Memos-flavoured wiki-style `[[reference]]` and
// block-level `![[embed]]` as an mdflow extension.
//
// A reference shares the `[` trigger with the core link rule and must be tried
// first, so [Syntax] prepends it. An embed is its own line: anything else on the
// line makes it an ordinary paragraph.
package resource

import (
	"html"
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// referenceTag is inline and atomic (it carries its own text); embedTag is a
// literal leaf block. Allocated by name.
var (
	referenceTag = token.NewAtomicTag("reference")
	embedTag     = token.NewTag("embed")
)

// Resource is the wiki-resource capability: `[[reference]]` and `![[embed]]`.
var Resource = extension.Capability{
	Name:   "resource",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers the resource rules on p, independent of any renderer.
func Syntax(p *parser.RuleSet) {
	// Must beat the core link rule, which also triggers on '['.
	p.PrependInlineRule(referenceRule{})
	p.AddLeafRule(embedRule{})
}

// IsResource reports whether e is a `[[reference]]` or a `![[embed]]`.
func IsResource(e mdflow.Event) bool {
	return (e.Node == token.Custom && e.Tag == referenceTag) ||
		(e.Node == token.CustomLeaf && e.Tag == embedTag)
}

// ---- inline: [[reference]] ----

// referenceRule handles `[[resource]]` and `[[resource?params]]`.
type referenceRule struct{}

func (referenceRule) Name() string     { return "reference" }
func (referenceRule) Triggers() []byte { return []byte{'['} }
func (referenceRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if !strings.HasPrefix(src[i:], "[[") {
		return false
	}
	end := strings.Index(src[i+2:], "]]")
	if end <= 0 {
		return false
	}
	name, params := splitResource(src[i+2 : i+2+end])
	s.Emit(token.Inline{Node: token.Custom, Tag: referenceTag, Text: name, Dest: params})
	s.Advance(end + 4)
	return true
}

// ---- block: ![[embed]] ----

// embedRule handles a line that is exactly `![[resource]]`.
type embedRule struct{}

func (embedRule) Name() string { return "embed" }
func (embedRule) Open(s *parser.BlockState, line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "![[") || !strings.HasSuffix(t, "]]") || len(t) < 5 {
		return false
	}
	name, params := splitResource(t[3 : len(t)-2])
	s.EmitLeaf(token.Leaf{Node: token.CustomLeaf, Tag: embedTag, Content: name, Info: params, Literal: true})
	return true
}

// splitResource splits `name?params` as gomark's resource syntax does.
func splitResource(s string) (name, params string) {
	if i := strings.IndexByte(s, '?'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// ---- output (HTML) ----

func output(r renderer.Renderer) {
	renderer.RegisterCustom(r, referenceTag, func(w renderer.Writer, t token.Inline) {
		w.WriteString(`<span class="reference" data-resource="`)
		w.WriteString(html.EscapeString(t.Text))
		if t.Dest != "" {
			w.WriteString(`" data-params="`)
			w.WriteString(html.EscapeString(t.Dest))
		}
		w.WriteString(`">`)
		w.WriteString(html.EscapeString(t.Text))
		w.WriteString("</span>")
	})
	renderer.RegisterCustomLeaf(r, embedTag,
		func(w renderer.Writer, leaf token.Leaf, _ []token.Inline, _ renderer.Renderer) {
			w.WriteString(`<div class="embed" data-resource="`)
			w.WriteString(html.EscapeString(leaf.Content))
			if leaf.Info != "" {
				w.WriteString(`" data-params="`)
				w.WriteString(html.EscapeString(leaf.Info))
			}
			w.WriteString(`">`)
			w.WriteString(html.EscapeString(leaf.Content))
			w.WriteString("</div>\n")
		})
}
