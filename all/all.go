// Package all bundles the GFM and Memos extensions with mdflow's complete
// CommonMark profile.
//
// It exists so the common "I want every bundled flavour" case is one import.
// [New] preserves mdflow.New's safe raw-HTML policy while adding all bundled GFM
// and Memos syntax.
package all

import (
	"sync"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/gfm"
	"github.com/Wenrh2004/mdflow/extension/memos"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
)

// All is the explicit capability set used by [New]: safe CommonMark raw HTML,
// GFM (tables, strikethrough, task lists), and Memos (math, hashtags,
// typography, resources).
//
// The order is load-bearing — strikethrough must precede typography's subscript
// on the shared `~` trigger, and the raw-HTML rule must follow the core autolink
// on `<`.
var All = extension.Set{gfm.GFM, memos.Memos, rawhtml.RawHTML}

// New builds a complete CommonMark 0.31.2 parser plus every bundled GFM and
// Memos extension. Raw HTML remains escaped unless rawhtml.WithUnsafeHTML is
// supplied explicitly for trusted input.
func New(opts ...mdflow.Option) *mdflow.Parser {
	if len(opts) == 0 {
		// A Parser is immutable: build the full profile once and share it.
		return shared()
	}
	return build(opts...)
}

var shared = sync.OnceValue(func() *mdflow.Parser { return build() })

func build(opts ...mdflow.Option) *mdflow.Parser {
	// Replace mdflow.New's safe raw-HTML default instead of adding All on top of
	// it: All already contains RawHTML, and registering the same syntax twice
	// would duplicate every `<` probe and block continuation.
	return mdflow.New(append([]mdflow.Option{mdflow.WithOnly(All)}, opts...)...)
}
