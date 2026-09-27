// Package html renders parsed structure as HTML.
//
// Design notes:
//
//  1. Renderer is an interface, so the output format is pluggable (HTML, plain
//     text, JSON, ...). Parsing and serialisation stay fully decoupled: the
//     parser emits []token.Inline and token.BlockEvent; the renderer only writes them out.
//  2. Everything writes into an io.Writer, so rendering streams and never
//     materialises a document-sized intermediate string.
//  3. Renderer dispatches through a table keyed by node kind / custom tag
//     (markdown-it's renderer.rules). Built-ins can be replaced via OverrideNode
//     and new node types registered via RegisterCustom, so an extension can
//     define both new syntax and its output without forking this package.
//
// Escaping is streamed directly to the destination. CommonMark's HTML output
// escapes &, <, > and ", while leaving apostrophes untouched.
package html

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/Wenrh2004/mdflow/internal/ascii"

	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// Renderer is the default HTML renderer.
type Renderer struct {
	overrides map[token.Node]renderer.InlineRenderFunc
	// Custom dispatch is indexed by token.Tag, not keyed by a string: a tag is a
	// small dense integer, so a handler lookup is a bounds check and a load
	// instead of hashing a string on every node of the document.
	custom     []renderer.InlineRenderFunc
	customLeaf []renderer.LeafRenderFunc
	customCont []renderer.ContainerRenderFunc
	// XHTML controls whether void elements are closed as `<br />` or `<br>`.
	XHTML bool
	// SafeLinks filters link and image destinations to an allowlist of URI
	// schemes, rendering an empty destination for anything else (javascript:,
	// data:, vbscript:, …). It is off by default so the profile stays
	// byte-for-byte CommonMark; mdflow.WithSafeLinks turns it on.
	SafeLinks bool
	// URLPolicy, when set, vets every link and image destination after the
	// SafeLinks scheme check. See [URLPolicy]; mdflow.WithURLPolicy sets it.
	URLPolicy URLPolicy
}

// NewRenderer builds the default HTML renderer.
func NewRenderer() *Renderer {
	return &Renderer{
		overrides: make(map[token.Node]renderer.InlineRenderFunc),
		XHTML:     true,
	}
}

// This renderer supports every optional capability in package renderer.
var (
	_ renderer.Renderer                 = (*Renderer)(nil)
	_ renderer.Cloner                   = (*Renderer)(nil)
	_ renderer.CustomRegistrar          = (*Renderer)(nil)
	_ renderer.CustomLeafRegistrar      = (*Renderer)(nil)
	_ renderer.CustomContainerRegistrar = (*Renderer)(nil)
	_ renderer.NodeOverrider            = (*Renderer)(nil)
	_ renderer.VoidElementCloser        = (*Renderer)(nil)
)

// OverrideNode replaces how a built-in inline node type renders (e.g. adding
// rel="nofollow" to every link). It implements [renderer.NodeOverrider].
func (h *Renderer) OverrideNode(node token.Node, fn renderer.InlineRenderFunc) {
	h.overrides[node] = fn
}

// RegisterCustom registers rendering for a [token.Custom] inline tag. It
// implements [renderer.CustomRegistrar].
func (h *Renderer) RegisterCustom(tag token.Tag, fn renderer.InlineRenderFunc) {
	h.custom = put(h.custom, tag, fn)
}

// RegisterCustomLeaf registers rendering for a [token.CustomLeaf] block. It
// implements [renderer.CustomLeafRegistrar].
func (h *Renderer) RegisterCustomLeaf(tag token.Tag, fn renderer.LeafRenderFunc) {
	h.customLeaf = put(h.customLeaf, tag, fn)
}

// RegisterCustomContainer registers rendering for a [token.CustomContainer]
// block. It implements [renderer.CustomContainerRegistrar].
func (h *Renderer) RegisterCustomContainer(tag token.Tag, fn renderer.ContainerRenderFunc) {
	h.customCont = put(h.customCont, tag, fn)
}

// CloseVoidElement implements [renderer.VoidElementCloser].
func (h *Renderer) CloseVoidElement(w renderer.Writer) {
	if h.XHTML {
		w.WriteString(" />")
	} else {
		w.WriteByte('>')
	}
}

// put stores v at tag, growing s as needed. Registration happens once at
// construction, so the reallocation is not on any hot path.
func put[T any](s []T, tag token.Tag, v T) []T {
	if int(tag) >= len(s) {
		grown := make([]T, int(tag)+1)
		copy(grown, s)
		s = grown
	}
	s[tag] = v
	return s
}

// Clone copies the dispatch tables so a derived parser can register overrides
// without the original seeing them. It implements [renderer.Cloner].
func (h *Renderer) Clone() renderer.Renderer {
	out := &Renderer{
		overrides:  make(map[token.Node]renderer.InlineRenderFunc, len(h.overrides)),
		custom:     slices.Clone(h.custom),
		customLeaf: slices.Clone(h.customLeaf),
		customCont: slices.Clone(h.customCont),
		XHTML:      h.XHTML,
		SafeLinks:  h.SafeLinks,
		URLPolicy:  h.URLPolicy,
	}
	maps.Copy(out.overrides, h.overrides)
	return out
}

// RenderLeaf implements Renderer.
func (h *Renderer) RenderLeaf(w renderer.Writer, leaf token.Leaf, inlines []token.Inline) {
	switch leaf.Node {
	case token.Heading:
		level := min(max(leaf.Level, 1), 6)
		w.WriteString(headingOpen[level])
		h.RenderInlines(w, inlines)
		w.WriteString(headingClose[level])
	case token.Paragraph:
		if leaf.Tight {
			h.RenderInlines(w, inlines) // tight list item: no <p> wrapper
			if leaf.BreakAfter {
				w.WriteByte('\n')
			}
			return
		}
		w.WriteString("<p>")
		h.RenderInlines(w, inlines)
		w.WriteString("</p>\n")
	case token.CodeBlock:
		w.WriteString("<pre><code")
		if lang := firstWord(leaf.Info); lang != "" {
			w.WriteString(` class="language-`)
			writeEscaped(w, lang)
			w.WriteByte('"')
		}
		w.WriteByte('>')
		writeEscaped(w, leaf.Content)
		w.WriteString("</code></pre>\n")
	case token.ThematicBreak:
		w.WriteString("<hr")
		h.CloseVoidElement(w)
		w.WriteByte('\n')
	case token.CustomLeaf:
		if int(leaf.Tag) < len(h.customLeaf) {
			if fn := h.customLeaf[leaf.Tag]; fn != nil {
				fn(w, leaf, inlines, h)
				return
			}
		}
		// Same reasoning as the inline case: emit the content rather than
		// discard the block because its rendering was never registered.
		//
		// One or the other, never both: a literal leaf keeps its body in
		// Content and has no inlines, while a markdown one has both, and
		// Content is the unparsed source of the very same text.
		if leaf.Literal {
			writeEscaped(w, leaf.Content)
			return
		}
		h.RenderInlines(w, inlines)
	}
}

var (
	headingOpen  = [7]string{"", "<h1>", "<h2>", "<h3>", "<h4>", "<h5>", "<h6>"}
	headingClose = [7]string{"", "</h1>\n", "</h2>\n", "</h3>\n", "</h4>\n", "</h5>\n", "</h6>\n"}
)

// RenderContainer implements Renderer.
func (h *Renderer) RenderContainer(w renderer.Writer, ev token.BlockEvent) {
	open := ev.Type == token.OpenBlock
	switch ev.Container {
	case token.Blockquote:
		w.WriteString(pick(open, "<blockquote>\n", "</blockquote>\n"))
	case token.List:
		switch {
		case open && ev.Ordered:
			if ev.Start != 1 {
				w.WriteString(`<ol start="`)
				w.WriteString(strconv.Itoa(ev.Start))
				w.WriteString("\">\n")
			} else {
				w.WriteString("<ol>\n")
			}
		case open:
			w.WriteString("<ul>\n")
		case ev.Ordered:
			w.WriteString("</ol>\n")
		default:
			w.WriteString("</ul>\n")
		}
	case token.ListItem:
		if !open {
			w.WriteString("</li>\n")
			return
		}
		w.WriteString("<li>")
		if ev.Newline {
			w.WriteByte('\n')
		}
	case token.CustomContainer:
		// No fallback: a container contributes structure, not content, so an
		// unregistered one is better skipped than guessed at. Its children
		// still render.
		if int(ev.Tag) < len(h.customCont) {
			if fn := h.customCont[ev.Tag]; fn != nil {
				fn(w, ev)
			}
		}
	}
}

// RenderInlines walks the flat token stream: paired nodes write their opening
// tag at the open token and their closing tag at the close token — no recursion,
// no Children. Images are the one exception: HTML has no closing tag for them,
// so their span is collapsed into a single <img> with a flattened alt.
func (h *Renderer) RenderInlines(w renderer.Writer, toks []token.Inline) {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if fn, ok := h.overrides[t.Node]; ok {
			fn(w, t)
			continue
		}
		switch t.Node {
		case token.Text:
			writeEscaped(w, t.Text)
		case token.CodeSpan:
			w.WriteString("<code>")
			writeEscaped(w, t.Text)
			w.WriteString("</code>")
		case token.SoftBreak:
			w.WriteByte('\n')
		case token.HardBreak:
			w.WriteString("<br")
			h.CloseVoidElement(w)
			w.WriteByte('\n')
		case token.Emph:
			w.WriteString(pick(!t.Close, "<em>", "</em>"))
		case token.Strong:
			w.WriteString(pick(!t.Close, "<strong>", "</strong>"))
		case token.Link:
			if t.Close {
				w.WriteString("</a>")
				break
			}
			w.WriteString(`<a href="`)
			if dest, ok := h.destination(LinkURL, t.Dest); ok {
				writeEscapedURL(w, dest)
			}
			if t.Title != "" {
				w.WriteString(`" title="`)
				writeEscaped(w, t.Title)
			}
			w.WriteString(`">`)
		case token.Image:
			if t.Close {
				break // consumed by the open token
			}
			end := findClose(toks, i, token.Image)
			dest, ok := h.destination(ImageURL, t.Dest)
			if !ok {
				// A refused image must not become an <img> at all: even an
				// empty src makes some browsers issue a request. Its alt text
				// is what a reader would have seen had it failed to load.
				writeEscaped(w, plainText(toks[i+1:end]))
				i = end
				break
			}
			w.WriteString(`<img src="`)
			writeEscapedURL(w, dest)
			w.WriteString(`" alt="`)
			writeEscaped(w, plainText(toks[i+1:end]))
			if t.Title != "" {
				w.WriteString(`" title="`)
				writeEscaped(w, t.Title)
			}
			w.WriteByte('"')
			h.CloseVoidElement(w)
			i = end
		case token.Custom:
			if int(t.Tag) < len(h.custom) {
				if fn := h.custom[t.Tag]; fn != nil {
					fn(w, t)
					break
				}
			}
			// No rendering registered for this tag — the syntax was enabled
			// without its output half. Emit the token's own text rather than
			// nothing: dropping it would delete the author's prose to punish a
			// configuration mistake, and the paired open/close tokens carry the
			// content between them either way.
			writeEscaped(w, t.Text)
		}
	}
}

// destination applies the configured URL checks to a decoded destination:
// first the SafeLinks scheme allowlist, then the URLPolicy.
func (h *Renderer) destination(kind URLKind, dest string) (string, bool) {
	if h.SafeLinks && !linkSchemeAllowed(dest) {
		return "", false
	}
	if h.URLPolicy != nil {
		return h.URLPolicy(kind, dest)
	}
	return dest, true
}

// findClose returns the index of the close token matching the open token at i,
// or len(toks) when the stream is truncated.
func findClose(toks []token.Inline, i int, node token.Node) int {
	depth := 0
	for j := i + 1; j < len(toks); j++ {
		if toks[j].Node != node {
			continue
		}
		if !toks[j].Close {
			depth++
			continue
		}
		if depth == 0 {
			return j
		}
		depth--
	}
	return len(toks)
}

// plainText flattens a token span to its literal text (used for image alt).
func plainText(toks []token.Inline) string {
	n := 0
	for _, t := range toks {
		switch t.Node {
		case token.Text, token.CodeSpan:
			n += len(t.Text)
		case token.SoftBreak, token.HardBreak:
			n++
		}
	}
	if n == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	for _, t := range toks {
		switch t.Node {
		case token.Text, token.CodeSpan:
			b.WriteString(t.Text)
		case token.SoftBreak, token.HardBreak:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// writeEscaped streams CommonMark's four HTML escapes without allocating an
// intermediate escaped string.
func writeEscaped(w renderer.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		var escaped string
		switch s[i] {
		case '&':
			escaped = "&amp;"
		case '<':
			escaped = "&lt;"
		case '>':
			escaped = "&gt;"
		case '"':
			escaped = "&quot;"
		default:
			continue
		}
		if start < i {
			w.WriteString(s[start:i])
		}
		w.WriteString(escaped)
		start = i + 1
	}
	if start < len(s) {
		w.WriteString(s[start:])
	}
}

const upperHex = "0123456789ABCDEF"

// writeEscapedURL follows the CommonMark reference renderer's href escaping:
// URL-safe ASCII and existing percent escapes pass through, ampersand and
// apostrophe use HTML entities, and every other byte is percent-encoded. Since
// non-ASCII input is UTF-8, encoding by byte produces its standard URL form.
func writeEscapedURL(w renderer.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x80 && isURLSafeASCII(c) {
			continue
		}
		if start < i {
			w.WriteString(s[start:i])
		}
		switch c {
		case '&':
			w.WriteString("&amp;")
		case '\'':
			w.WriteString("&#x27;")
		default:
			w.WriteByte('%')
			w.WriteByte(upperHex[c>>4])
			w.WriteByte(upperHex[c&0x0f])
		}
		start = i + 1
	}
	if start < len(s) {
		w.WriteString(s[start:])
	}
}

// safeLinkSchemes are the URI schemes WithSafeLinks permits in a link or image
// destination. Every other scheme — javascript:, data:, vbscript:, … — is
// treated as unsafe and rendered with an empty destination.
var safeLinkSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
	"tel":    true,
}

// linkSchemeAllowed reports whether dest may be emitted as a destination under
// WithSafeLinks. A relative reference (no scheme) is always allowed; a scheme is
// allowed only if it is on the allowlist. The scheme is read the way a browser's
// URL parser sees it — leading control/space stripped and embedded tab/newline
// removed — so a decoded entity or a "java&#9;script:" cannot smuggle a
// disallowed scheme past the check. dest is the already entity-decoded
// destination, which is exactly why the filter belongs here at the renderer.
func linkSchemeAllowed(dest string) bool {
	var buf [24]byte
	n := 0
	i := 0
	for i < len(dest) && dest[i] <= ' ' {
		i++ // the URL parser strips leading C0 controls and spaces
	}
	for ; i < len(dest); i++ {
		c := dest[i]
		switch {
		case c == '\t' || c == '\n' || c == '\r':
			continue // removed from the URL before scheme parsing
		case c == ':':
			if n == 0 {
				return true // empty scheme: treat as a relative reference
			}
			return safeLinkSchemes[string(buf[:n])]
		case c == '/' || c == '?' || c == '#':
			return true // path/query/fragment before any ':' — relative
		case n == 0:
			if !isSchemeStart(c) {
				return true // not a scheme start — relative reference
			}
			buf[n] = ascii.Lower(c)
			n++
		case isSchemeByte(c):
			if n >= len(buf) {
				return false // implausibly long scheme, not on any allowlist
			}
			buf[n] = ascii.Lower(c)
			n++
		default:
			return true // a non-scheme byte before ':' — relative reference
		}
	}
	return true // no ':' at all — relative reference
}

func isSchemeStart(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func isSchemeByte(c byte) bool {
	return isSchemeStart(c) || '0' <= c && c <= '9' || c == '+' || c == '-' || c == '.'
}

func isURLSafeASCII(c byte) bool {
	if '0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '(', ')', '*', '+', ',', '-', '.', '/',
		':', ';', '=', '?', '@', '^', '_', '~':
		return true
	default:
		return false
	}
}

// firstWord returns the first word of an info string (the code fence language).
func firstWord(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return s[:i]
		}
	}
	return s
}
