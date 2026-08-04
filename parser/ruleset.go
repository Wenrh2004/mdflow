package parser

import (
	"github.com/Wenrh2004/mdflow/token"
)

// ---- rule interfaces (public, for extensions) ----

// ContainerRule tries to open a container block at the current line.
//
// Continuation of the *built-in* container kinds lives in continueContainer;
// a container rule is only responsible for *opening*. That covers essentially
// every extension need (introducing a new container entry syntax).
type ContainerRule interface {
	Name() string
	// Open tries to open a container; on success it returns the remainder of
	// the line with the marker stripped.
	Open(s *BlockState, line string) (rest string, ok bool)
}

// LeafRule tries to classify one line (container prefixes already stripped)
// as a leaf block.
type LeafRule interface {
	Name() string
	// Open returns true if it consumed the line.
	Open(s *BlockState, line string) bool
}

// InlineRule parses one inline construct.
type InlineRule interface {
	Name() string
	// Triggers returns the bytes that activate this rule (e.g. '`', '[', '*').
	// The scanner only attempts the rule when it sees one of them.
	Triggers() []byte
	// Match tries to parse at s.Pos(); on success it advances s and returns true.
	Match(s *InlineState) bool
}

// inlinePost is whole-sequence inline post-processing (emphasis pairing).
type inlinePost interface {
	process(items []inlineItem) []inlineItem
}

// ---- RuleSet ----

// RuleSet holds one complete rule set. It is used immutably: once built it is
// shared read-only, including across goroutines. Rules themselves are
// stateless; all per-parse mutable state lives in BlockState / InlineState.
type RuleSet struct {
	containerRules []ContainerRule
	leafRules      []LeafRule
	paragraph      LeafRule // fallback leaf rule, always tried last
	inline         *InlineRules
	// continuations let a rule own the lines of a multi-line leaf it opened
	// (a table's body rows, a math block's contents) before the rule loop runs.
	continuations []Continuation
	// finalisers turn a multi-event accumulating leaf into block events when it
	// closes. Keyed by the leaf's tag, so the core state machine can close a
	// table (or anything shaped like one) without knowing what a table is.
	finalisers map[token.Tag]finalise
}

// Continuation claims the current line for an already-open multi-line leaf.
// It returns true when it consumed the line.
type Continuation func(s *BlockState, line string) bool

// finalise is the stored, type-erased finaliser. Its scratch is boxed as any so
// one map can hold finalisers over different scratch types; [AddFinalise] is the
// only way to install one and unboxes back to the caller's type, so an extension
// author never writes the any or the assertion themselves.
type finalise func(s *BlockState, lines []string, scratch any)

// New builds a RuleSet with the CommonMark-subset rules registered.
//
// GFM and Memos-flavoured syntax is deliberately absent here; it lives in
// package extension, so this package stays the smallest thing that can parse a
// document. The one exception is the task-list marker, which is a few lines
// inside the list rule and not worth a hook.
func New() *RuleSet {
	c := &RuleSet{
		paragraph:  paragraphRule{},
		inline:     &InlineRules{rules: make(map[byte][]InlineRule, 16)},
		finalisers: make(map[token.Tag]finalise),
	}

	// Containers: blockquote, list.
	c.AddContainerRule(blockquoteRule{})
	c.AddContainerRule(listRule{})

	// Leaves, in priority order. setext must precede thematicBreak so that
	// `---` under a paragraph reads as <h2>, not <hr>.
	c.AddLeafRule(blankRule{})
	c.AddLeafRule(setextHeadingRule{})
	c.AddLeafRule(thematicBreakRule{})
	c.AddLeafRule(atxHeadingRule{})
	c.AddLeafRule(fenceRule{})

	// Inline rules. Within one trigger byte, registration order is priority.
	c.AddInlineRule(escapeRule{})
	c.AddInlineRule(codeSpanRule{})
	c.AddInlineRule(hardBreakRule{})
	c.AddInlineRule(imageRule{})
	c.AddInlineRule(linkRule{})
	c.AddInlineRule(autolinkRule{})
	c.AddInlineRule(emphasisRule{})
	c.inline.post = []inlinePost{emphasisPost{}}

	return c
}

// Clone deep-copies everything an extension could mutate, so a derived parser
// can register rules without the original seeing them. Rule values themselves
// are stateless and shared.
func (c *RuleSet) Clone() *RuleSet {
	out := &RuleSet{
		containerRules: append([]ContainerRule(nil), c.containerRules...),
		leafRules:      append([]LeafRule(nil), c.leafRules...),
		paragraph:      c.paragraph,
		continuations:  append([]Continuation(nil), c.continuations...),
		finalisers:     make(map[token.Tag]finalise, len(c.finalisers)),
		inline: &InlineRules{
			rules:    make(map[byte][]InlineRule, len(c.inline.rules)),
			post:     append([]inlinePost(nil), c.inline.post...),
			triggers: c.inline.triggers,
		},
	}
	for k, v := range c.finalisers {
		out.finalisers[k] = v
	}
	for k, v := range c.inline.rules {
		out.inline.rules[k] = append([]InlineRule(nil), v...)
	}
	return out
}

// AddContainerRule registers a container rule.
func (c *RuleSet) AddContainerRule(r ContainerRule) { c.containerRules = append(c.containerRules, r) }

// AddLeafRule registers a leaf rule ahead of the paragraph fallback.
func (c *RuleSet) AddLeafRule(r LeafRule) { c.leafRules = append(c.leafRules, r) }

// PrependLeafRule registers a leaf rule ahead of all existing ones, for syntax
// that must be considered before a more permissive rule swallows the line.
func (c *RuleSet) PrependLeafRule(r LeafRule) {
	c.leafRules = append([]LeafRule{r}, c.leafRules...)
}

// AddInlineRule registers an inline rule under each of its trigger bytes.
//
// Order matters within a trigger: the first rule that matches wins, which is
// how `~~strike~~` beats `~sub~` and `<url>` beats `<tag>`.
func (c *RuleSet) AddInlineRule(r InlineRule) {
	for _, t := range r.Triggers() {
		c.inline.rules[t] = append(c.inline.rules[t], r)
		c.inline.triggers[t] = true // feeds the no-markup fast path
	}
}

// PrependInlineRule registers an inline rule ahead of the rules already sharing
// a trigger byte — `[[ref]]` has to be tried before `[text](url)`.
func (c *RuleSet) PrependInlineRule(r InlineRule) {
	for _, t := range r.Triggers() {
		c.inline.rules[t] = append([]InlineRule{r}, c.inline.rules[t]...)
		c.inline.triggers[t] = true
	}
}

// AddContinuation registers a handler that can claim lines for a multi-line
// leaf. Handlers are tried in registration order before the leaf rules run.
func (c *RuleSet) AddContinuation(fn Continuation) { c.continuations = append(c.continuations, fn) }

// AddFinalise registers the finaliser for an accumulating leaf tag. fn runs when
// a leaf opened with that tag closes, receiving the buffered lines and the
// scratch — already typed as S — that [StartAccumulator] stowed, and turns them
// into block events with [BlockState.Emit].
//
// The generic signature is the whole point: the scratch a rule writes and the
// scratch a finaliser reads are one type, checked at compile time, with the
// any-boxing that lets one registry hold finalisers over different scratch types
// confined to this function. Registering the same tag twice replaces the
// earlier finaliser.
func AddFinalise[S any](c *RuleSet, tag token.Tag, fn func(s *BlockState, lines []string, scratch S)) {
	c.finalisers[tag] = func(s *BlockState, lines []string, scratch any) {
		var zero S
		if scratch != nil {
			zero, _ = scratch.(S)
		}
		fn(s, lines, zero)
	}
}

// ParseInline parses a leaf block's text into inline tokens.
//
// A leaf marked Literal holds verbatim text — a code fence, a math block, an
// embed target — and returns nil. That flag is what lets a custom leaf declare
// which it is: a table cell holds markdown and is parsed, a math block does not
// and is left alone, and this package needs to know nothing else about either.
func (c *RuleSet) ParseInline(leaf token.Leaf) []token.Inline {
	if leaf.Literal || leaf.Content == "" {
		return nil
	}
	switch leaf.Node {
	case token.Heading, token.Paragraph, token.CustomLeaf:
		return c.inline.Parse(leaf.Content)
	}
	return nil
}

// Inline exposes the inline parser, for callers parsing a fragment rather than
// a whole leaf.
func (c *RuleSet) Inline() *InlineRules { return c.inline }
