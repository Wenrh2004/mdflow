package mdflow_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/text"
	"github.com/Wenrh2004/mdflow/token"
)

func TestSoftBreakIsAnAtomicEvent(t *testing.T) {
	const src = "a\nb\n"
	p := mdflow.New()

	gotEvents := slices.Collect(p.Events(src))
	wantEvents := []mdflow.Event{
		{Type: mdflow.EnterEvent, Node: token.Paragraph, Content: "a\nb"},
		{Type: mdflow.TextEvent, Node: token.Text, Text: "a"},
		{Type: mdflow.EnterEvent, Node: token.SoftBreak},
		{Type: mdflow.TextEvent, Node: token.Text, Text: "b"},
		{Type: mdflow.LeaveEvent, Node: token.Paragraph},
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("Events(%q)\n got: %#v\nwant: %#v", src, gotEvents, wantEvents)
	}
	if !gotEvents[2].IsAtomic() {
		t.Fatalf("soft break event is not atomic: %#v", gotEvents[2])
	}

	const wantHTML = "<p>a\nb</p>\n"
	if got := p.HTML(src); got != wantHTML {
		t.Fatalf("HTML(%q) = %q, want %q", src, got, wantHTML)
	}
	if got, want := p.Text(src), "a\nb"; got != want {
		t.Fatalf("Text(%q) = %q, want %q", src, got, want)
	}
	plain := mdflow.NewWith(parser.New(), text.NewRenderer())
	if got, want := plain.HTML(src), "a\nb\n\n"; got != want {
		t.Fatalf("text renderer output for %q = %q, want %q", src, got, want)
	}

	identity := p.Map(func(e mdflow.Event) mdflow.Event { return e })
	if got := identity.HTML(src); got != wantHTML {
		t.Fatalf("identity middleware HTML(%q) = %q, want %q", src, got, wantHTML)
	}
}

func TestImageAltFlattensBreaksToSpaces(t *testing.T) {
	const src = "![soft\nbreak](/soft) ![hard\\\nbreak](/hard)\n"
	const want = `<p><img src="/soft" alt="soft break" /> <img src="/hard" alt="hard break" /></p>` + "\n"

	if got := mdflow.New().HTML(src); got != want {
		t.Fatalf("HTML(%q)\n got: %q\nwant: %q", src, got, want)
	}
}
