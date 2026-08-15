package parser

import "testing"

func TestNormalizeReferenceLabelASCII(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim and fold", in: "  Foo BAR  ", want: "foo bar"},
		{name: "all label whitespace", in: " \t\r\n ", want: ""},
		{name: "collapse every ASCII label whitespace", in: "foo\t \r\nbar", want: "foo bar"},
		{name: "preserve other ASCII control bytes", in: "foo\vbar", want: "foo\vbar"},
		{name: "already normalised", in: "foo bar", want: "foo bar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeReferenceLabel(test.in); got != test.want {
				t.Fatalf("normalizeReferenceLabel(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestNormalizeReferenceLabelUnicodeFullCaseFold(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "capital sharp s expands", in: "\u1e9e", want: "ss"},
		{name: "small sharp s expands", in: "Stra\u00dfe", want: "strasse"},
		{name: "ligature expands", in: "\ufb03", want: "ffi"},
		{name: "sigma forms agree", in: "\u03a3\u03c3\u03c2", want: "\u03c3\u03c3\u03c3"},
		{name: "default fold is not Turkic", in: "I\u0130\u0131", want: "ii\u0307\u0131"},
		{name: "supplementary mapping", in: "\U00010400", want: "\U00010428"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeReferenceLabel(test.in); got != test.want {
				t.Fatalf("normalizeReferenceLabel(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestNormalizeReferenceLabelDoesNotNormalizeUnicode(t *testing.T) {
	composed := normalizeReferenceLabel("\u00e9")
	decomposed := normalizeReferenceLabel("e\u0301")
	if composed != "\u00e9" {
		t.Fatalf("composed spelling changed to %q", composed)
	}
	if decomposed != "e\u0301" {
		t.Fatalf("decomposed spelling changed to %q", decomposed)
	}
	if composed == decomposed {
		t.Fatal("normalisation unexpectedly made canonically equivalent spellings equal")
	}
}

func TestNormalizeReferenceLabelNormalizedASCIIDoesNotAllocate(t *testing.T) {
	if got := testing.AllocsPerRun(100, func() {
		_ = normalizeReferenceLabel("already normalised")
	}); got != 0 {
		t.Fatalf("allocations = %v, want 0", got)
	}
}
