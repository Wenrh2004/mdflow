package mdflow

import (
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// documentEmitter consumes one block event whose inline phase is complete.
// The callback is synchronous: inlines only has to remain valid for the call.
type documentEmitter func(token.BlockEvent, []token.Inline) bool

// documentDriver joins the sequential block phase to the resumable inline
// phase. Ordinary documents never enter suffix: each block event goes straight
// from BlockState to the emitter. Once an inline cursor pauses, the driver holds
// that event and the raw suffix behind it until the earliest cursor can resume.
//
// This is deliberately a queue rather than a document buffer. Definitions keep
// arriving through BlockState one line at a time, and every resolved prefix is
// released immediately. Each queued event is started once, cleared when
// consumed, and moved only by amortised compaction.
type documentDriver struct {
	blocks *parser.BlockState

	suffix     []token.BlockEvent
	suffixHead int
	cursor     *parser.InlineCursor
	closed     bool
	released   bool

	// sealUndefinedAfter, when > 0, is the streaming seal-undefined policy: once
	// this many block events queue behind a cursor blocked on an undefined
	// shortcut reference, the reference commits as literal text rather than
	// withholding all output until Close. Zero keeps strict CommonMark behaviour.
	sealUndefinedAfter int
}

func newDocumentDriver(blocks *parser.BlockState) *documentDriver {
	return &documentDriver{blocks: blocks}
}

// FeedLine advances the block machine, then tries the earliest unresolved
// suffix even when the block batch is empty. A definition-only paragraph emits
// no block event, but closing it may still unlock that suffix.
func (d *documentDriver) FeedLine(line string, emit documentEmitter) bool {
	if d.closed || d.released {
		return false
	}
	events := d.blocks.FeedLine(line)
	var keepGoing bool
	if d.hasSuffix() {
		d.queue(events)
		keepGoing = d.drain(emit)
	} else {
		keepGoing = d.consumeFresh(events, emit)
	}
	if keepGoing {
		keepGoing = d.applySealPolicy(emit)
	}
	d.blocks.ReleaseEvents()
	return keepGoing
}

// Blocked reports whether the driver is currently withholding output behind an
// unresolved shortcut reference, and the normalized label it waits on.
func (d *documentDriver) Blocked() (string, bool) {
	if d.cursor == nil {
		return "", false
	}
	return d.cursor.PendingLabel()
}

// applySealPolicy enforces the streaming seal-undefined policy: while a cursor
// is blocked on an undefined reference and at least sealUndefinedAfter block
// events have queued behind it, resolve that leaf to literal text and drain what
// follows. Off (and a no-op) unless the policy is configured.
func (d *documentDriver) applySealPolicy(emit documentEmitter) bool {
	if d.sealUndefinedAfter <= 0 {
		return true
	}
	for d.cursor != nil && d.queuedBehindHead() >= d.sealUndefinedAfter {
		if !d.forceHeadLiteral(emit) {
			return false
		}
		if !d.drain(emit) {
			return false
		}
	}
	return true
}

// queuedBehindHead counts the block events queued behind the blocked head leaf.
func (d *documentDriver) queuedBehindHead() int {
	return len(d.suffix) - d.suffixHead - 1
}

// forceHeadLiteral resolves the blocked head leaf with undefined references
// turned to literal text, emits it, and clears the cursor. The shared resolver
// is left untouched, so forward definitions elsewhere still resolve.
func (d *documentDriver) forceHeadLiteral(emit documentEmitter) bool {
	tokens, _ := d.cursor.ForceLiteral()
	d.cursor.Release()
	d.cursor = nil
	event := d.suffix[d.suffixHead]
	keepGoing := emit(event, tokens)
	d.popSuffix()
	return keepGoing
}

// Close closes block structure first so the final paragraph can register its
// definitions, seals the resolver, and only then drains inline work. Sealing is
// what turns every still-missing reference into final literal text.
func (d *documentDriver) Close(emit documentEmitter) bool {
	if d.closed || d.released {
		return true
	}
	events := d.blocks.CloseAll()
	d.blocks.SealReferences()
	d.closed = true
	if d.hasSuffix() {
		d.queue(events)
		keepGoing := d.drain(emit)
		d.blocks.ReleaseEvents()
		return keepGoing
	}
	keepGoing := d.consumeFresh(events, emit)
	d.blocks.ReleaseEvents()
	return keepGoing
}

// clone snapshots both phases for Stream.Provisional. The live cursor must be
// rebound to the cloned BlockState so speculative sealing cannot affect the
// live resolver.
func (d *documentDriver) clone() *documentDriver {
	blocks := d.blocks.Clone()
	// sealUndefinedAfter is intentionally not copied: the clone exists for a
	// Provisional snapshot, whose Close seals the resolver unconditionally, so an
	// undefined reference already resolves to literal text without the policy.
	out := &documentDriver{blocks: blocks, closed: d.closed}
	if !d.hasSuffix() {
		return out
	}
	out.suffix = append([]token.BlockEvent(nil), d.suffix[d.suffixHead:]...)
	if d.cursor != nil {
		out.cursor = d.cursor.CloneFor(blocks)
	}
	return out
}

// Release abandons a tentative suffix. It is idempotent so every early-break,
// cancellation and normal-close path can defer it without coordinating.
func (d *documentDriver) Release() {
	if d == nil || d.released {
		return
	}
	if d.cursor != nil {
		d.cursor.Release()
		d.cursor = nil
	}
	for i := d.suffixHead; i < len(d.suffix); i++ {
		d.suffix[i] = token.BlockEvent{}
	}
	d.suffix = d.suffix[:0]
	d.suffixHead = 0
	d.released = true
}

func (d *documentDriver) hasSuffix() bool { return d.suffixHead < len(d.suffix) }

// consumeFresh preserves the no-reference hot path: completed events call the
// emitter directly and never touch the suffix queue.
func (d *documentDriver) consumeFresh(events []token.BlockEvent, emit documentEmitter) bool {
	for i := range events {
		tokens, cursor := d.start(events[i])
		if cursor != nil {
			d.cursor = cursor
			d.suffix = append(d.suffix, events[i])
			d.queue(events[i+1:])
			return true
		}
		if !emit(events[i], tokens) {
			return false
		}
	}
	return true
}

func (d *documentDriver) queue(events []token.BlockEvent) {
	d.suffix = append(d.suffix, events...)
}

func (d *documentDriver) drain(emit documentEmitter) bool {
	for d.hasSuffix() {
		event := d.suffix[d.suffixHead]
		var tokens []token.Inline
		if d.cursor == nil {
			var cursor *parser.InlineCursor
			tokens, cursor = d.start(event)
			if cursor != nil {
				d.cursor = cursor
				return true
			}
		} else {
			var complete bool
			tokens, complete = d.cursor.Resume()
			if !complete {
				return true
			}
		}

		keepGoing := emit(event, tokens)
		if d.cursor != nil {
			d.cursor.Release()
			d.cursor = nil
		}
		d.popSuffix()
		if !keepGoing {
			return false
		}
	}
	return true
}

func (d *documentDriver) start(event token.BlockEvent) ([]token.Inline, *parser.InlineCursor) {
	if event.Type != token.LeafBlock {
		return nil, nil
	}
	return d.blocks.StartInline(event.Leaf)
}

func (d *documentDriver) popSuffix() {
	d.suffix[d.suffixHead] = token.BlockEvent{}
	d.suffixHead++
	if d.suffixHead == len(d.suffix) {
		d.suffix = d.suffix[:0]
		d.suffixHead = 0
		return
	}
	// Moving only after at least half the buffer became dead makes compaction
	// amortised linear; the small floor avoids copying short reference bursts.
	if d.suffixHead < 64 || d.suffixHead*2 < len(d.suffix) {
		return
	}
	remaining := copy(d.suffix, d.suffix[d.suffixHead:])
	clear(d.suffix[remaining:])
	d.suffix = d.suffix[:remaining]
	d.suffixHead = 0
}
