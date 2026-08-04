package mdflow_test

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

// specExample is one entry of the CommonMark spec test suite
// (testdata/spec.json, pinned at spec version 0.31.2).
type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

// unsupportedSections are the spec sections mdflow's core deliberately does not
// implement, so their examples are excluded from the "supported subset" figure.
// Each is a documented omission, not a bug:
//
//   - Indented code blocks, Link reference definitions, HTML blocks: declared
//     absent from the core in the package doc ("Deliberately absent from the
//     core: indented code blocks, reference links, raw HTML blocks ...").
//   - Entity and numeric character references: the core escapes for output but
//     does not decode source entities like &amp; or &#42;.
//   - Tabs: the core does not expand a leading tab to the CommonMark four-space
//     stop, treating indentation by byte instead.
//
// The raw figure below counts every section regardless, so this list can only
// ever make the reported number more generous in a way the reader can audit
// against the section names — it cannot hide a regression, which the floor
// assertions catch on the raw count.
var unsupportedSections = map[string]bool{
	"Indented code blocks":                    true,
	"Link reference definitions":              true,
	"HTML blocks":                             true,
	"Entity and numeric character references": true,
	"Tabs": true,
}

// Regression floors, measured against spec 0.31.2. They assert "no worse than
// today": a change that improves conformance raises the real number above the
// floor and the test still passes, while a regression drops below it and fails.
// Bump them when conformance genuinely improves.
const (
	rawPassFloor     = 321 // out of 652 total
	subsetPassFloor  = 313 // out of the supported-subset total
	subsetRatioFloor = 0.578
)

// TestCommonMarkConformance runs the pinned CommonMark spec suite through the
// CommonMark-only core parser and reports the pass rate, replacing the README's
// prose compliance claims with a measured number. It is a report with a floor,
// not a strict gate: individual mismatches in a supported section are logged,
// not failed, so the suite stays useful as coverage grows.
func TestCommonMarkConformance(t *testing.T) {
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

	p := mdflow.New()

	type stat struct{ pass, total int }
	bySection := map[string]*stat{}
	var rawPass, subsetPass, subsetTotal int

	for _, e := range examples {
		s := bySection[e.Section]
		if s == nil {
			s = &stat{}
			bySection[e.Section] = s
		}
		s.total++
		ok := p.HTML(e.Markdown) == e.HTML
		if ok {
			rawPass++
			s.pass++
		}
		if !unsupportedSections[e.Section] {
			subsetTotal++
			if ok {
				subsetPass++
			}
		}
	}

	sections := make([]string, 0, len(bySection))
	for name := range bySection {
		sections = append(sections, name)
	}
	sort.Strings(sections)
	for _, name := range sections {
		s := bySection[name]
		skipped := ""
		if unsupportedSections[name] {
			skipped = "  (excluded: unsupported)"
		}
		t.Logf("%3d/%3d  %s%s", s.pass, s.total, name, skipped)
	}

	rawRatio := 100 * float64(rawPass) / float64(len(examples))
	subsetRatio := float64(subsetPass) / float64(subsetTotal)
	t.Logf("raw:              %d/%d = %.1f%%", rawPass, len(examples), rawRatio)
	t.Logf("supported subset: %d/%d = %.1f%%", subsetPass, subsetTotal, 100*subsetRatio)

	if rawPass < rawPassFloor {
		t.Errorf("raw conformance regressed: %d passing, floor is %d", rawPass, rawPassFloor)
	}
	if subsetPass < subsetPassFloor {
		t.Errorf("supported-subset conformance regressed: %d passing, floor is %d", subsetPass, subsetPassFloor)
	}
	if subsetRatio < subsetRatioFloor {
		t.Errorf("supported-subset ratio regressed: %.3f, floor is %.3f", subsetRatio, subsetRatioFloor)
	}
}
