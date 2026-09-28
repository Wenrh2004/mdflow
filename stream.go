package mdflow

import (
	"strings"

	"github.com/Wenrh2004/mdflow/token"
)

// Incremental parsing for streaming producers (an LLM emitting tokens, a file
// being tailed, a websocket).
//
// The naive approach a chat UI reaches for first — re-parse the whole document
// on every chunk — is O(n²) in document length. The block state machine never
// revisits a closed block, so feeding it line by line is O(n) overall: each
// chunk costs only the lines it completes.
//
// A Stream is *not* safe for concurrent use; it owns mutable parse state. Use
// one per document. The Parser it came from stays shareable.
//
// A Stream needs no context: it is driven one Feed at a time by the caller, so
// cancellation is simply the caller deciding to stop feeding and call Finish (or
// abandon the Stream). The context-aware terminal operations on Parser exist
// for the whole-document paths, where the loop is the library's rather than the
// caller's.

// Stream is an incremental parse session. Feed it chunks of markdown and it
// returns the HTML that has become final; Provisional shows a tentative view of
// the not-yet-closed tail.
type Stream struct {
	p       *Parser
	driver  *documentDriver
	pending strings.Builder // bytes after the last logical line ending
	out     strings.Builder // scratch for the delta of one Feed
	afterCR bool            // a trailing CR was emitted; swallow a following LF
	blocks  int             // final count retained after the driver is released
	closed  bool

	sealUndefinedAfter int // streaming seal-undefined policy; 0 keeps strict

	// provCache memoises the rendered bytes of each event Provisional emits,
	// keyed by emit position and a structural signature. While a list holds its
	// events open, every Provisional would otherwise re-render the whole withheld
	// tail; the cache lets an unchanged prefix be reused so only the growing tail
	// is re-rendered. provRefs is the reference-definition fingerprint the cache
	// was built under: a new or changed definition (including one still being
	// typed in the partial line) can change how an earlier reference renders, so
	// a change there rebuilds the cache.
	provCache   []provCacheEntry
	provRefs    uint64
	provEpoch   int // commitEpoch the cache positions are relative to
	commitEpoch int // bumped by every committed event, so the cache drops when the base shifts
}

// provCacheEntry is one emitted event's rendered bytes and the signature they
// were rendered under. The signature captures every field the renderer reads
// except the content bytes themselves, which are immutable at a given emit
// position (the held buffer is append-only), so a matching signature at the
// same position guarantees identical output.
type provCacheEntry struct {
	sig   provSignature
	bytes string
}

// provSignature is the comparable render-affecting projection of a block event.
// Content and Info enter only as lengths: a held event's bytes never change at a
// fixed position, and the one growing event (the open leaf) changes length as it
// grows, so length alone distinguishes it.
type provSignature struct {
	typ        token.BlockOp
	container  token.Node
	tag        token.Tag
	start      int
	blockAlign token.Align
	ordered    bool
	blockTight bool
	newline    bool
	leafNode   token.Node
	leafTag    token.Tag
	level      int
	leafAlign  token.Align
	context    token.InlineContext
	contentLen int
	infoLen    int
	literal    bool
	leafTight  bool
	breakAfter bool
	header     bool
}

func provSignatureOf(ev token.BlockEvent) provSignature {
	return provSignature{
		typ:        ev.Type,
		container:  ev.Container,
		tag:        ev.Tag,
		start:      ev.Start,
		blockAlign: ev.Align,
		ordered:    ev.Ordered,
		blockTight: ev.Tight,
		newline:    ev.Newline,
		leafNode:   ev.Leaf.Node,
		leafTag:    ev.Leaf.Tag,
		level:      ev.Leaf.Level,
		leafAlign:  ev.Leaf.Align,
		context:    ev.Leaf.Context,
		contentLen: len(ev.Leaf.Content),
		infoLen:    len(ev.Leaf.Info),
		literal:    ev.Leaf.Literal,
		leafTight:  ev.Leaf.Tight,
		breakAfter: ev.Leaf.BreakAfter,
		header:     ev.Leaf.Header,
	}
}

// StreamOption configures an incremental parse session.
type StreamOption func(*Stream)

// SealUndefinedReferencesAfter trades strict CommonMark forward-reference
// resolution for streaming liveness: once n block events have queued behind a
// shortcut reference with no definition yet, the reference commits as literal
// text instead of withholding all output until Finish. n must be >= 1; without
// this option a stream stays strict and withholds until Finish.
//
// This is a correctness-for-liveness trade, not merely an earlier commit. If a
// definition arrives *after* the threshold has fired — a citation defined in a
// references section at the end of the document — strict mode (and HTML) would
// have resolved the reference to a link, but this policy has already committed
// it as literal text and cannot revise it. Use it when incremental delivery
// matters more than late-binding references, and pair it with [Stream.Blocked]
// if the UI needs to distinguish committed output from still-resolving text.
func SealUndefinedReferencesAfter(n int) StreamOption {
	return func(s *Stream) {
		if n >= 1 {
			s.sealUndefinedAfter = n
		}
	}
}

// Stream starts an incremental parse session.
func (p *Parser) Stream(opts ...StreamOption) *Stream {
	// The block state comes from the parser's pool, as it does for a whole-
	// document render: a chat server opening thousands of short streams
	// otherwise builds a fresh state machine (and its buffers) for each one.
	// Finish returns it; a stream abandoned without Finish is simply collected.
	s := &Stream{p: p, driver: newDocumentDriver(p.borrow())}
	for _, opt := range opts {
		opt(s)
	}
	s.driver.sealUndefinedAfter = s.sealUndefinedAfter
	return s
}

// Feed consumes the next chunk and returns the HTML that just became final.
// A chunk may end mid-line, mid-word or mid-fence; the split points do not
// affect the result.
func (s *Stream) Feed(chunk string) string {
	if s.closed || chunk == "" {
		return ""
	}
	s.out.Reset()
	if s.afterCR {
		if chunk[0] == '\n' {
			chunk = chunk[1:]
		}
		s.afterCR = false
	}
	for len(chunk) > 0 {
		i := strings.IndexAny(chunk, "\r\n")
		if i < 0 {
			s.pending.WriteString(chunk)
			break
		}
		// A line wholly inside this chunk is a substring of it — strings are
		// immutable, so it can be fed without copying. Only a line split across
		// chunks is assembled in pending.
		line := chunk[:i]
		if s.pending.Len() > 0 {
			s.pending.WriteString(line)
			line = s.pending.String()
			s.pending.Reset()
		}
		s.driver.FeedLine(line, s.emit)

		ending := chunk[i]
		chunk = chunk[i+1:]
		if ending != '\r' {
			continue
		}
		if len(chunk) > 0 && chunk[0] == '\n' {
			chunk = chunk[1:]
		} else if len(chunk) == 0 {
			s.afterCR = true
		}
	}
	result := s.out.String()
	s.out.Reset()
	return result
}

// Provisional renders only the not-yet-committed tail. It finalises a snapshot
// as if the current prefix ended now, which resolves open-list tightness and
// every other EOF-dependent decision without changing the live parse. Events
// already returned by Feed are absent from the snapshot's buffers, so they are
// never repeated.
//
// While a list holds its events open, the tail Provisional renders grows with
// the list, so repeated calls reuse a per-event render cache for the unchanged
// prefix and re-render only the growing tail. Correctness is guarded two ways: a
// structural signature per event catches any change in a held event's rendering
// (tightness flips, line-break hints), and a reference-definition version catches
// a late definition that changes how an earlier reference renders — a mismatch
// rebuilds the cache from scratch. (Cloning, inline parsing and assembling the
// full tail string remain proportional to the withheld tail; only re-rendering
// is memoised.)
func (s *Stream) Provisional() string {
	if s.closed {
		return ""
	}
	// Cache positions are relative to the committed prefix. Any committed output
	// since the cache was built — a held list closing, or a driver-level suffix
	// draining when a blocked reference resolves — shifts that base and would make
	// old positions collide, so drop the cache. commitEpoch counts every committed
	// event, which BlockState.Total alone misses (a definition-only paragraph and
	// a suffix drain commit output without emitting a new block event).
	if s.commitEpoch != s.provEpoch {
		s.provCache = s.provCache[:0]
		s.provEpoch = s.commitEpoch
	}
	// Only the held events have immutable content and may be cached by position;
	// everything a snapshot emits beyond them is the volatile open tail.
	cacheLimit := s.driver.blocks.HeldCount()
	hadCache := len(s.provCache) > 0
	out, refs := s.renderProvisional(cacheLimit)
	if hadCache && refs != s.provRefs {
		// A definition seen this snapshot may have changed an earlier reference,
		// so the reused prefix could be stale. Rebuild once from an empty cache.
		s.provCache = s.provCache[:0]
		out, refs = s.renderProvisional(cacheLimit)
	}
	s.provRefs = refs
	return out
}

// renderProvisional renders the not-yet-committed tail into a fresh string,
// reusing s.provCache for the first cacheLimit events (the immutable held
// prefix) whose signature is unchanged and refreshing the rest. It returns the
// rendered tail and the snapshot's reference fingerprint.
func (s *Stream) renderProvisional(cacheLimit int) (string, uint64) {
	driver := s.driver.clone()
	defer driver.Release()
	var b strings.Builder
	idx := 0
	render := func(ev token.BlockEvent, inlines []token.Inline) bool {
		if idx >= cacheLimit {
			// Volatile open tail: render fresh, never cached. Drop any stale
			// entries at or past this position on first crossing.
			if idx < len(s.provCache) {
				s.provCache = s.provCache[:idx]
			}
			s.p.writeDocumentEvent(&b, ev, inlines)
			idx++
			return true
		}
		sig := provSignatureOf(ev)
		if idx < len(s.provCache) && s.provCache[idx].sig == sig {
			b.WriteString(s.provCache[idx].bytes)
			idx++
			return true
		}
		var one strings.Builder
		s.p.writeDocumentEvent(&one, ev, inlines)
		bytes := one.String()
		b.WriteString(bytes)
		// A mismatch invalidates this position and everything after it: the
		// emitted order is append-stable, so a divergence never predates idx.
		s.provCache = append(s.provCache[:idx], provCacheEntry{sig: sig, bytes: bytes})
		idx++
		return true
	}
	if pending := s.pending.String(); pending != "" {
		driver.FeedLine(pending, render)
	}
	driver.Close(render)
	if idx < len(s.provCache) {
		s.provCache = s.provCache[:idx]
	}
	return b.String(), driver.blocks.ReferenceFingerprint()
}

// Finish flushes the trailing partial line and closes every open block,
// returning the final HTML delta. It is idempotent: later calls return "".
//
// It is not named Close because it does not have io.Closer's shape — it
// returns the last of the output rather than an error. [Parser.NewWriter] is
// the io.WriteCloser spelling of a Stream.
func (s *Stream) Finish() string {
	if s.closed {
		return ""
	}
	s.out.Reset()
	if p := s.pending.String(); p != "" {
		s.pending.Reset()
		s.driver.FeedLine(p, s.emit)
	}
	s.driver.Close(s.emit)
	s.blocks = s.driver.blocks.Total()
	s.driver.Release()
	s.p.release(s.driver.blocks)
	s.driver = nil
	s.closed = true
	s.afterCR = false
	result := s.out.String()
	s.pending.Reset()
	s.out.Reset()
	s.provCache = nil
	s.provRefs = 0
	s.provEpoch = 0
	s.commitEpoch = 0
	return result
}

// Blocked reports whether the committed stream is currently withholding output
// behind an unresolved shortcut reference, and the normalised label it waits
// on. It lets a streaming UI decide whether to keep waiting, Finish early, or
// show a spinner; it is false once the reference resolves or the stream is
// finished.
func (s *Stream) Blocked() (label string, ok bool) {
	if s.driver == nil {
		return "", false
	}
	return s.driver.Blocked()
}

// BlockCount reports how many block events have been emitted so far — a cheap
// progress signal for a streaming UI.
func (s *Stream) BlockCount() int {
	if s.driver == nil {
		return s.blocks
	}
	return s.driver.blocks.Total()
}

func (s *Stream) emit(ev token.BlockEvent, inlines []token.Inline) bool {
	s.p.writeDocumentEvent(&s.out, ev, inlines)
	s.commitEpoch++ // the committed base moved; Provisional's cache must not span it
	return true
}
