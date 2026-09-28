package mdflow

import (
	"iter"
	"sync"

	"github.com/Wenrh2004/mdflow/internal/drive"

	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
)

// This file is the facade skeleton: the types a Parser is made of, its
// construction, the chaining layer that derives new parsers, and the object
// pool. The terminal operations that consume a Parser live in render.go,
// events.go and extract.go.

// Middleware transforms the unified event stream. It is the unit of composition
// for the functional layer: every Map/Filter/Tap on a Parser is sugar for one.
type Middleware func(iter.Seq[Event]) iter.Seq[Event]

// Spec is the pair of things a Parser needs: syntax and output.
//
// It is the wired result, not a request — [Builder] holds the pending choices
// and produces this once.
type Spec struct {
	Rules    *parser.RuleSet
	Renderer renderer.Renderer
}

func (s *Spec) clone() *Spec {
	return &Spec{Rules: s.Rules.Clone(), Renderer: renderer.Clone(s.Renderer)}
}

// apply wires both halves of each capability: all the syntax first, then all
// the output. Splitting the phases keeps registration order meaningful within
// each without the two contending.
func (s *Spec) apply(exts ...extension.Extension) {
	for _, e := range exts {
		e.Rules(s.Rules)
	}
	for _, e := range exts {
		e.Render(s.Renderer)
	}
}

// Parser is a reusable, immutable, concurrency-safe markdown parser.
//
// Every chaining method returns a *new* Parser sharing the old one's rule set,
// so a configured Parser can live in a package-level var and be derived from
// freely without locking:
//
//	var base = mdflow.New()
//	var plain = base.Transform(mdflow.Drop(mdflow.IsImage))
//
// The zero value is not usable; call New.
type Parser struct {
	cfg      *Spec
	pipeline []Middleware
	chain    Middleware // pre-composed pipeline, nil when empty
	pool     sync.Pool  // drive.Block, keyed to cfg
}

// New builds the complete CommonMark 0.31.2 facade: package parser supplies the
// built-in rules, the default HTML renderer supplies markup, and raw HTML is
// composed through its extension capability with safe escaping. Use
// rawhtml.WithUnsafeHTML only for trusted input that requires verbatim output.
//
//	import "github.com/Wenrh2004/mdflow/extension/rawhtml"
//	trusted := mdflow.New(rawhtml.WithUnsafeHTML())
//
// Flavour syntax remains opt-in. In particular, task lists belong to GFM and
// are not part of the core parser vocabulary.
//
// To add capabilities, compose the layers directly or use the umbrella `all`
// module for the full set:
//
//	import "github.com/Wenrh2004/mdflow/extension/gfm"
//	p := mdflow.New(mdflow.WithExtensions(gfm.GFM))
//
//	import "github.com/Wenrh2004/mdflow/all"
//	p := all.New() // CommonMark + bundled GFM and Memos extensions
//
// A Parser is immutable, so New with no options returns one shared instance
// rather than rebuilding the same rule set and renderer on every call — a
// handler that calls mdflow.New() per request pays for it once per process.
func New(opts ...Option) *Parser {
	if len(opts) == 0 {
		return defaultParser()
	}
	return NewBuilder().With(opts...).Build()
}

// NewWith builds a Parser from an explicit parser/renderer pair and only the
// extensions named. parser.New intentionally omits raw HTML, so an explicit
// profile that needs complete CommonMark must include rawhtml.RawHTML.
func NewWith(p *parser.RuleSet, r renderer.Renderer, exts ...extension.Extension) *Parser {
	return NewBuilder().Rules(p).Renderer(r).Only(exts...).Build()
}

func newParser(cfg *Spec, pipeline []Middleware) *Parser {
	p := &Parser{cfg: cfg, pipeline: pipeline}
	if len(pipeline) > 0 {
		// iterx.Compose is generic over the element type, but Middleware is a
		// named func type, so a []Middleware will not spread into its variadic
		// of the underlying signature. Restate the stages as that signature.
		stages := make([]func(iter.Seq[Event]) iter.Seq[Event], len(pipeline))
		for i, m := range pipeline {
			stages[i] = m
		}
		p.chain = iterx.Compose(stages...)
	}
	p.pool.New = func() any { return drive.NewBlock(cfg.Rules) }
	return p
}

// ---- chaining (each returns a derived Parser; the receiver is untouched) ----

// WithExtensions derives a Parser with additional capabilities. It deep-copies
// the rule set and the renderer, so the receiver keeps working with its original
// configuration.
//
// The name says "derive", not "register": Builder.Use registers into a config
// being assembled and costs nothing, while this clones a whole rule set and
// renderer. One verb for two cost models made that invisible at the call site.
func (p *Parser) WithExtensions(exts ...extension.Extension) *Parser {
	spec := p.cfg.clone()
	spec.apply(exts...)
	return newParser(spec, p.pipeline)
}

// With derives a Parser with opts applied on top of the receiver's
// configuration: its capabilities, renderer settings, middlewares and worker
// count carry over, and the receiver is untouched. It is how a construction
// option joins a chain:
//
//	md := mdflow.New().
//		With(mdflow.WithSafeLinks(), mdflow.WithHTML5()).
//		Transform(mdflow.ShiftHeadings(1))
//
// Options that add (WithExtensions, WithOutput, WithSafeLinks, WithURLPolicy,
// WithHTML5) extend what the receiver already has. Options that replace
// (WithRules, WithRenderer, WithOnly) start from what they supply instead, as
// they would in [New]; capabilities already applied to the receiver cannot be
// subtracted.
func (p *Parser) With(opts ...Option) *Parser {
	if len(opts) == 0 {
		return p
	}
	b := &Builder{rules: p.cfg.Rules, rend: p.cfg.Renderer}
	derived := b.With(opts...).Build()
	return newParser(derived.cfg, p.pipeline)
}

// Transform derives a Parser with additional event middlewares appended.
func (p *Parser) Transform(ms ...Middleware) *Parser {
	if len(ms) == 0 {
		return p
	}
	next := make([]Middleware, 0, len(p.pipeline)+len(ms))
	next = append(next, p.pipeline...)
	next = append(next, ms...)
	return newParser(p.cfg, next)
}

// Map derives a Parser that rewrites every event with f.
func (p *Parser) Map(f func(Event) Event) *Parser {
	return p.Transform(func(seq iter.Seq[Event]) iter.Seq[Event] { return iterx.Map(seq, f) })
}

// Filter derives a Parser that keeps only events satisfying pred.
//
// Dropping one half of an EnterEvent/LeaveEvent pair produces unbalanced markup, so
// prefer the span-aware helpers (Drop, Unwrap) for structural edits.
func (p *Parser) Filter(pred func(Event) bool) *Parser {
	return p.Transform(func(seq iter.Seq[Event]) iter.Seq[Event] { return iterx.Filter(seq, pred) })
}

// Reject derives a Parser that drops events satisfying pred.
func (p *Parser) Reject(pred func(Event) bool) *Parser {
	return p.Filter(func(e Event) bool { return !pred(e) })
}

// Tap derives a Parser that calls f for every event and passes it through
// unchanged — the escape hatch for metrics, logging and side-channel collection.
func (p *Parser) Tap(f func(Event)) *Parser {
	return p.Map(func(e Event) Event {
		f(e)
		return e
	})
}

// Spec returns a deep copy of the parser's rule set and renderer. Changing the
// copy never affects p — a Parser is immutable, which is what makes it safe to
// share. To derive a parser with more syntax or output, use
// [Parser.WithExtensions] or [Parser.With]; to start from the copy, pass its
// fields to [NewWith].
func (p *Parser) Spec() *Spec { return p.cfg.clone() }

// ---- pooling ----

func (p *Parser) borrow() drive.Block {
	bp := p.pool.Get().(drive.Block)
	bp.Reset()
	return bp
}

func (p *Parser) release(bp drive.Block) {
	bp.Reset()
	p.pool.Put(bp)
}
