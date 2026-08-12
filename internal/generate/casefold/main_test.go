package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestCheckedInUnicodeSnapshot(t *testing.T) {
	data, err := os.ReadFile("CaseFolding-15.0.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != sourceSHA {
		t.Fatalf("snapshot SHA-256 = %s, want %s", got, sourceSHA)
	}
	folds, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(folds); got != wantEntries {
		t.Fatalf("mappings = %d, want %d", got, wantEntries)
	}
	runes := 0
	for _, fold := range folds {
		runes += len(fold.target)
	}
	if runes != wantRunes {
		t.Fatalf("mapped runes = %d, want %d", runes, wantRunes)
	}
}

func TestGeneratedTableIsCurrent(t *testing.T) {
	data, err := os.ReadFile("CaseFolding-15.0.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	folds, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := render(folds)
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, err := os.ReadFile("../../../parser/casefold_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("parser/casefold_gen.go is stale; run go generate ./parser")
	}
}

func TestParseUsesDefaultFullMappings(t *testing.T) {
	_, err := parse([]byte(`
0041; C; 0061; # common
0049; T; 0131; # Turkic, excluded
00DF; F; 0073 0073; # full
0130; F; 0069 0307; # default full
0130; T; 0069; # Turkic, excluded
1E9E; S; 00DF; # simple, excluded
1E9E; F; 0073 0073; # full
`))
	if err == nil {
		t.Fatal("parse unexpectedly accepted an incomplete snapshot")
	}
	// parse also enforces the pinned snapshot's complete record count. A small
	// fixture reaching this check proves that only its four C/F records were
	// accepted without weakening the production invariant.
	if got, want := err.Error(), "parsed 4 C/F mappings, want 1530"; got != want {
		t.Fatalf("parse error = %q, want %q", got, want)
	}
}
