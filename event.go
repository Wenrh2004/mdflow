package mdflow

import "github.com/Wenrh2004/mdflow/token"

// A unified pull event stream, in the shape pulldown-cmark popularised: block
// and inline structure flattened into one linear EnterEvent/LeaveEvent/Text/Code stream.
// Containers, leaves, emphasis and links all appear as an EnterEvent ... LeaveEvent pair;
// text and inline code are leaf events.
//
// This is the surface the functional layer works on. It is a plain value type,
// so Map/Filter/Reduce over it are ordinary Go — no visitor interfaces, no AST
// walking, and no document tree is ever built.
//
// The stream lives here rather than in package token because nothing below this
// package emits or consumes it: the parser produces token.BlockEvent, the
// renderers consume token.Leaf and token.Inline, and the flattening into Event
// is the facade's own contribution.

// EventType discriminates unified events.
type EventType uint8

// Event types carry an -Event suffix, following the convention of
// golang.org/x/net/html (NodeType/…Node, TokenType/…Token). Node kinds are
// unprefixed, so the suffix is also what keeps TextEvent from colliding with
// the token.Text node kind.
const (
	EnterEvent EventType = iota // entering a node (container, leaf or paired inline)
	LeaveEvent                  // leaving a node
	TextEvent                   // a run of literal text
	CodeEvent                   // an inline code span
)

// String implements fmt.Stringer.
func (k EventType) String() string {
	switch k {
	case EnterEvent:
		return "enter"
	case LeaveEvent:
		return "leave"
	case TextEvent:
		return "text"
	case CodeEvent:
		return "code"
	}
	return "unknown"
}

// Event is one event in the unified stream.
type Event struct {
	Type EventType
	Node token.Node // for EnterEvent/LeaveEvent: which node

	Text  string    // Text/Code content; also a code block's literal body
	Dest  string    // Link / Image target
	Title string    // link/image title
	Tag   token.Tag // Custom discriminator
	Info  string    // CodeBlock info string

	Level   int  // Heading level
	Ordered bool // List
	Start   int  // List first ordinal
	Tight   bool // paragraph inside a tight list item
	Literal bool // leaf Enter: body is verbatim, arriving as one TextEvent

	Align  token.Align // TableCell
	Header bool        // TableCell in the header row
	Task   int8        // ListItem: 0 none, 1 unchecked, 2 checked
}

// IsBlock reports whether the event's node is a block-level node.
func (e Event) IsBlock() bool { return e.Node < token.Text }

// ---- conversion helpers ----

// ContainerEvent builds an Enter/Leave event for a container block.
func ContainerEvent(typ EventType, ev token.BlockEvent) Event {
	return Event{
		Type:    typ,
		Node:    ev.Container,
		Tag:     ev.Tag,
		Ordered: ev.Ordered,
		Start:   ev.Start,
		Task:    ev.Task,
	}
}

// InlineEvent converts one flat inline token into a unified event.
func InlineEvent(t token.Inline) Event {
	switch t.Node {
	case token.Text:
		return Event{Type: TextEvent, Node: token.Text, Text: t.Text}
	case token.CodeSpan:
		return Event{Type: CodeEvent, Node: token.CodeSpan, Text: t.Text}
	}
	typ := EnterEvent
	if t.Close {
		typ = LeaveEvent
	}
	return Event{Type: typ, Node: t.Node, Text: t.Text, Dest: t.Dest, Title: t.Title, Tag: t.Tag}
}

// isAtomicNode reports whether a node is self-contained: it appears as a single
// EnterEvent with no matching LeaveEvent, because it has no child content.
//
// Keeping them as one event rather than an empty EnterEvent/LeaveEvent pair is what lets
// [Event.InlineToken] round-trip exactly — a pair would reconstruct two tokens
// and render the node twice.
func isAtomicNode(k token.Node, tag token.Tag) bool {
	if k == token.HardBreak {
		return true
	}
	return k == token.Custom && tag.IsAtomic()
}

// IsAtomic reports whether the event is a self-contained node, i.e. an EnterEvent
// that will not be followed by a matching LeaveEvent.
func (e Event) IsAtomic() bool { return isAtomicNode(e.Node, e.Tag) }

// InlineToken is the inverse of InlineEvent, used when a transformed event
// stream has to be handed back to a Renderer.
func (e Event) InlineToken() token.Inline {
	switch e.Type {
	case TextEvent:
		return token.Inline{Node: token.Text, Text: e.Text}
	case CodeEvent:
		return token.Inline{Node: token.CodeSpan, Text: e.Text}
	case LeaveEvent:
		return token.Inline{Node: e.Node, Close: true, Text: e.Text, Dest: e.Dest, Title: e.Title, Tag: e.Tag}
	default:
		return token.Inline{Node: e.Node, Text: e.Text, Dest: e.Dest, Title: e.Title, Tag: e.Tag}
	}
}

// IsLeafNode reports whether a node kind is a leaf block, i.e. one whose EnterEvent
// starts buffering inline tokens.
func IsLeafNode(k token.Node) bool {
	switch k {
	case token.Heading, token.Paragraph, token.CodeBlock, token.ThematicBreak,
		token.CustomLeaf:
		return true
	}
	return false
}
