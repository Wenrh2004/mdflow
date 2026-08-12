// Package tasklist implements GFM task-list item markers as an mdflow
// extension.
//
// The core contributes only one structural fact: whether a paragraph is the
// first direct child of a list item. This package owns the marker grammar, its
// event identity and its HTML checkbox output.
package tasklist

import (
	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

var markerTag = token.NewAtomicTag("task_list_marker")

// TaskList is the GFM task-list capability.
var TaskList = extension.Capability{
	Name:   "task_list",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers task-list marker recognition independently of a renderer.
func Syntax(p *parser.RuleSet) { p.PrependInlineRule(markerRule{}) }

// IsTaskListMarker reports whether e is a task-list marker atom.
func IsTaskListMarker(e mdflow.Event) bool {
	return e.Node == token.Custom && e.Tag == markerTag
}

// Checked reports whether e is a checked task-list marker. It returns false
// for events that do not belong to this extension.
func Checked(e mdflow.Event) bool {
	return IsTaskListMarker(e) && len(e.Text) == 3 && (e.Text[1] == 'x' || e.Text[1] == 'X')
}

type markerRule struct{}

func (markerRule) Name() string     { return "task_list_marker" }
func (markerRule) Triggers() []byte { return []byte{'['} }
func (markerRule) Match(s *parser.InlineState) bool {
	i, src := s.Pos(), s.Src()
	if i != 0 || s.Context()&token.InlineContextListItemHead == 0 || len(src) < 4 {
		return false
	}
	if src[0] != '[' || src[2] != ']' {
		return false
	}
	if src[1] != 'x' && src[1] != 'X' && !isGFMWhitespace(src[1]) {
		return false
	}
	if !startsWhitespace(src[3:]) {
		return false
	}
	// Keep the exact spelling on the token. A renderer without this
	// capability can then emit the author's marker instead of deleting text.
	s.Emit(token.Inline{Node: token.Custom, Tag: markerTag, Text: src[:3]})
	s.Advance(3)
	return true
}

// startsWhitespace implements GFM's whitespace-character class.
func startsWhitespace(src string) bool {
	return src != "" && isGFMWhitespace(src[0])
}

func isGFMWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	default:
		return false
	}
}

func output(r renderer.Renderer) {
	renderer.RegisterCustom(r, markerTag, func(w renderer.Writer, marker token.Inline) {
		w.WriteString(`<input type="checkbox"`)
		if len(marker.Text) == 3 && (marker.Text[1] == 'x' || marker.Text[1] == 'X') {
			w.WriteString(" checked")
		}
		w.WriteString(" disabled")
		if !renderer.CloseVoidElement(r, w) {
			w.WriteString(" />")
		}
	})
}
