package mdflow

import (
	"strings"

	"github.com/Wenrh2004/mdflow/parser"
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
// cancellation is simply the caller deciding to stop feeding and call Close (or
// abandon the Stream). The context-aware terminal operations on Parser exist
// for the whole-document paths, where the loop is the library's rather than the
// caller's.

// Stream is an incremental parse session. Feed it chunks of markdown and it
// returns the HTML that has become final; Provisional shows a tentative view of
// the not-yet-closed tail.
type Stream struct {
	p       *Parser
	bs      *parser.BlockState
	pending strings.Builder // bytes after the last newline
	out     strings.Builder // scratch for the delta of one Feed
	closed  bool
}

// Stream starts an incremental parse session.
func (p *Parser) Stream() *Stream {
	return &Stream{p: p, bs: parser.NewBlockState(p.cfg.Rules)}
}

// Feed consumes the next chunk and returns the HTML that just became final.
// A chunk may end mid-line, mid-word or mid-fence; the split points do not
// affect the result.
func (s *Stream) Feed(chunk string) string {
	if s.closed || chunk == "" {
		return ""
	}
	s.out.Reset()
	for {
		i := strings.IndexByte(chunk, '\n')
		if i < 0 {
			s.pending.WriteString(chunk)
			break
		}
		s.pending.WriteString(chunk[:i])
		line := strings.TrimSuffix(s.pending.String(), "\r")
		s.pending.Reset()
		s.emit(s.bs.FeedLine(line))
		chunk = chunk[i+1:]
	}
	return s.out.String()
}

// Write implements io.Writer, discarding the delta. Pair it with HTML or a Tap
// middleware when the caller only wants the finished document.
func (s *Stream) Write(b []byte) (int, error) {
	s.Feed(string(b))
	return len(b), nil
}

// Provisional renders the currently open tail — container opening tags, a
// tentative view of the open leaf, then the closing tags. Concatenating the
// deltas so far with Provisional gives a displayable document at any instant,
// which is exactly what a streaming UI needs between chunks.
func (s *Stream) Provisional() string {
	if s.closed {
		return ""
	}
	bs := s.bs
	if pending := s.pending.String(); pending != "" {
		// The trailing partial line is not in the state machine yet. Feed it to
		// a snapshot so the real parse stays untouched.
		bs = s.bs.Clone()
		bs.FeedLine(strings.TrimSuffix(pending, "\r"))
	}
	containers, leaf, tail := bs.OpenState()

	var b strings.Builder
	for _, c := range containers {
		s.p.cfg.Renderer.RenderContainer(&b, c)
	}
	switch {
	case leaf != nil:
		s.p.cfg.Renderer.RenderLeaf(&b, *leaf, s.p.cfg.Rules.ParseInline(*leaf))
	case tail != nil:
		// A multi-event leaf (a table) projects to a run of block events rather
		// than one leaf; render them through the same path a closed block takes.
		for i := range tail {
			s.p.writeEvent(&b, tail[i])
		}
	}
	for i := len(containers) - 1; i >= 0; i-- {
		c := containers[i]
		c.Type = token.CloseBlock
		s.p.cfg.Renderer.RenderContainer(&b, c)
	}
	return b.String()
}

// Close flushes the trailing partial line and closes every open block,
// returning the final HTML delta. It is idempotent.
func (s *Stream) Close() string {
	if s.closed {
		return ""
	}
	s.out.Reset()
	if p := s.pending.String(); p != "" {
		s.pending.Reset()
		s.emit(s.bs.FeedLine(strings.TrimSuffix(p, "\r")))
	}
	s.emit(s.bs.CloseAll())
	s.closed = true
	return s.out.String()
}

// Blocks reports how many block events have been emitted so far — a cheap
// progress signal for a streaming UI.
func (s *Stream) Blocks() int { return s.bs.Total() }

func (s *Stream) emit(events []token.BlockEvent) {
	for i := range events {
		s.p.writeEvent(&s.out, events[i])
	}
}
