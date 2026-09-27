package token

import "testing"

func TestNewTagIsIdempotentByName(t *testing.T) {
	a := NewTag("github.com/Wenrh2004/mdflow/token.test_idempotent")
	if b := NewTag("github.com/Wenrh2004/mdflow/token.test_idempotent"); a != b {
		t.Fatalf("same name gave %d and %d", a, b)
	}
	if got, want := a.String(), "github.com/Wenrh2004/mdflow/token.test_idempotent"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if a.IsAtomic() {
		t.Fatal("NewTag produced an atomic tag")
	}
}

// Registering one name as both paired and atomic means the syntax and output
// halves of a capability disagree about the node's shape; NewTag must refuse
// rather than hand both the same tag.
func TestNewTagRejectsConflictingShape(t *testing.T) {
	NewTag("github.com/Wenrh2004/mdflow/token.test_conflict")
	defer func() {
		if recover() == nil {
			t.Fatal("NewAtomicTag reused a paired tag's name without panicking")
		}
	}()
	NewAtomicTag("github.com/Wenrh2004/mdflow/token.test_conflict")
}

// The tag space is no longer one byte: allocating past 255 must work.
func TestTagSpaceExceedsOneByte(t *testing.T) {
	var last Tag
	for i := 0; i < 300; i++ {
		last = NewTag("github.com/Wenrh2004/mdflow/token.test_space_" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}
	if last <= 255 {
		t.Fatalf("300 fresh tags ended at %d; want past the old one-byte limit", last)
	}
	if last.IsAtomic() {
		t.Fatal("a paired tag past 255 reads as atomic")
	}
}
