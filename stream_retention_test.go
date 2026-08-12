package mdflow

import (
	"strings"
	"testing"
)

func TestClosedStreamDropsDocumentState(t *testing.T) {
	s := New().Stream()
	s.Feed("- [label]\n\n[label]: /" + strings.Repeat("destination", 128))
	if got := s.Close(); got == "" {
		t.Fatal("Close returned no final output")
	}
	blocks := s.Blocks()
	if blocks == 0 {
		t.Fatal("closed stream lost its final block count")
	}

	if s.driver != nil {
		t.Fatal("closed stream retained its document driver")
	}
	if s.pending.Len() != 0 || s.pending.Cap() != 0 {
		t.Fatalf("closed stream retained pending input: len=%d cap=%d", s.pending.Len(), s.pending.Cap())
	}
	if s.out.Len() != 0 || s.out.Cap() != 0 {
		t.Fatalf("closed stream retained output scratch: len=%d cap=%d", s.out.Len(), s.out.Cap())
	}
	if got := s.Blocks(); got != blocks {
		t.Fatalf("Blocks after cleanup = %d, want %d", got, blocks)
	}
}

func TestOpenStreamDropsCommittedOutputScratch(t *testing.T) {
	s := New().Stream()
	src := "# " + strings.Repeat("heading", 256) + "\n\n"
	first := s.Feed(src)
	if first == "" {
		t.Fatal("Feed returned no committed output")
	}
	if want := New().HTML(src); first != want {
		t.Fatalf("first delta = %q, want %q", first, want)
	}
	if s.out.Len() != 0 || s.out.Cap() != 0 {
		t.Fatalf("open stream retained committed output: len=%d cap=%d", s.out.Len(), s.out.Cap())
	}

	got := s.Feed("after\n") + s.Close()
	if want := New().HTML("after\n"); got != want {
		t.Fatalf("stream after scratch release = %q, want %q", got, want)
	}
	if want := New().HTML(src); first != want {
		t.Fatalf("later reuse mutated the returned delta: got %q want %q", first, want)
	}
}
