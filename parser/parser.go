// Package parser turns markdown text into a stream of block and inline events.
//
// It knows nothing about output. A [RuleSet] holds only syntax — container
// rules, leaf rules and inline rules, in a registry modelled on markdown-it's
// ruler. Choosing a Renderer and wiring the two together is the facade's job,
// which is what keeps a new output format from touching this package at all.
//
//	ContainerRule  opens a container block (blockquote, list, ...)
//	LeafRule       classifies a line as a leaf block (heading, fence, rule, ...)
//	InlineRule     parses an inline construct, keyed by trigger byte
//	Continuation   claims a line for an already-open multi-line leaf
//
// The state machines, the lexical predicates and the concrete rules all live
// here, because rules need the state machine's internals beside them. What is
// exported is what a rule author uses. Driving the machines — feeding lines,
// sealing references, pausing and resuming inline scans — is the mdflow
// facade's job; those operations are unexported here and reach the facade
// through an internal package, so they can change without breaking a rule.
package parser
