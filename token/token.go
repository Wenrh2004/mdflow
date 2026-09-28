// Package token is the vocabulary the parser and the renderers share: node
// kinds, the flat inline token, the closed leaf block, and the block event.
//
// It is deliberately not called "ast" and deliberately holds no tree. An
// [Inline] carries a Close flag rather than children, so a document is a linear
// token stream that can be emitted and rendered in one pass — the representation
// a syntax tree would replace, not a step toward one.
//
// This package depends on nothing.
package token

// Node identifies a node type. Block-level and inline nodes share one
// enumeration so that a single flat event stream can express both.
type Node uint8

// The enumeration is CommonMark plus three open kinds, and nothing else. GFM
// tables, math, hashtags and the rest are *extensions*: they arrive as a Custom,
// CustomLeaf or CustomContainer carrying a Tag, on exactly the same footing as
// syntax a third party invents. That is what lets this package and renderer/html
// stay ignorant of every flavour built on top of them.
//
// Block kinds sort before [Text] and inline kinds after it, which is the
// invariant [Event.IsBlock] reads.
const (
	// ---- block ----

	Document Node = iota
	Heading
	Paragraph
	CodeBlock
	Blockquote
	List
	ListItem
	ThematicBreak

	// CustomLeaf is an extension leaf block: Leaf{Node: CustomLeaf, Tag: ...}.
	CustomLeaf
	// CustomContainer is an extension container block:
	// BlockEvent{Container: CustomContainer, Tag: ...}.
	CustomContainer

	// ---- inline ----

	Text
	Emph
	Strong
	CodeSpan
	Link
	Image
	HardBreak

	// Custom is an extension inline node: Inline{Node: Custom, Tag: ...}.
	Custom

	// SoftBreak was added after the original node set so existing numeric node
	// values remain stable.
	SoftBreak
)

var nodeNames = [...]string{
	Document:        "document",
	Heading:         "heading",
	Paragraph:       "paragraph",
	CodeBlock:       "code_block",
	Blockquote:      "blockquote",
	List:            "list",
	ListItem:        "list_item",
	ThematicBreak:   "thematic_break",
	CustomLeaf:      "custom_leaf",
	CustomContainer: "custom_container",
	Text:            "text",
	Emph:            "emph",
	Strong:          "strong",
	CodeSpan:        "code_span",
	Link:            "link",
	Image:           "image",
	HardBreak:       "hard_break",
	Custom:          "custom",
	SoftBreak:       "soft_break",
}

// String implements fmt.Stringer.
func (k Node) String() string {
	if int(k) < len(nodeNames) && nodeNames[k] != "" {
		return nodeNames[k]
	}
	return "unknown"
}

// Align is a GFM table column alignment.
type Align uint8

// Table column alignments.
const (
	AlignDefault Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// InlineContext describes structural facts about the leaf whose inline content
// is being parsed. It is a bit set so future extensions can consume orthogonal
// context without teaching the core about their syntax.
type InlineContext uint8

const (
	// InlineContextListItemHead marks the first direct Paragraph child of a
	// ListItem. It describes structure only; extensions decide how to use it.
	InlineContextListItemHead InlineContext = 1 << iota
)

// Inline is one flat inline token — the same shape as pulldown-cmark's
// Start/End events or markdown-it's _open/_close tokens, deliberately *not* a
// tree with Children.
//
//   - Paired nodes (Emph / Strong / Link / Image / Custom) are two tokens: an
//     opening one (Close=false) and a closing one (Close=true). Everything
//     between them in the stream is their content.
//   - Leaf tokens (Text / CodeSpan / SoftBreak / HardBreak) always have Close=false and
//     carry their own content.
//
// Keeping inline output flat means linear emission and linear rendering: no
// tree, no per-node Children slice allocation.
type Inline struct {
	Text  string // literal content of Text / CodeSpan
	Dest  string // target URL of a Link / Image open token
	Title string // optional title attribute of a link/image
	Node  Node
	Tag   Tag  // Custom discriminator; the renderer dispatches on it
	Close bool // paired nodes: false=open, true=close; leaf tokens always false
}

// Leaf is a closed leaf block: the output of the block phase and the input of
// the inline phase.
//
// It is a pure value type (no shared pointers), which is what lets it be handed
// across goroutines without any copying discipline.
// Fields are ordered widest-first so the single-byte ones share one tail word
// rather than each forcing its own padding. A Leaf is copied by value on every
// block event, and BlockEvent embeds one, so the packing is worth the slightly
// less tidy reading order.
type Leaf struct {
	Info    string // fenced code info string (e.g. "go")
	Content string // raw text (inline not yet parsed; literal for code blocks)
	Level   int    // heading level 1-6
	Node    Node   // Heading | Paragraph | CodeBlock | ThematicBreak | CustomLeaf
	Tag     Tag    // CustomLeaf discriminator; the renderer dispatches on it
	Literal bool   // Content is verbatim text and must not be inline-parsed
	Tight   bool   // inside a tight list item: render without <p>
	// BreakAfter separates a tight paragraph from its following block sibling.
	// It is decided when the containing list closes, alongside Tight.
	BreakAfter bool
	Align      Align
	Header     bool // table cell in the header row
	Context    InlineContext
}

// BlockOp discriminates block event types.
type BlockOp uint8

const (
	// LeafBlock: a leaf block closed; its Content is ready for inline parsing.
	LeafBlock BlockOp = iota
	// OpenBlock: a container block (blockquote/list/list_item/table/...) opened.
	OpenBlock
	// CloseBlock: a container block closed.
	CloseBlock
)

// BlockEvent is what the block parser emits. Seq is the document ordinal; it
// doubles as the reordering key for any concurrent consumer, so that inline
// work finishing out of order still reassembles into document order.
// Fields are ordered widest-first, as in [Leaf]: one is copied per block, and
// a held list keeps many alive at once, so the padding is not free.
type BlockEvent struct {
	Leaf      Leaf // valid when Type == LeafBlock
	Seq       int
	Start     int     // Container == List: first ordinal
	Type      BlockOp //
	Container Node    // valid when Type == OpenBlock/CloseBlock
	Tag       Tag     // Container == CustomContainer discriminator
	Ordered   bool    // Container == List: ordered list?
	Tight     bool    // Container == List: final tightness
	Newline   bool    // Container == ListItem: first block starts on a new line
	Align     Align
}
