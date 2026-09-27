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
	referenceTag = token.NewAtomicTag("github.com/Wenrh2004/mdflow/extension/resource.reference")
	embedTag     = token.NewTag("github.com/Wenrh2004/mdflow/extension/resource.embed")
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

// referenceScanTag keys the per-parse memo below. It is a private tag used only
// as a memo slot key, never emitted or rendered.
var referenceScanTag = token.NewTag("github.com/Wenrh2004/mdflow/extension/resource.resource.reference.scan")

// referenceScanMemo caches the offset from which the remaining source is known
// to contain no closing "]]". A run of unmatched "[[" would otherwise rescan the
// tail from every position, which is O(n^2); once one probe proves the tail has
// no close, every later probe inside that tail is a constant-time reject. It is
// per-parse scratch obtained through [parser.InlineState.Memo]; CloneInlineMemo
// keeps Stream.Provisional snapshots isolated from the live parse.
type referenceScanMemo struct {
	noCloseFrom int // smallest offset proven to hold no "]]"; -1 when unknown
}

func (m *referenceScanMemo) CloneInlineMemo() parser.InlineMemo {
	clone := *m
	return &clone
}

// referenceRule handles `[[resource]]` and `[[resource?params]]`.
type referenceRule struct{}

func (referenceRule) Name() string     { return "reference" }
func (referenceRule) Triggers() []byte { return []byte{'['} }
func (referenceRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if !strings.HasPrefix(src[i:], "[[") {
		return false
	}
	start := i + 2
	memo := s.Memo(referenceScanTag, func() parser.InlineMemo {
		return &referenceScanMemo{noCloseFrom: -1}
	}).(*referenceScanMemo)
	// Src is immutable and the scan offset only advances, so once some suffix is
	// known to have no "]]", every later "[[" inside it cannot either.
	if memo.noCloseFrom >= 0 && start >= memo.noCloseFrom {
		s.AddWork(1)
		return false
	}
	rest := src[start:]
	end := strings.Index(rest, "]]")
	if end < 0 {
		s.AddWork(len(rest)) // report the scanned span to the work seam
		memo.noCloseFrom = start
		return false
	}
	s.AddWork(end + 2)
	if end == 0 {
		return false
	}
	name, params := splitResource(src[start : start+end])
	s.Emit(token.Inline{Node: token.Custom, Tag: referenceTag, Text: name, Dest: params})
	s.Advance(end + 4)
	return true
}

// ---- block: ![[embed]] ----

// embedRule handles a line that is exactly `![[resource]]`.
type embedRule struct{}

func (embedRule) Name() string { return "embed" }
func (embedRule) InterruptsParagraph(line string) bool {
	_, _, ok := parseEmbed(line)
	return ok
}
func (embedRule) Open(s *parser.BlockState, line string) bool {
	name, params, ok := parseEmbed(line)
	if !ok {
		return false
	}
	s.EmitLeaf(token.Leaf{Node: token.CustomLeaf, Tag: embedTag, Content: name, Info: params, Literal: true})
	return true
}

func parseEmbed(line string) (name, params string, ok bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "![[") || !strings.HasSuffix(t, "]]") || len(t) < 5 {
		return "", "", false
	}
	name, params = splitResource(t[3 : len(t)-2])
	return name, params, true
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
