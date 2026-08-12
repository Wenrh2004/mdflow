package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

// Blocked reports the normalized label a stream is currently withholding output
// behind, and clears once a matching definition arrives.
func TestStreamBlockedReportsPendingLabel(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()

	s.Feed("[foo]\n\n")
	if label, ok := s.Blocked(); !ok || label != "foo" {
		t.Fatalf("expected blocked on %q, got (%q, %v)", "foo", label, ok)
	}

	out := s.Feed("[foo]: /url\n\n")
	if label, ok := s.Blocked(); ok {
		t.Fatalf("still blocked on %q after the definition arrived", label)
	}
	if !strings.Contains(out, `href="/url"`) {
		t.Fatalf("definition did not resolve the reference: %q", out)
	}
}

// A closed stream is never blocked.
func TestStreamBlockedIsFalseAfterClose(t *testing.T) {
	s := mdflow.New().Stream()
	s.Feed("[foo]\n\n")
	s.Close()
	if _, ok := s.Blocked(); ok {
		t.Fatalf("closed stream reported blocked")
	}
}

// Strict (default) still withholds everything behind an undefined shortcut
// reference until Close, and the final output equals the whole-document render.
func TestStreamStrictWithholdsUndefinedReference(t *testing.T) {
	p := mdflow.New()
	s := p.Stream()

	var committed strings.Builder
	committed.WriteString(s.Feed("[foo]\n\nbar\n\nbaz\n\n"))
	if committed.Len() != 0 {
		t.Fatalf("strict stream committed before Close: %q", committed.String())
	}
	committed.WriteString(s.Close())

	if got, want := committed.String(), p.HTML("[foo]\n\nbar\n\nbaz\n\n"); got != want {
		t.Fatalf("strict final mismatch:\n got %q\nwant %q", got, want)
	}
}

// The opt-in seal policy commits an undefined shortcut reference as literal text
// once a following block queues behind it — before Close — while the final
// output still equals the whole-document render (Close seals it either way).
func TestStreamSealUndefinedReferencesCommitsEarly(t *testing.T) {
	p := mdflow.New()
	s := p.Stream(mdflow.SealUndefinedReferencesAfter(1))

	var committed strings.Builder
	committed.WriteString(s.Feed("[foo]\n\nbar\n\n"))
	if !strings.Contains(committed.String(), "<p>[foo]</p>") {
		t.Fatalf("seal policy did not commit [foo] early: %q", committed.String())
	}
	committed.WriteString(s.Close())

	if got, want := committed.String(), p.HTML("[foo]\n\nbar\n\n"); got != want {
		t.Fatalf("seal final mismatch:\n got %q\nwant %q", got, want)
	}
}

// The seal policy is a correctness-for-liveness trade: a definition that arrives
// after the threshold fires can no longer resolve the reference, so the output
// deliberately diverges from strict CommonMark. This pins that divergence so it
// stays intentional, and confirms strict mode does resolve the late definition.
func TestStreamSealUndefinedDivergesFromStrictOnLateDefinition(t *testing.T) {
	p := mdflow.New()
	doc := "[foo]\n\nbar\n\n[foo]: /url\n\n"

	strict := p.Stream()
	var strictOut strings.Builder
	strictOut.WriteString(strict.Feed(doc))
	strictOut.WriteString(strict.Close())
	if got, want := strictOut.String(), p.HTML(doc); got != want {
		t.Fatalf("strict stream must match whole-document render:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(strictOut.String(), `<a href="/url">foo</a>`) {
		t.Fatalf("strict stream should resolve the late definition: %q", strictOut.String())
	}

	sealed := p.Stream(mdflow.SealUndefinedReferencesAfter(1))
	var sealedOut strings.Builder
	sealedOut.WriteString(sealed.Feed(doc))
	sealedOut.WriteString(sealed.Close())
	if !strings.Contains(sealedOut.String(), "<p>[foo]</p>") {
		t.Fatalf("seal policy should have committed [foo] as literal text: %q", sealedOut.String())
	}
	if strings.Contains(sealedOut.String(), `href="/url"`) {
		t.Fatalf("seal policy must not retroactively resolve a sealed reference: %q", sealedOut.String())
	}
}
