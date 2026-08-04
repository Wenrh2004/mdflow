// Package extension defines the seam an out-of-tree capability plugs into.
//
// A capability owns *both* halves of a feature: the rules that recognise the
// syntax, and the rendering that decides what it emits. Neither half has an
// independent existence — a rule with no rendering parses text into a node
// nothing knows how to write — so [Extension] requires both, and a [Capability]
// declares them side by side in one value.
//
// This package holds only the seam: the interface, the ordinary [Capability]
// that implements it, [Set] for grouping, and the two registration helpers a
// renderer-agnostic output half reaches for. It names no syntax and no tag. Each
// capability lives in its own module under this path — extension/table,
// extension/math, and so on — depends on the parser and renderer layers, and
// allocates its own [token.Tag] with [token.NewTag]. The core imports none of
// them, so a binary links exactly the capabilities it names and no more.
package extension

import (
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// Extension is one capability: syntax and the output it produces.
type Extension interface {
	// Rules registers the syntax with a rule set.
	Rules(p *parser.RuleSet)
	// Render registers the output with a renderer. A renderer that cannot host
	// the capability's nodes declines them, and the registration is skipped.
	Render(r renderer.Renderer)
}

// Capability is the ordinary way to build an [Extension]: name the feature and
// give its two halves.
//
// Either half may be nil — syntax the core renderer already knows how to write
// needs no Output — but a capability with neither does nothing at all.
type Capability struct {
	Name   string
	Syntax func(p *parser.RuleSet)
	Output func(r renderer.Renderer)
}

// Rules implements [Extension].
func (c Capability) Rules(p *parser.RuleSet) {
	if c.Syntax != nil {
		c.Syntax(p)
	}
}

// Render implements [Extension].
func (c Capability) Render(r renderer.Renderer) {
	if c.Output != nil {
		c.Output(r)
	}
}

// Set is an ordered group of capabilities that acts as one.
//
// Order is load-bearing within each half: rules registered earlier win their
// trigger byte, so strikethrough must precede typography's subscript rule on
// `~`. Applying all the rules and then all the renderings preserves that,
// because the two halves never contend with each other.
type Set []Extension

// Rules implements [Extension].
func (s Set) Rules(p *parser.RuleSet) {
	for _, e := range s {
		e.Rules(p)
	}
}

// Render implements [Extension].
func (s Set) Render(r renderer.Renderer) {
	for _, e := range s {
		e.Render(r)
	}
}

// ---- registration helpers ----
//
// The common shapes an output half needs, so a capability module writes its
// markup without reimplementing the open/close dispatch. Escaping is left to the
// standard library's html.EscapeString at each call site, so this package pulls
// in no more than the seam it exists to define.

// Paired registers a custom inline tag that wraps content in an open/close pair,
// the shape strikethrough (<del>) and the typography wrappers (<mark>, <sub>)
// share. It reports whether r could host the registration.
func Paired(r renderer.Renderer, tag token.Tag, open, close string) bool {
	return renderer.RegisterCustom(r, tag, func(w renderer.Writer, t token.Inline) {
		if t.Close {
			w.WriteString(close)
		} else {
			w.WriteString(open)
		}
	})
}

// Container registers a custom container tag's opening and closing markup, the
// shape a table's <table>/<thead>/<tr> wrappers share. It reports whether r
// could host the registration.
func Container(r renderer.Renderer, tag token.Tag, open, close string) bool {
	return renderer.RegisterCustomContainer(r, tag, func(w renderer.Writer, ev token.BlockEvent) {
		if ev.Type == token.OpenBlock {
			w.WriteString(open)
		} else {
			w.WriteString(close)
		}
	})
}
