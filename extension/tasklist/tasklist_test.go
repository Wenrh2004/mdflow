package tasklist_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/tasklist"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/text"
	"github.com/Wenrh2004/mdflow/token"
)

func TestTaskListIsOptIn(t *testing.T) {
	const src = "- [ ] todo\n- [x] done\n- [X] DONE\n"

	if got, want := mdflow.New().HTML(src),
		"<ul>\n<li>[ ] todo</li>\n<li>[x] done</li>\n<li>[X] DONE</li>\n</ul>\n"; got != want {
		t.Fatalf("CommonMark profile parsed GFM task syntax:\n got: %q\nwant: %q", got, want)
	}

	p := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	if got, want := p.HTML(src),
		"<ul>\n<li><input type=\"checkbox\" disabled /> todo</li>\n"+
			"<li><input type=\"checkbox\" checked disabled /> done</li>\n"+
			"<li><input type=\"checkbox\" checked disabled /> DONE</li>\n</ul>\n"; got != want {
		t.Fatalf("task-list extension output:\n got: %q\nwant: %q", got, want)
	}
}

func TestTaskListMarkerNeedsTheListItemHeadContext(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"first direct paragraph", "- [x] yes\n", 1},
		{"nested first direct paragraph", "> - [x] yes\n", 1},
		{"ordinary paragraph", "[x] no\n", 0},
		{"later in first paragraph", "- before [x] no\n", 0},
		{"inside inline markup", "- **[x]** no\n", 0},
		{"second direct paragraph", "- first\n\n  [x] second\n", 0},
		{"paragraph after another block", "- > quote\n\n  [x] second\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Count(p.HTML(tc.src), `type="checkbox"`); got != tc.want {
				t.Fatalf("checkbox count = %d, want %d; HTML: %q", got, tc.want, p.HTML(tc.src))
			}
		})
	}
}

func TestTaskListMarkerSeparator(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"space", "- [x] done\n", true},
		{"tab", "- [x]\tdone\n", true},
		{"form feed", "- [x]\fdone\n", true},
		{"line ending before content", "- [x]\n  done\n", true},
		{"vertical tab", "- [x]\vdone\n", true},
		{"no separator", "- [x]done\n", false},
		{"end of paragraph", "- [x]\n", false},
		{"nonbreaking space", "- [x]\u00a0done\n", false},
		{"punctuation", "- [x]. done\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := slices.Collect(p.Events(tc.src))
			found := false
			for _, event := range events {
				found = found || tasklist.IsTaskListMarker(event)
			}
			if found != tc.want {
				t.Fatalf("marker found = %v, want %v; events: %#v", found, tc.want, events)
			}
		})
	}
}

func TestUncheckedTaskListMarkerAcceptsGFMWhitespace(t *testing.T) {
	rules := parser.New()
	tasklist.Syntax(rules)
	for _, middle := range []byte{' ', '\t', '\n', '\v', '\f', '\r'} {
		t.Run(string([]byte{middle}), func(t *testing.T) {
			leaf := token.Leaf{
				Node: token.Paragraph, Context: token.InlineContextListItemHead,
				Content: "[" + string([]byte{middle}) + "] done",
			}
			for _, inline := range rules.ParseInline(leaf) {
				event := mdflow.InlineEvent(inline)
				if !tasklist.IsTaskListMarker(event) {
					continue
				}
				if tasklist.Checked(event) {
					t.Fatalf("Checked(%q) = true", event.Text)
				}
				if want := "[" + string([]byte{middle}) + "]"; event.Text != want {
					t.Fatalf("marker text = %q, want %q", event.Text, want)
				}
				return
			}
			t.Fatalf("unchecked marker with middle byte %#x was not recognised", middle)
		})
	}
	leaf := token.Leaf{
		Node: token.Paragraph, Context: token.InlineContextListItemHead,
		Content: "[\u00a0] done",
	}
	for _, inline := range rules.ParseInline(leaf) {
		if tasklist.IsTaskListMarker(mdflow.InlineEvent(inline)) {
			t.Fatal("nonbreaking space was accepted inside an unchecked marker")
		}
	}
}

func TestTaskListMarkerEventAndHelpers(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	events := slices.Collect(p.Events("- [X] done\n"))

	var marker mdflow.Event
	found := false
	for _, event := range events {
		if event.Type == mdflow.EnterEvent && event.Node == token.Paragraph {
			if event.Context&token.InlineContextListItemHead == 0 {
				t.Errorf("head paragraph context = %v, want InlineContextListItemHead", event.Context)
			}
		}
		if tasklist.IsTaskListMarker(event) {
			marker, found = event, true
		}
	}
	if !found {
		t.Fatal("task-list marker event not found")
	}
	if !marker.IsAtomic() || marker.Type != mdflow.EnterEvent || marker.Text != "[X]" {
		t.Fatalf("marker event = %#v, want one atomic enter preserving source marker", marker)
	}
	if !tasklist.Checked(marker) {
		t.Fatalf("Checked(%#v) = false", marker)
	}
	if tasklist.Checked(mdflow.Event{Type: mdflow.EnterEvent, Node: token.Text, Text: "[x]"}) {
		t.Fatal("Checked accepted a non-task event")
	}
}

func TestTaskListSyntaxOnlyFallsBackToSourceMarker(t *testing.T) {
	syntaxOnly := extension.Capability{Name: "task-list-syntax", Syntax: tasklist.Syntax}
	p := mdflow.New(mdflow.WithOnly(syntaxOnly))
	if got, want := p.HTML("- [x] done\n"), "<ul>\n<li>[x] done</li>\n</ul>\n"; got != want {
		t.Fatalf("unregistered HTML fallback = %q, want %q", got, want)
	}

	plain := mdflow.New(mdflow.WithExtensions(tasklist.TaskList), mdflow.WithRenderer(text.NewRenderer()))
	if got, want := plain.HTML("- [x] done\n"), "- [x] done\n\n"; got != want {
		t.Fatalf("plain-text fallback = %q, want %q", got, want)
	}
}

func TestTaskListHTML5AndIdentityPipeline(t *testing.T) {
	const src = "- [ ] todo\n- [x] done\n"
	base := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	identity := base.Map(func(event mdflow.Event) mdflow.Event { return event })
	if got, want := identity.HTML(src), base.HTML(src); got != want {
		t.Fatalf("identity pipeline differs:\n got: %q\nwant: %q", got, want)
	}

	html5 := mdflow.New(mdflow.WithExtensions(tasklist.TaskList), mdflow.WithHTML5())
	if got := html5.HTML("- [x] done\n"); strings.Contains(got, " />") || !strings.Contains(got, "disabled>") {
		t.Fatalf("HTML5 checkbox spelling = %q", got)
	}
}

func TestTaskListStreamMatchesBatchAtEveryByteBoundary(t *testing.T) {
	const src = "> - [ ] todo\r\n> - [X]\tdone\n\nfinal\n"
	p := mdflow.New(mdflow.WithExtensions(tasklist.TaskList))
	want := p.HTML(src)

	for split := 1; split <= len(src); split++ {
		stream := p.Stream()
		var got strings.Builder
		for i := 0; i < len(src); i += split {
			got.WriteString(stream.Feed(src[i:min(i+split, len(src))]))
		}
		got.WriteString(stream.Finish())
		if got.String() != want {
			t.Fatalf("chunk=%d:\n got: %q\nwant: %q", split, got.String(), want)
		}
	}
}
