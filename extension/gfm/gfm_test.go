package gfm_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension/gfm"
)

func TestGFMIncludesTaskLists(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(gfm.GFM))
	if got, want := p.HTML("- [ ] todo\n- [X] done\n"),
		"<ul>\n<li><input type=\"checkbox\" disabled /> todo</li>\n"+
			"<li><input type=\"checkbox\" checked disabled /> done</li>\n</ul>\n"; got != want {
		t.Fatalf("GFM task-list output:\n got: %q\nwant: %q", got, want)
	}
}

func TestGFMKeepsCommonMarkLazyContinuation(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(gfm.GFM))
	tests := []string{
		"> paragraph\nordinary continuation\n",
		"> a\n--- | ---\n",
		"> a | b\n--- | ---\n",
	}
	for _, src := range tests {
		want := "<blockquote>\n<p>" + src[2:len(src)-1] + "</p>\n</blockquote>\n"
		if got := p.HTML(src); got != want {
			t.Errorf("GFM lazy continuation for %q:\n got: %q\nwant: %q", src, got, want)
		}
	}
}
