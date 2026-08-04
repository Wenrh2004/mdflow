package mdflow

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

// Builder assembles a Parser.
//
// It exists because configuration has an order and options do not. An option
// that *performs* its work as it runs cannot commute with one that replaces the
// thing it worked on: supplying a renderer used to discard every capability
// already registered against the old one, and WithHTML5 landed or did not
// depending on where it sat in the argument list.
//
// So nothing here configures anything. Each method records an intention, and
// [Builder.Build] wires them once, in a fixed order:
//
//	rules and renderer settle → capabilities apply → renderer tweaks apply
//
// [Option] is a function over a Builder and [New] delegates here, so both
// spellings share one implementation and neither can drift.
type Builder struct {
	rules  *parser.RuleSet
	rend   renderer.Renderer
	base   extension.Extension // the starting set, replaced by Only; nil by default
	extra  []extension.Extension
	tweaks []func(renderer.Renderer)
}

// NewBuilder starts a Builder with the CommonMark-subset rules and no
// extensions, matching [New]. Add capabilities with [Builder.Use] or
// [Builder.Only], each imported from its own module.
func NewBuilder() *Builder { return &Builder{base: nil} }

// Rules supplies the rule set to build on, instead of the CommonMark default.
func (b *Builder) Rules(rs *parser.RuleSet) *Builder { b.rules = rs; return b }

// Renderer supplies the renderer, instead of the default HTML one.
func (b *Builder) Renderer(r renderer.Renderer) *Builder { b.rend = r; return b }

// Use adds capabilities on top of whatever set is already selected.
func (b *Builder) Use(exts ...extension.Extension) *Builder {
	b.extra = append(b.extra, exts...)
	return b
}

// Only replaces the default capability set with exactly the ones named,
// discarding anything added so far. Only() with no arguments builds a parser
// with no extensions at all.
func (b *Builder) Only(exts ...extension.Extension) *Builder {
	b.base = extension.Set(exts)
	b.extra = nil
	return b
}

// With applies options, so the two spellings compose.
func (b *Builder) With(opts ...Option) *Builder {
	for _, o := range opts {
		o(b)
	}
	return b
}

// tweak records a renderer adjustment to run after every capability has been
// wired, so it always sees the renderer that is actually in use.
func (b *Builder) tweak(f func(renderer.Renderer)) { b.tweaks = append(b.tweaks, f) }

// Build wires everything recorded so far and returns the Parser.
func (b *Builder) Build() *Parser {
	spec := &Spec{Rules: b.rules, Renderer: b.rend}
	if spec.Rules == nil {
		spec.Rules = parser.New()
	}
	if spec.Renderer == nil {
		spec.Renderer = html.NewRenderer()
	}
	if b.base != nil {
		spec.apply(b.base)
	}
	spec.apply(b.extra...)
	for _, t := range b.tweaks {
		t(spec.Renderer)
	}
	return newParser(spec, nil, 0)
}

// ---- options ----

// Option records one configuration choice on a [Builder].
type Option func(*Builder)

// WithExtensions adds capabilities to the default set.
func WithExtensions(exts ...extension.Extension) Option {
	return func(b *Builder) { b.Use(exts...) }
}

// WithOnly replaces the default capability set with exactly the ones named.
func WithOnly(exts ...extension.Extension) Option {
	return func(b *Builder) { b.Only(exts...) }
}

// WithRules supplies the rule set to build on.
func WithRules(rs *parser.RuleSet) Option { return func(b *Builder) { b.Rules(rs) } }

// WithRenderer replaces the renderer wholesale.
func WithRenderer(r renderer.Renderer) Option { return func(b *Builder) { b.Renderer(r) } }

// WithHTML5 emits `<br>`/`<hr>`/`<img>` instead of their self-closing XHTML
// spellings. It applies to the HTML renderer and is ignored by any other.
func WithHTML5() Option {
	return func(b *Builder) {
		b.tweak(func(r renderer.Renderer) {
			if h, ok := r.(*html.Renderer); ok {
				h.XHTML = false
			}
		})
	}
}
