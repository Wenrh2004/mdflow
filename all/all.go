// Package all bundles every mdflow extension and offers a parser preconfigured
// with the full syntax set.
//
// It exists so the common "I want everything" case is one import, while the core
// module stays CommonMark-only and a binary that names a smaller bundle links
// nothing more. [New] reproduces what mdflow.New once defaulted to, byte for
// byte.
package all

import (
	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/gfm"
	"github.com/Wenrh2004/mdflow/extension/memos"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
)

// All enables every bundled capability: GFM (tables, strikethrough), Memos
// (math, hashtags, typography, resources) and raw HTML.
//
// The order is load-bearing — strikethrough must precede typography's subscript
// on the shared `~` trigger, and the raw-HTML rule must follow the core autolink
// on `<` — so it matches the original mdflow default exactly.
var All = extension.Set{gfm.GFM, memos.Memos, rawhtml.RawHTML}

// New builds a parser with the CommonMark core plus every bundled extension. It
// is the full-syntax counterpart to mdflow.New, which is now CommonMark-only.
func New(opts ...mdflow.Option) *mdflow.Parser {
	return mdflow.New(append([]mdflow.Option{mdflow.WithExtensions(All)}, opts...)...)
}
