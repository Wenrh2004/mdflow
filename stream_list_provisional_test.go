package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

// While a list is open, every event is held until the list closes, so the
// render cache behind Provisional carries the most weight there. At every step
// of a growing list the provisional snapshot must still equal the whole-document
// render of the prefix fed so far, and repeated calls must be stable.
func TestProvisionalCacheMatchesWholeDocumentAcrossGrowingList(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()

	var committed strings.Builder
	var fed strings.Builder
	for i := 0; i < 40; i++ {
		chunk := "- item line one\n  with a second line\n"
		fed.WriteString(chunk)
		committed.WriteString(s.Feed(chunk))

		prov := s.Provisional()
		if again := s.Provisional(); again != prov {
			t.Fatalf("step %d: repeated Provisional differs:\n first %q\nsecond %q", i, prov, again)
		}
		if got, want := committed.String()+prov, p.HTML(fed.String()); got != want {
			t.Fatalf("step %d: committed+Provisional != whole-document render\n got %q\nwant %q", i, got, want)
		}
	}

	committed.WriteString(s.Close())
	if got, want := committed.String(), p.HTML(fed.String()); got != want {
		t.Fatalf("final committed != whole-document render\n got %q\nwant %q", got, want)
	}
}

// A list whose tightness flips loose partway (a blank line between items) must
// still render correctly through the cache — the flip retroactively changes
// earlier items, which the per-event signature must catch.
func TestProvisionalCacheHandlesTightToLooseFlip(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()

	steps := []string{"- a\n", "- b\n", "\n", "- c\n"} // the blank makes the list loose
	var committed, fed strings.Builder
	for i, chunk := range steps {
		fed.WriteString(chunk)
		committed.WriteString(s.Feed(chunk))
		if got, want := committed.String()+s.Provisional(), p.HTML(fed.String()); got != want {
			t.Fatalf("step %d after %q: mismatch\n got %q\nwant %q", i, chunk, got, want)
		}
	}
	committed.WriteString(s.Close())
	if got, want := committed.String(), p.HTML(fed.String()); got != want {
		t.Fatalf("final mismatch\n got %q\nwant %q", got, want)
	}
}

// A driver-level suffix (a reference blocked before a list opens) draining when
// its definition arrives commits output without emitting a new block event, so
// the provisional render cache must invalidate on committed output, not on the
// block count. This exercises that path: the cache must stay correct as the
// committed base shifts underneath an open list.
func TestProvisionalCacheHandlesSuffixDrainUnderOpenList(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()

	steps := []string{
		"[foo]\n\n",     // a shortcut reference with no definition yet: blocks the driver
		"- a\n",         // open a list (its events are held for tightness)
		"- b\n",         // grow the list; Provisional caches its held prefix
		"[foo]: /url\n", // define foo: resolves the blocked suffix, no new block event
		"- c\n",         // keep the list open
	}
	var committed, fed strings.Builder
	for i, chunk := range steps {
		fed.WriteString(chunk)
		committed.WriteString(s.Feed(chunk))
		if got, want := committed.String()+s.Provisional(), p.HTML(fed.String()); got != want {
			t.Fatalf("step %d after %q: mismatch\n got %q\nwant %q", i, chunk, got, want)
		}
	}
	committed.WriteString(s.Close())
	if got, want := committed.String(), p.HTML(fed.String()); got != want {
		t.Fatalf("final mismatch\n got %q\nwant %q", got, want)
	}
}
