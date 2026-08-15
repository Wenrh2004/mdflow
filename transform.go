package mdflow

import (
	"iter"
	"strings"

	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/token"
)

// Ready-made predicates and middlewares. They exist so the common edits —
// shifting heading levels, rewriting links, stripping a node type — are one
// call rather than a hand-rolled state machine, and so the span-balancing
// subtleties are solved once.

// ---- predicates ----

// IsHeading reports whether e enters or leaves a heading.
func IsHeading(e Event) bool { return e.Node == token.Heading }

// IsCodeBlock reports whether e enters or leaves a fenced code block.
func IsCodeBlock(e Event) bool { return e.Node == token.CodeBlock }

// IsImage reports whether e enters or leaves an image.
func IsImage(e Event) bool { return e.Node == token.Image }

// IsLink reports whether e enters or leaves a link.
func IsLink(e Event) bool { return e.Node == token.Link }

// The predicates below match syntax that an extension introduces, so they match
// on a tag rather than a node kind. Each extension module exports its own —
// table.IsTable, math.IsMath — because the core vocabulary names none of them.
// [Tagged] and [TaggedLeaf] build the same thing for any tag, including one your
// own extension defines.

// Tagged returns a predicate matching a custom inline node by tag.
func Tagged(tag token.Tag) func(Event) bool {
	return func(e Event) bool { return e.Node == token.Custom && e.Tag == tag }
}

// TaggedLeaf returns a predicate matching a custom leaf block by tag.
func TaggedLeaf(tag token.Tag) func(Event) bool {
	return func(e Event) bool { return e.Node == token.CustomLeaf && e.Tag == tag }
}

// AtLevel returns a predicate matching headings at the given level.
func AtLevel(level int) func(Event) bool {
	return func(e Event) bool { return e.Node == token.Heading && e.Level == level }
}

// ---- middlewares ----

// ShiftHeadings moves every heading by delta levels, clamped to 1..6. Useful
// when a document is embedded under an existing <h1>.
func ShiftHeadings(delta int) Middleware {
	return func(seq iter.Seq[Event]) iter.Seq[Event] {
		return iterx.Map(seq, func(e Event) Event {
			if e.Node == token.Heading && e.Type == EnterEvent {
				e.Level = min(max(e.Level+delta, 1), 6)
			}
			return e
		})
	}
}

// RewriteLinks applies f to every link and image destination. Returning "" for
// a link leaves the destination untouched.
func RewriteLinks(f func(dest string) string) Middleware {
	return func(seq iter.Seq[Event]) iter.Seq[Event] {
		return iterx.Map(seq, func(e Event) Event {
			if e.Type == EnterEvent && (e.Node == token.Link || e.Node == token.Image) {
				if d := f(e.Dest); d != "" {
					e.Dest = d
				}
			}
			return e
		})
	}
}

// MapText applies f to every literal text run, leaving inline code alone.
func MapText(f func(string) string) Middleware {
	return func(seq iter.Seq[Event]) iter.Seq[Event] {
		return iterx.Map(seq, func(e Event) Event {
			if e.Type == TextEvent {
				e.Text = f(e.Text)
			}
			return e
		})
	}
}

// Drop removes whole nodes — the EnterEvent, the LeaveEvent and everything between —
// so the output stays balanced. Filtering on the same predicate would emit one
// half of the pair and corrupt the markup.
func Drop(pred func(Event) bool) Middleware {
	return func(seq iter.Seq[Event]) iter.Seq[Event] {
		return func(yield func(Event) bool) {
			depth := 0
			for e := range seq {
				if depth > 0 {
					// Once a span is selected, structural balance—not another
					// predicate call—identifies its matching leave. Predicates
					// may depend on metadata carried only by the opening event.
					if e.Type == EnterEvent && !e.IsAtomic() {
						depth++
					} else if e.Type == LeaveEvent {
						depth--
					}
					continue
				}
				if pred(e) && e.Type == EnterEvent {
					if !e.IsAtomic() {
						depth = 1 // skip until the matching LeaveEvent
					}
					continue // an atomic node is the whole span
				}
				if !yield(e) {
					return
				}
			}
		}
	}
}

// Unwrap removes a node's EnterEvent/LeaveEvent markers but keeps its content — e.g.
// stripping every link while leaving the anchor text in place.
func Unwrap(pred func(Event) bool) Middleware {
	return func(seq iter.Seq[Event]) iter.Seq[Event] {
		return func(yield func(Event) bool) {
			selected := make([]bool, 0, 8)
			for e := range seq {
				switch e.Type {
				case EnterEvent:
					unwrap := pred(e)
					if !e.IsAtomic() {
						selected = append(selected, unwrap)
					}
					if unwrap {
						continue
					}
				case LeaveEvent:
					unwrap := false
					if n := len(selected); n > 0 {
						unwrap = selected[n-1]
						selected = selected[:n-1]
					}
					if unwrap {
						continue
					}
				}
				if !yield(e) {
					return
				}
			}
		}
	}
}

// Slugify turns heading text into a URL fragment, matching the widely used
// GitHub rules: lowercase, spaces to hyphens, drop everything that is neither
// alphanumeric, hyphen nor underscore. Non-ASCII letters are kept, since
// dropping them would collapse every CJK heading to the empty string.
func Slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	dash := false // collapse runs of separators into a single hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127:
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
