package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	internalrawhtml "github.com/Wenrh2004/mdflow/internal/rawhtml"
)

func TestCommonMarkListTightnessAndBlockLayout(t *testing.T) {
	t.Parallel()

	wanted := map[int]bool{
		4: true, 5: true, 7: true, 9: true,
		108: true, 109: true,
		270: true, 271: true, 273: true, 274: true, 277: true,
		286: true, 287: true, 288: true, 289: true, 290: true,
	}
	for n := 306; n <= 326; n++ {
		if n != 317 { // reference-definition visibility belongs to the ref slice
			wanted[n] = true
		}
	}

	p := mdflow.New(mdflow.WithExtensions(internalrawhtml.UnsafeHTML))
	identity := p.Map(func(event mdflow.Event) mdflow.Event { return event })
	seen := 0
	for _, example := range loadSpecExamples(t) {
		if !wanted[example.Example] {
			continue
		}
		seen++
		if got := p.HTML(example.Markdown); got != example.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", example.Example, example.Section, example.Markdown, got, example.HTML)
		}
		if got := identity.HTML(example.Markdown); got != example.HTML {
			t.Errorf("example %d identity pipeline\nmarkdown: %q\n got: %q\nwant: %q", example.Example, example.Markdown, got, example.HTML)
		}
	}
	if seen != len(wanted) {
		t.Fatalf("loaded %d selected examples, want %d", seen, len(wanted))
	}
}

func TestListStreamCommitsOnlyFinalLayout(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"tight nested list":         "- a\n  - b\n",
		"loose sibling items":       "- a\n\n- b\n",
		"nested loose outer tight":  "- a\n  - b\n\n    c\n- d\n",
		"first child indented code": "-\t\tfoo\n",
		"blank inside blockquote":   "* a\n  > b\n  >\n* c\n",
	}

	p := mdflow.New()
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stream := p.Stream()
			var committed strings.Builder
			for i := 0; i < len(src); i++ {
				delta := stream.Feed(src[i : i+1])
				committed.WriteString(delta)
				if delta != "" {
					t.Fatalf("prefix %d committed unresolved list layout: %q", i+1, delta)
				}
				if got, want := committed.String()+stream.Provisional(), p.HTML(src[:i+1]); got != want {
					t.Fatalf("prefix %d %q: committed + provisional = %q, want %q", i+1, src[:i+1], got, want)
				}
			}
			committed.WriteString(stream.Finish())
			if got, want := committed.String(), p.HTML(src); got != want {
				t.Fatalf("closed stream = %q, want %q", got, want)
			}
		})
	}
}
