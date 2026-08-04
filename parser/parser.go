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
// here rather than behind an internal package. Rules need the state machine's
// internals, so they have to sit beside it — and a boundary that re-exported
// every one of those internals under a second name bought no encapsulation
// while costing the package its documentation, because a type alias renders
// none of its method set. What this package promises, it declares.
package parser

import "strings"

// EachLine splits src on '\n' without allocating a []string, trimming a
// trailing '\r' so CRLF input costs nothing extra. fn returning false stops the
// walk.
//
// It is exported because driving the block machine line by line is the caller's
// job on the streaming and fan-out paths, and every such caller has to agree
// with the parser about where a line ends.
func EachLine(src string, fn func(line string) bool) {
	for {
		i := strings.IndexByte(src, '\n')
		if i < 0 {
			break
		}
		line := src[:i]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if !fn(line) {
			return
		}
		src = src[i+1:]
	}
	if src != "" {
		if src[len(src)-1] == '\r' {
			src = src[:len(src)-1]
		}
		fn(src)
	}
}
