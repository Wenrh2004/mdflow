package token

import (
	"strconv"
	"sync"
	"sync/atomic"
)

// Tag identifies the node an extension introduces. It is the *identity* of the
// node, never its markup: it names what the author wrote, and each renderer
// decides what that becomes. Strikethrough renders as <del> in HTML, but a
// terminal renderer is free to strike the run and a JSON renderer to emit a type
// field — none of which a tag spelled "del" would leave room for.
//
// A Tag is a small integer, not a string. Every node past CommonMark carries one
// and every renderer dispatches on one, so it sits on the hottest paths in the
// library twice over: as a field on [Inline], [Leaf] and [BlockEvent], and as
// the key a renderer looks its handler up by. As an integer it costs two bytes
// in those structs — which fit in padding they already had — instead of a
// sixteen-byte string header, and dispatch is a slice index instead of a string
// hash.
//
// The core vocabulary names no extension. A capability allocates its own tag
// with [NewTag] (or [NewAtomicTag]) at configuration time, and the parser rule
// and the renderer registration that make up that capability each ask for it by
// name — idempotently, so they agree without a shared constant. The zero value
// means "no tag" and belongs to every non-custom node.
//
// Because the name is the identity, two unrelated extensions that pick the same
// name share a tag. Qualify names with the defining package's import path, as
// encoding/gob does for registered types:
//
//	var cellTag = token.NewTag("example.com/mdext/table.cell")
//
// Every bundled capability does.
type Tag uint16

// The one reserved tag. Everything past it is allocated by [NewTag]; the core
// defines no others, so an extension lives in a module of its own with no tag
// constant to import.
const (
	NoTag Tag = iota

	// firstUserTag is where [NewTag] starts allocating.
	firstUserTag
)

// IsAtomic reports whether the tag names a self-contained inline node, i.e. one
// emitted as a single token rather than an open/close pair.
//
// A tag from [NewTag] is paired by default; use [NewAtomicTag] for one that is
// not.
//
// This is on the rendering hot path, once per custom node, so it must not lock.
// It reads a dense []bool snapshot published through an atomic pointer: a bounds
// check and a load. The snapshot is rebuilt lazily — under the registry lock, off
// the hot path — the first time a tag is queried after one is added, which only
// happens during configuration.
func (t Tag) IsAtomic() bool {
	if snap := registry.snap.Load(); snap != nil {
		if s := *snap; int(t) < len(s) {
			return s[t]
		}
	}
	return atomicSlow(t)
}

// atomicSlow publishes a fresh atomicity snapshot and answers from it. It runs
// only when the snapshot is missing or too short — i.e. once after each tag is
// created — never in steady state.
func atomicSlow(t Tag) bool {
	registry.mu.RLock()
	// Copy rather than publish the live slice: a later NewTag may append in
	// place, and a reader holding the published pointer must never see that
	// mutation. The copy is immutable once stored.
	snap := make([]bool, len(registry.atomic))
	copy(snap, registry.atomic)
	registry.mu.RUnlock()

	registry.snap.Store(&snap)
	return int(t) < len(snap) && snap[t]
}

// String returns the tag's name, which is what a caller sees in a log line or a
// %v of an event. It is not on any rendering path, so it consults the registry
// directly.
func (t Tag) String() string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if int(t) < len(registry.names) && registry.names[t] != "" {
		return registry.names[t]
	}
	return "unknown"
}

// registry holds the tags callers define. It is consulted when a tag is created
// and when one is printed — never while rendering, which dispatches on the
// integer alone and reads atomicity from the snapshot [IsAtomic] publishes.
//
// names and atomic are dense, indexed by the tag itself; the two reserved
// indices (NoTag, firstUserTag) are seeded so the first allocation lands on
// firstUserTag.
var registry = struct {
	mu     sync.RWMutex
	byName map[string]Tag
	names  []string // index by Tag; "" for a slot no tag holds
	atomic []bool   // index by Tag; the source of truth IsAtomic snapshots
	snap   atomic.Pointer[[]bool]
}{
	byName: make(map[string]Tag),
	names:  make([]string, firstUserTag),
	atomic: make([]bool, firstUserTag),
}

// NewTag returns the tag for name, allocating one the first time it is seen.
//
// It is idempotent: the same name always yields the same Tag, so an extension
// and a renderer can each ask for their tag independently and agree. Call it
// during initialisation or configuration — never per document.
//
// NewTag panics if name was already registered by [NewAtomicTag]: the two
// halves of a capability disagreeing about a node's shape is a programming
// error that would otherwise render one node as two. It also panics once all
// 65535 tags are taken, which no realistic set of extensions reaches and a loop
// calling it with fresh names eventually does.
func NewTag(name string) Tag { return newTag(name, false) }

// NewAtomicTag is [NewTag] for a node emitted as a single token with no closing
// counterpart, the way a hashtag or an inline math span is.
func NewAtomicTag(name string) Tag { return newTag(name, true) }

func newTag(name string, isAtomic bool) Tag {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if t, ok := registry.byName[name]; ok {
		if registry.atomic[t] != isAtomic {
			panic("mdflow/token: tag " + strconv.Quote(name) + " registered as both atomic and paired")
		}
		return t
	}
	if len(registry.names) >= 1<<16 {
		panic("mdflow/token: tag space exhausted (65535 tags)")
	}
	t := Tag(len(registry.names))
	registry.byName[name] = t
	registry.names = append(registry.names, name)
	registry.atomic = append(registry.atomic, isAtomic)
	registry.snap.Store(nil) // invalidate; IsAtomic rebuilds on next query
	return t
}
