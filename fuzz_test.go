package mdflow_test

import (
	"strings"
	"testing"

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

// textArtefacts are the bytes the text renderer may emit that were not in the
// input. Two classes, both structure rather than invented content:
//
//   - Framing the renderer adds: a newline separates blocks, and a list item is
//     prefixed with a marker ("- ", "[ ] " or "[x] "), contributing '-', ' ',
//     '[', ']' and 'x'.
//   - The parser's internal hard-break spelling: the block layer rewrites a
//     trailing-space hard break into a backslash before inline structure is
//     known. It is normally re-consumed by the hard-break rule, but inside a
//     multi-line code span it surfaces literally — a documented, pre-existing
//     limitation of this CommonMark *subset* (see the code-span note in doc.go),
//     not content invention. So '\' is allowed too.
//
// Everything outside this set — any letter, digit or other punctuation not in
// the source — would mean text extraction invented document content, which it
// must never do. That is the property this fuzzer guards.
const textArtefacts = "\n- []x\\"

// FuzzText fuzzes the plain-text extraction path: it must never panic, and every
// output byte must be either input content or one of the structural artefacts
// the renderer is allowed to add (see textArtefacts). An invented byte outside
// that set fails the fuzzer.
func FuzzText(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	p := mdflow.New()
	f.Fuzz(func(t *testing.T, src string) {
		out := p.Text(src)
		for i := 0; i < len(out); i++ {
			c := out[i]
			if strings.IndexByte(textArtefacts, c) >= 0 {
				continue
			}
			if strings.IndexByte(src, c) < 0 {
				t.Errorf("Text introduced byte %q not in input or artefacts\n src: %q\n out: %q", c, src, out)
				break
			}
		}
	})
}
