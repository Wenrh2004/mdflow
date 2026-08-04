// Package renderer defines how parsed structure becomes output.
//
// Parsing and serialisation share only the token types, so a new output format
// (plain text, JSON, a terminal pager) is a new implementation of [Renderer]
// rather than a change anywhere in the parser.
//
// # Capabilities
//
// [Renderer] is the floor: enough to serialise a CommonMark document. Anything
// beyond that — hosting the custom nodes an extension introduces, or deep-copying
// itself so a derived parser can diverge — is an *optional capability*, declared
// as its own small interface in the manner of io.ReaderFrom or http.Flusher.
//
// This is what lets package extension configure output without importing
// renderer/html. It also makes an unsupported capability a condition a caller
// can observe: the [RegisterCustom] family reports whether the registration
// landed, where a type assertion against a concrete renderer could only skip in
// silence.
//
// A Renderer carrying mutable configuration should implement [Cloner]. One that
// does not is shared between a parser and every parser derived from it, so its
// configuration must be treated as immutable once built.
package renderer

import (
	"io"

	"github.com/Wenrh2004/mdflow/token"
)

// Writer is the minimal sink a renderer writes into. Both *strings.Builder and
// *bufio.Writer satisfy it.
//
// Requiring WriteString and WriteByte alongside Write is what lets renderers
// emit markup without allocating a []byte for every tag.
type Writer interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

// Renderer serialises parsed structure into some output format.
//
// RenderLeaf receives a closed leaf block together with its already-parsed
// inline tokens (nil for code blocks and thematic breaks); RenderContainer
// writes a container's opening or closing markup; RenderInlines walks a flat
// token run, and is part of the interface so that a custom leaf's registered
// renderer can emit that leaf's inline content without reimplementing the walk.
type Renderer interface {
	RenderLeaf(w Writer, leaf token.Leaf, inlines []token.Inline)
	RenderContainer(w Writer, ev token.BlockEvent)
	RenderInlines(w Writer, toks []token.Inline)
}

// Render funcs for the three levels at which an extension can introduce a node.
//
// Only [LeafRenderFunc] receives the Renderer: a custom leaf owns inline content
// and has to hand it back to be rendered. Inline and container nodes are
// open/close markup around a stream that renders itself, so giving them a
// recursion handle would add a parameter with no caller — which is exactly what
// the previous *html.Renderer parameter was, and why no interface could carry it.
type (
	// InlineRenderFunc renders one inline token.
	InlineRenderFunc func(w Writer, tok token.Inline)

	// LeafRenderFunc renders one custom leaf block and its inline content.
	LeafRenderFunc func(w Writer, leaf token.Leaf, inlines []token.Inline, r Renderer)

	// ContainerRenderFunc writes a custom container's opening or closing markup.
	ContainerRenderFunc func(w Writer, ev token.BlockEvent)
)

// The optional capabilities. A Renderer implements the ones its output format
// can meaningfully support; the helpers below turn "does it?" into a bool
// instead of a type assertion at every call site.
type (
	// Cloner deep-copies a Renderer, so a derived parser can register its own
	// output without the original seeing it.
	Cloner interface{ Clone() Renderer }

	// CustomRegistrar hosts custom inline nodes, keyed by tag.
	CustomRegistrar interface {
		RegisterCustom(tag token.Tag, fn InlineRenderFunc)
	}

	// CustomLeafRegistrar hosts custom leaf blocks, keyed by tag.
	CustomLeafRegistrar interface {
		RegisterCustomLeaf(tag token.Tag, fn LeafRenderFunc)
	}

	// CustomContainerRegistrar hosts custom container blocks, keyed by tag.
	CustomContainerRegistrar interface {
		RegisterCustomContainer(tag token.Tag, fn ContainerRenderFunc)
	}

	// NodeOverrider replaces how a built-in inline node renders, e.g. to add
	// rel="nofollow" to every link.
	NodeOverrider interface {
		OverrideNode(node token.Node, fn InlineRenderFunc)
	}
)

// RegisterCustom registers fn as the rendering for a custom inline tag,
// reporting whether r was able to accept it.
func RegisterCustom(r Renderer, tag token.Tag, fn InlineRenderFunc) bool {
	reg, ok := r.(CustomRegistrar)
	if ok {
		reg.RegisterCustom(tag, fn)
	}
	return ok
}

// RegisterCustomLeaf registers fn as the rendering for a custom leaf block,
// reporting whether r was able to accept it.
func RegisterCustomLeaf(r Renderer, tag token.Tag, fn LeafRenderFunc) bool {
	reg, ok := r.(CustomLeafRegistrar)
	if ok {
		reg.RegisterCustomLeaf(tag, fn)
	}
	return ok
}

// RegisterCustomContainer registers fn as the rendering for a custom container,
// reporting whether r was able to accept it.
func RegisterCustomContainer(r Renderer, tag token.Tag, fn ContainerRenderFunc) bool {
	reg, ok := r.(CustomContainerRegistrar)
	if ok {
		reg.RegisterCustomContainer(tag, fn)
	}
	return ok
}

// OverrideNode replaces the rendering of a built-in node kind, reporting
// whether r was able to accept it.
func OverrideNode(r Renderer, node token.Node, fn InlineRenderFunc) bool {
	ov, ok := r.(NodeOverrider)
	if ok {
		ov.OverrideNode(node, fn)
	}
	return ok
}

// Clone returns a deep copy of r when r implements [Cloner], and r itself
// otherwise.
//
// Returning the original is safe only because a Renderer that omits Cloner is
// thereby declaring that it carries no per-parser mutable state. One that does
// carry such state and omits Cloner will be shared silently — which is the
// contract Cloner exists to let a renderer opt out of.
func Clone(r Renderer) Renderer {
	if c, ok := r.(Cloner); ok {
		return c.Clone()
	}
	return r
}
