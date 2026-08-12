package main

import (
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
