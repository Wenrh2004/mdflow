package rawhtml

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// FilteredHTML is GFM's "disallowed raw HTML" output: raw HTML is emitted
// verbatim except that the opening '<' of the tags that change how a browser
// parses the rest of the page — script, style, iframe, textarea, title, xmp,
// noembed, noframes and plaintext — is escaped. Like UnsafeHTML it has no
// syntax half.
var FilteredHTML = extension.Capability{
	Name:   "raw_html_filtered",
	Output: filteredOutput,
}

func filteredOutput(r renderer.Renderer) {
	renderer.RegisterCustom(r, InlineTag, func(w renderer.Writer, tok token.Inline) {
		writeFiltered(w, tok.Text)
	})
	renderer.RegisterCustomLeaf(r, BlockTag,
		func(w renderer.Writer, leaf token.Leaf, _ []token.Inline, _ renderer.Renderer) {
			writeFiltered(w, leaf.Content)
		})
}

var disallowedTags = [...]string{
	"title", "textarea", "style", "xmp", "iframe",
	"noembed", "noframes", "script", "plaintext",
}

// writeFiltered writes s verbatim, replacing '<' with "&lt;" wherever it opens
// or closes a disallowed tag.
func writeFiltered(w renderer.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '<' || !disallowedAt(s, i+1) {
			continue
		}
		w.WriteString(s[start:i])
		w.WriteString("&lt;")
		start = i + 1
	}
	w.WriteString(s[start:])
}

// disallowedAt reports whether s[i:] begins an optional '/', a disallowed tag
// name in any case, and then a byte that ends a tag name.
func disallowedAt(s string, i int) bool {
	if i < len(s) && s[i] == '/' {
		i++
	}
	for _, name := range disallowedTags {
		end := i + len(name)
		if end > len(s) || !equalFoldASCII(s[i:end], name) {
			continue
		}
		if end == len(s) {
			return true
		}
		switch s[end] {
		case ' ', '\t', '\n', '\r', '\f', '>', '/':
			return true
		}
	}
	return false
}
