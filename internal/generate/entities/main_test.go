package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestCheckedInWHATWGSnapshot(t *testing.T) {
	data, err := os.ReadFile("entities.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "d741d877ac77c4194c4ad526b5b4a19aef8dfe411ab840a466891cdbb9f362e6" {
		t.Fatalf("snapshot SHA-256 = %s", got)
	}
	entities, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(entities), 2125; got != want {
		t.Fatalf("entities = %d, want %d", got, want)
	}
}

// TestGeneratedTableIsCurrent regenerates the table from the checked-in
// snapshot and requires the checked-in file to match, so a hand edit to
// parser/entities_gen.go — or a generator change nobody re-ran — fails here.
func TestGeneratedTableIsCurrent(t *testing.T) {
	data, err := os.ReadFile("entities.json")
	if err != nil {
		t.Fatal(err)
	}
	entities, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := render(entities)
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, err := os.ReadFile("../../../parser/entities_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("parser/entities_gen.go is stale; run go generate ./parser")
	}
}
