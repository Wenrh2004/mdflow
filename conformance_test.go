package mdflow_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	internalrawhtml "github.com/Wenrh2004/mdflow/internal/rawhtml"
)

// specExample is one entry of the CommonMark spec test suite
// (testdata/spec.json, pinned at spec version 0.31.2).
type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadSpecExamples(t *testing.T) []specExample {
	t.Helper()
	raw, err := os.ReadFile("testdata/spec.json")
	if err != nil {
		t.Fatalf("read spec.json: %v", err)
	}
	var examples []specExample
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatalf("parse spec.json: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("spec.json held no examples")
	}
	return examples
}

// TestCommonMarkConformance is the release gate for the complete CommonMark
// 0.31.2 protocol. The official examples expect trusted raw HTML verbatim;
// mdflow.New keeps the same syntax safe by default, so this profile changes
// only raw-HTML output.
func TestCommonMarkConformance(t *testing.T) {
	examples := loadSpecExamples(t)
	if got, want := len(examples), 652; got != want {
		t.Fatalf("pinned CommonMark suite has %d examples, want %d", got, want)
	}

	p := mdflow.New(mdflow.WithExtensions(internalrawhtml.UnsafeHTML))
	for _, e := range examples {
		if got := p.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Section, e.Markdown, got, e.HTML)
		}
	}
}

func TestCommonMarkProtocolAcrossEventAndStreamSurfaces(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(internalrawhtml.UnsafeHTML))
	identity := p.Map(func(event mdflow.Event) mdflow.Event { return event })

	for _, e := range loadSpecExamples(t) {
		if got := identity.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d identity pipeline\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Markdown, got, e.HTML)
			continue
		}

		stream := p.Stream()
		var committed strings.Builder
		for i := 0; i < len(e.Markdown); i++ {
			committed.WriteString(stream.Feed(e.Markdown[i : i+1]))
			if got, want := committed.String()+stream.Provisional(), p.HTML(e.Markdown[:i+1]); got != want {
				t.Errorf("example %d stream prefix %d\nmarkdown: %q\n got: %q\nwant: %q", e.Example, i+1, e.Markdown[:i+1], got, want)
				break
			}
		}
		committed.WriteString(stream.Finish())
		if got := committed.String(); got != e.HTML {
			t.Errorf("example %d byte stream\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Markdown, got, e.HTML)
		}
	}
}

func TestStrictCompletedCommonMarkSections(t *testing.T) {
	completed := map[string]bool{
		"ATX headings":                 true,
		"Autolinks":                    true,
		"Blank lines":                  true,
		"Block quotes":                 true,
		"Code spans":                   true,
		"Emphasis and strong emphasis": true,
		"Fenced code blocks":           true,
		"HTML blocks":                  true,
		"Hard line breaks":             true,
		"Indented code blocks":         true,
		"Inlines":                      true,
		"List items":                   true,
		"Paragraphs":                   true,
		"Precedence":                   true,
		"Raw HTML":                     true,
		"Setext headings":              true,
		"Soft line breaks":             true,
		"Tabs":                         true,
		"Textual content":              true,
		"Thematic breaks":              true,
	}
	// The official suite expects raw HTML verbatim. mdflow.New keeps the same
	// syntax safe by default; this trusted profile changes only its output.
	p := mdflow.New(mdflow.WithExtensions(internalrawhtml.UnsafeHTML))
	for _, e := range loadSpecExamples(t) {
		if !completed[e.Section] {
			continue
		}
		if got := p.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Section, e.Markdown, got, e.HTML)
		}
	}
}

func TestStrictCommonMarkInlineLinks(t *testing.T) {
	p := mdflow.New()
	for _, e := range loadSpecExamples(t) {
		// Examples 482-526 are the inline-link portion of the Links section.
		// Examples 491, 494 and 524 require the trusted raw-HTML profile and are
		// covered by TestCommonMarkConformance rather than this narrow safe-profile gate.
		if e.Section != "Links" || e.Example < 482 || e.Example > 526 ||
			e.Example == 491 || e.Example == 494 || e.Example == 524 {
			continue
		}
		if got := p.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Section, e.Markdown, got, e.HTML)
		}
	}
}

func TestStrictCommonMarkInlineImages(t *testing.T) {
	inlineImages := map[int]bool{
		572: true,
		574: true,
		575: true,
		578: true,
		579: true,
		580: true,
		581: true,
	}
	p := mdflow.New()
	for _, e := range loadSpecExamples(t) {
		// The remaining Images examples use reference definitions, which belong
		// to the later reference-link slice rather than this inline-image gate.
		if e.Section != "Images" || !inlineImages[e.Example] {
			continue
		}
		if got := p.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Section, e.Markdown, got, e.HTML)
		}
	}
}

func TestStrictCommonMarkReferences(t *testing.T) {
	p := mdflow.New(mdflow.WithExtensions(internalrawhtml.UnsafeHTML))
	for _, e := range loadSpecExamples(t) {
		referenceCase := e.Section == "Link reference definitions" ||
			e.Section == "Links" && e.Example >= 527 ||
			e.Section == "Images" ||
			e.Example == 23 || e.Example == 33 || e.Example == 317
		if !referenceCase {
			continue
		}
		if got := p.HTML(e.Markdown); got != e.HTML {
			t.Errorf("example %d (%s)\nmarkdown: %q\n got: %q\nwant: %q", e.Example, e.Section, e.Markdown, got, e.HTML)
		}
	}
}
