package mdflow_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Wenrh2004/mdflow"
)

// fuzzSeeds are inputs worth starting every target from: the trigger bytes, the
// block openers, and a few shapes that have historically been fiddly (nested
// emphasis, an unterminated fence, a lone CR).
var fuzzSeeds = []string{
	"",
	"\n",
	"# h\n\ntext\n",
	"*a* **b** `c`\n",
	"***bi***\n",
	"[a](/b \"t\")\n",
	"![img](/i.png)\n",
	"<https://go.dev>\n",
	"> quote\n>\n> more\n",
	"- one\n- two\n\n1. a\n2. b\n",
	"- [ ] todo\n- [x] done\n",
	"```go\nfunc main() {}\n```\n",
	"a\r\nb\r\n",
	"para  \nwith hard break\n",
	"\\*escaped\\*\n",
	"unterminated `code span\n",
	"###### h6\n",
	"---\n",
	"\x84\x00&amp;\n",
}

// FuzzHTML asserts two invariants over arbitrary input: rendering never panics,
// and the []byte entry point produces the exact same bytes as the string one —
// the zero-copy asString bridge must be transparent. A panic or a divergence
// fails the fuzzer.
func FuzzHTML(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	p := mdflow.New()
	f.Fuzz(func(t *testing.T, src string) {
		got := p.HTML(src)
		if b := p.HTMLBytes([]byte(src)); b != got {
			t.Errorf("HTMLBytes disagrees with HTML\n src: %q\n str: %q\nbytes: %q", src, got, b)
		}
		checkOutputBounded(t, src, got)
	})
}

// FuzzStreamMatchesBatch is the streaming invariant: for any input and any chunk
// size, feeding the stream chunk by chunk and closing it must yield exactly the
// same HTML as a single-shot render. This is the property the whole incremental
// design rests on, so it is the one most worth fuzzing — a split point that
// changes the output is a bug in the state machine.
func FuzzStreamMatchesBatch(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, 1)
		f.Add(s, 3)
		f.Add(s, 7)
	}
	p := mdflow.New()
	f.Fuzz(func(t *testing.T, src string, chunk int) {
		if chunk <= 0 {
			chunk = 1
		}
		if chunk > len(src) && len(src) > 0 {
			chunk = len(src)
		}
		want := p.HTML(src)
		var got strings.Builder
		s := p.Stream()
		for i := 0; i < len(src); i += chunk {
			got.WriteString(s.Feed(src[i:min(i+chunk, len(src))]))
		}
		got.WriteString(s.Close())
		if got.String() != want {
			t.Errorf("stream != batch\n src: %q chunk=%d\n got: %q\nwant: %q", src, chunk, got.String(), want)
		}
	})
}

// FuzzText fuzzes the plain-text extraction path. CommonMark source
// normalisation may replace invalid UTF-8/NUL with U+FFFD, and entity decoding
// may emit Unicode bytes not literally present in the source, so byte-provenance
// is not a valid invariant. The actual boundary is that extraction is always
// valid UTF-8, never leaks U+0000, and the []byte bridge is transparent.
func FuzzText(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	p := mdflow.New()
	f.Fuzz(func(t *testing.T, src string) {
		out := p.Text(src)
		if !utf8.ValidString(out) {
			t.Errorf("Text returned invalid UTF-8\n src: %q\n out: %q", src, out)
		}
		if strings.IndexByte(out, 0) >= 0 {
			t.Errorf("Text leaked U+0000\n src: %q\n out: %q", src, out)
		}
		if bytes := p.TextBytes([]byte(src)); bytes != out {
			t.Errorf("TextBytes disagrees with Text\n src: %q\n str: %q\nbytes: %q", src, out, bytes)
		}
	})
}

// checkOutputBounded asserts output stays linear in input. The factor covers
// the densest legitimate markup (a lone '>' opens and closes a whole
// <blockquote>); the constant covers the reference-expansion budget, which may
// add up to its floor regardless of input size. A quadratic blowup — a
// reference expanded thousands of times, a table padded cell by cell — breaks
// this within a few kilobytes.
func checkOutputBounded(t *testing.T, src, out string) {
	t.Helper()
	if limit := 64*len(src) + 256<<10; len(out) > limit {
		t.Errorf("output %d bytes from %d bytes of input exceeds %d\n src: %q", len(out), len(src), limit, src)
	}
}
