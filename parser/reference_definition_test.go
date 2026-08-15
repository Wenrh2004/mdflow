package parser

import (
	"strings"
	"testing"
)

func TestScanReferenceDefinitionCommonMarkExamples192Through202(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLabel string
		wantDest  string
		wantTitle string
		wantOK    bool
		wantEnd   int
	}{
		{
			name:      "192 ordinary definition",
			src:       `[foo]: /url "title"`,
			wantLabel: "foo", wantDest: "/url", wantTitle: "title", wantOK: true,
		},
		{
			name:      "193 destination and title on following lines",
			src:       "   [foo]: \n      /url  \n           'the title'  ",
			wantLabel: "foo", wantDest: "/url", wantTitle: "the title", wantOK: true,
		},
		{
			name:      "194 escaped label close and balanced destination",
			src:       `[Foo*bar\]]:my_(url) 'title (with parens)'`,
			wantLabel: `Foo*bar\]`, wantDest: "my_(url)", wantTitle: "title (with parens)", wantOK: true,
		},
		{
			name:      "195 pointy destination may contain a space",
			src:       "[Foo bar]:\n<my url>\n'title'",
			wantLabel: "Foo bar", wantDest: "my url", wantTitle: "title", wantOK: true,
		},
		{
			name:      "196 multiline title",
			src:       "[foo]: /url '\ntitle\nline1\nline2\n'",
			wantLabel: "foo", wantDest: "/url", wantTitle: "\ntitle\nline1\nline2\n", wantOK: true,
		},
		{
			name:   "197 title may not contain a blank line",
			src:    "[foo]: /url 'title\n\nwith blank line'",
			wantOK: false,
		},
		{
			name:      "198 title may be omitted",
			src:       "[foo]:\n/url",
			wantLabel: "foo", wantDest: "/url", wantOK: true,
		},
		{
			name:   "199 destination may not be omitted",
			src:    "[foo]:",
			wantOK: false,
		},
		{
			name:      "200 empty pointy destination",
			src:       "[foo]: <>",
			wantLabel: "foo", wantDest: "", wantOK: true,
		},
		{
			name:   "201 title needs a separator",
			src:    "[foo]: <bar>(baz)",
			wantOK: false,
		},
		{
			name:      "202 escapes are decoded only after acceptance",
			src:       `[foo]: /url\bar\*baz "foo\"bar\baz"`,
			wantLabel: "foo", wantDest: `/url\bar*baz`, wantTitle: `foo"bar\baz`, wantOK: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			label, def, end, ok := scanReferenceDefinition(test.src)
			if ok != test.wantOK {
				t.Fatalf("scanReferenceDefinition(%q) ok = %v, want %v", test.src, ok, test.wantOK)
			}
			if !ok {
				return
			}
			wantEnd := test.wantEnd
			if wantEnd == 0 {
				wantEnd = len(test.src)
			}
			if label != test.wantLabel || def.destination != test.wantDest || def.title != test.wantTitle || end != wantEnd {
				t.Fatalf("scanReferenceDefinition(%q) = label %q, def %#v, end %d; want label %q, dest %q, title %q, end %d",
					test.src, label, def, end, test.wantLabel, test.wantDest, test.wantTitle, wantEnd)
			}
		})
	}
}

func TestScanReferenceDefinitionLabelBoundaries(t *testing.T) {
	valid999 := strings.Repeat("é", 999)
	invalid1000 := valid999 + "é"
	tests := []struct {
		name      string
		src       string
		wantLabel string
		wantOK    bool
	}{
		{name: "999 Unicode characters", src: "[" + valid999 + "]: /url", wantLabel: valid999, wantOK: true},
		{name: "1000 Unicode characters", src: "[" + invalid1000 + "]: /url", wantOK: false},
		{name: "escaped closing bracket", src: `[a\]b]: /url`, wantLabel: `a\]b`, wantOK: true},
		{name: "escaped opening bracket", src: `[a\[b]: /url`, wantLabel: `a\[b`, wantOK: true},
		{name: "unescaped opening bracket", src: `[a[b]: /url`, wantOK: false},
		{name: "only label whitespace", src: "[ \t\n ]: /url", wantOK: false},
		{name: "NBSP is label content", src: "[\u00a0]: /url", wantLabel: "\u00a0", wantOK: true},
		{name: "empty label", src: `[]: /url`, wantOK: false},
		{name: "four-space indentation", src: `    [foo]: /url`, wantOK: false},
		{name: "leading tab reaches fourth column", src: "\t[foo]: /url", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			label, _, _, ok := scanReferenceDefinition(test.src)
			if ok != test.wantOK {
				t.Fatalf("scanReferenceDefinition(%q) ok = %v, want %v", test.src, ok, test.wantOK)
			}
			if ok && label != test.wantLabel {
				t.Fatalf("label = %q, want %q", label, test.wantLabel)
			}
		})
	}
}

func TestScanReferenceDefinitionTitleFailureFallsBackOnlyAcrossLine(t *testing.T) {
	t.Run("same line garbage invalidates definition", func(t *testing.T) {
		if _, _, _, ok := scanReferenceDefinition(`[foo]: /url "title" ok`); ok {
			t.Fatal("same-line trailing garbage unexpectedly accepted")
		}
	})

	t.Run("next line invalid title leaves no-title definition", func(t *testing.T) {
		const src = "[foo]: /url\n\"title\" ok"
		label, def, end, ok := scanReferenceDefinition(src)
		if !ok || label != "foo" || def.destination != "/url" || def.title != "" || end != len("[foo]: /url") {
			t.Fatalf("scanReferenceDefinition(%q) = %q, %#v, %d, %v", src, label, def, end, ok)
		}

		var refs referenceResolver
		consumed := scanReferenceDefinitionPrefix(src, &refs)
		if got, want := src[consumed:], `"title" ok`; got != want {
			t.Fatalf("remainder = %q, want %q", got, want)
		}
	})

	t.Run("next line unterminated title also falls back", func(t *testing.T) {
		const src = "[foo]: /url\n'till open"
		_, def, end, ok := scanReferenceDefinition(src)
		if !ok || def.title != "" || end != len("[foo]: /url") {
			t.Fatalf("scanReferenceDefinition(%q) = %#v, %d, %v", src, def, end, ok)
		}
	})
}

func TestScanReferenceDefinitionPrefixRegistersFirstDefinitionOnly(t *testing.T) {
	const src = "[Foo  BAR]: /first 'one'\n[foo bar]: /second 'two'\nvisible"
	var refs referenceResolver
	consumed := scanReferenceDefinitionPrefix(src, &refs)
	if got, want := src[consumed:], "visible"; got != want {
		t.Fatalf("remainder = %q, want %q", got, want)
	}
	def, ok := refs.lookup(" foo\tbar ")
	if !ok || def.destination != "/first" || def.title != "one" {
		t.Fatalf("lookup returned %#v, %v; want first definition", def, ok)
	}
	if len(refs.definitions) != 1 {
		t.Fatalf("stored %d definitions, want 1", len(refs.definitions))
	}
}

func TestReferenceResolverUsesUnicodeFullCaseFold(t *testing.T) {
	var refs referenceResolver
	if !refs.define("ẞ", referenceDefinition{destination: "/first"}) {
		t.Fatal("first definition was rejected")
	}
	if refs.define("SS", referenceDefinition{destination: "/second"}) {
		t.Fatal("case-fold-equivalent duplicate replaced first definition")
	}
	def, ok := refs.lookup("ss")
	if !ok || def.destination != "/first" {
		t.Fatalf("lookup(ss) = %#v, %v", def, ok)
	}

	if !refs.define("ΑΓΩ", referenceDefinition{destination: "/greek"}) {
		t.Fatal("Greek definition was rejected")
	}
	def, ok = refs.lookup("αγω")
	if !ok || def.destination != "/greek" {
		t.Fatalf("Greek lookup = %#v, %v", def, ok)
	}
}

func TestReferenceResolverLifecycle(t *testing.T) {
	var resolver referenceResolver
	if !resolver.define("foo", referenceDefinition{destination: "/first"}) {
		t.Fatal("initial definition was rejected")
	}

	resolver.seal()
	if !resolver.sealed {
		t.Fatal("seal did not mark the resolver sealed")
	}
	if resolver.define("bar", referenceDefinition{destination: "/late"}) {
		t.Fatal("sealed resolver accepted a new definition")
	}
	if definition, ok := resolver.lookup("foo"); !ok || definition.destination != "/first" {
		t.Fatalf("sealed lookup = %#v, %v", definition, ok)
	}

	cloned := resolver.clone()
	if !cloned.sealed {
		t.Fatal("clone lost the sealed state")
	}
	cloned.definitions["foo"] = referenceDefinition{destination: "/clone"}
	if definition, _ := resolver.lookup("foo"); definition.destination != "/first" {
		t.Fatalf("mutating clone changed source to %#v", definition)
	}

	cloned.reset()
	if cloned.sealed || len(cloned.definitions) != 0 {
		t.Fatalf("reset clone = %#v, want empty and unsealed", cloned)
	}
	if !cloned.define("bar", referenceDefinition{destination: "/after-reset"}) {
		t.Fatal("reset resolver rejected a new definition")
	}
	if _, ok := resolver.lookup("bar"); ok {
		t.Fatal("reset clone mutated source definitions")
	}
}

func TestScanReferenceDefinitionPrefixAcceptsLogicalLineEndings(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		src := "[foo]: /one" + ending + "[bar]: /two" + ending + "rest"
		var refs referenceResolver
		consumed := scanReferenceDefinitionPrefix(src, &refs)
		if got := src[consumed:]; got != "rest" {
			t.Fatalf("ending %q: remainder = %q, want rest", ending, got)
		}
		if _, ok := refs.lookup("foo"); !ok {
			t.Fatalf("ending %q: foo missing", ending)
		}
		if _, ok := refs.lookup("bar"); !ok {
			t.Fatalf("ending %q: bar missing", ending)
		}
	}
}
