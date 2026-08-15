package parser

import (
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func TestPointyLinkDestinationRejectsLineEndingAndEscapedClose(t *testing.T) {
	rules := New().Inline()
	for _, src := range []string{
		"[link](<foo\nbar>)",
		"[link](<foo\\>)",
	} {
		for _, inline := range rules.Parse(src) {
			if inline.Node == token.Link {
				t.Fatalf("Parse(%q) unexpectedly produced link token %#v", src, inline)
			}
		}
	}
}
