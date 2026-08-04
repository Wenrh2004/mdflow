// Package mdflow is a streaming Markdown parser with a chainable,
// functional API.
//
// # Layers
//
// mdflow itself is a facade. It owns no syntax and no markup — it wires the
// layers below together and adds the chaining and functional surface:
//
//	token           node kinds, inline tokens, leaves, block events; no dependencies
//	parser          the state machines, the rules, and the extension seam
//	renderer        the Renderer interface and its optional capabilities
//	renderer/html   the HTML implementation
//	extension       the capability seam: syntax paired with the output it produces
//	extension/*      each capability in its own module (table, math, ...)
//	mdflow          facade: composition, chaining, streaming, fan-out
//
// Dependencies point strictly downward, and none of them is a type assertion in
// disguise: an extension configures output through the capability interfaces in
// package renderer, never by reaching for the concrete HTML renderer. A new
// output format is a new Renderer; new syntax is a new capability. Neither
// touches the other, and the core imports no extension at all — a binary links
// exactly the capabilities it names.
//
// Use [New] for the CommonMark subset, [NewBuilder] to add capabilities, or the
// umbrella `all` module for the full syntax set:
//
//	import "github.com/Wenrh2004/mdflow/all"
//	html := all.New().HTML(src)
//
// # Three ideas
//
// No syntax tree. Parsing emits a flat stream of Enter/Leave/Text/Code events
// (the pulldown-cmark model). Nothing allocates a node per construct, nothing
// walks a tree twice, and a consumer can stop halfway through a document. That
// is why the vocabulary package is called token and not ast.
//
// Line-driven and incremental. The block state machine never revisits a closed
// block, so feeding a document in chunks costs the same as parsing it whole.
// That is what makes [Parser.Stream] O(n) where re-parsing on every chunk — the
// usual approach in a chat UI — is O(n²).
//
// Rules, not switches. Block and inline syntax live in registries — container,
// leaf and inline rules in package parser — and output lives behind a Renderer,
// so new syntax and new output formats are additions rather than forks.
// Everything past CommonMark (GFM tables, math, hashtags, typography, wiki
// resources) is registered through that seam, on the same footing as syntax you
// add yourself: the core vocabulary names none of it.
//
// # Basic use
//
//	html := mdflow.HTML("# Hello\n\nSome *markdown*.\n")
//
// For repeated parsing, build a [Parser] once and share it. It is immutable and
// safe for concurrent use; its scratch state is pooled.
//
//	var md = mdflow.New()
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		md.Render(w, source) // streams; no document-sized string
//	}
//
// # Chaining
//
// Every chaining method returns a new Parser, so derived parsers never disturb
// the one they came from:
//
//	docs := mdflow.New().
//		Transform(mdflow.ShiftHeadings(1)).
//		Transform(mdflow.RewriteLinks(absolutize)).
//		Transform(mdflow.Drop(mdflow.IsImage))
//
//	html := docs.HTML(src)
//
// A Parser with no middleware installed takes a fast path that bypasses the
// event stream entirely, so the functional layer costs nothing when unused.
//
// # Functional style
//
// The event stream is an [iter.Seq], so it composes with ordinary Go. The
// generic combinators (iterx.Map, iterx.Filter, iterx.Reduce, iterx.Collect,
// iterx.Take, ...) live in package [github.com/Wenrh2004/mdflow/iterx] as free
// functions, because Go does not permit type parameters on methods.
//
//	words := iterx.Reduce(mdflow.Events(src), 0, func(n int, e mdflow.Event) int {
//		if e.Type == mdflow.TextEvent {
//			return n + len(strings.Fields(e.Text))
//		}
//		return n
//	})
//
// Extension syntax is matched by tag rather than by node kind, since the core
// vocabulary does not enumerate it. Each extension module exports its own
// predicate — table.IsTable, math.IsMath — while [Tagged] and [TaggedLeaf] build
// one for any tag, including one your own capability defines.
//
// # Parallelism
//
// Block structure is inherently sequential, but inline parsing of a closed leaf
// depends on nothing outside that leaf, so phase two distributes.
// [Parser.Workers] fans it across cores — 1.45x at 175 KiB, 2.0x at 2 MiB, for
// +5-23% memory. It is opt-in, and falls back to sequential below 16 KiB or
// when a middleware chain is installed. See [Parser.Workers] for the measured
// trade-off.
//
// # Supported syntax
//
// The core is a CommonMark subset: ATX and setext headings, paragraphs, fenced
// code, blockquotes, ordered/unordered lists, task lists, thematic breaks,
// emphasis, strong, inline code, links, images, autolinks, hard breaks and
// backslash escapes.
//
// The common GFM and Memos-flavoured extensions — tables, strikethrough, math
// blocks, embeds, inline math, hashtags, highlight, subscript, superscript,
// spoilers, references and inline raw HTML — each live in a module under
// extension/, and the `all` module bundles the full set. Raw HTML is recognised
// but escaped on output unless the rawhtml extension's WithUnsafeHTML is set.
//
// Deliberately absent from the core: indented code blocks, reference links, raw
// HTML blocks and footnotes. See README.md for the reasoning and the full
// compliance notes.
//
// One known edge: a hard break spelled as two trailing spaces is normalised at
// the line-based block layer before inline structure is known, so when such a
// line falls inside a code span that spans a newline, a literal backslash
// surfaces in the span instead of the space CommonMark specifies. It is a corner
// of the subset, not content invention, and is guarded by FuzzText.
package mdflow
