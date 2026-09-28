package mdflow_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestWriterMatchesHTML(t *testing.T) {
	src := "# Title\n\nSome *text* and [a link][l].\n\n- one\n- two\n\n```go\nx := 1\n```\n\n[l]: /u\n"
	md := mdflow.New()
	for _, chunk := range []int{1, 3, 7, 64, len(src)} {
		var out strings.Builder
		w := md.NewWriter(&out)
		for rest := src; rest != ""; {
			n := min(chunk, len(rest))
			if _, err := io.WriteString(w, rest[:n]); err != nil {
				t.Fatal(err)
			}
			rest = rest[n:]
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), md.HTML(src); got != want {
			t.Fatalf("chunk %d: got %q want %q", chunk, got, want)
		}
	}
}

func TestWriterIoCopy(t *testing.T) {
	var out strings.Builder
	w := mdflow.New().NewWriter(&out)
	if _, err := io.Copy(w, strings.NewReader("a *b*\n\nc\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "<p>a <em>b</em></p>\n<p>c</p>\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriterErrorsAreSticky(t *testing.T) {
	boom := errors.New("boom")
	w := mdflow.New().NewWriter(failWriter{boom})
	if _, err := w.Write([]byte("para\n\n")); !errors.Is(err, boom) {
		t.Fatalf("first failing write: got %v want %v", err, boom)
	}
	if _, err := w.Write([]byte("more\n")); !errors.Is(err, boom) {
		t.Fatalf("write after failure: got %v want sticky %v", err, boom)
	}
	if err := w.Close(); !errors.Is(err, boom) {
		t.Fatalf("close after failure: got %v want sticky %v", err, boom)
	}
}

func TestWriterClosed(t *testing.T) {
	w := mdflow.New().NewWriter(io.Discard)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); !errors.Is(err, mdflow.ErrClosed) {
		t.Fatalf("write after close: got %v want ErrClosed", err)
	}
	if err := w.Close(); !errors.Is(err, mdflow.ErrClosed) {
		t.Fatalf("second close: got %v want ErrClosed", err)
	}
}

func TestParserWithChains(t *testing.T) {
	base := mdflow.New().Transform(mdflow.ShiftHeadings(1))
	derived := base.With(mdflow.WithSafeLinks(), mdflow.WithHTML5())
	src := "# h\n\n[x](javascript:alert(1))  \nnext\n"
	if got, want := derived.HTML(src), "<h2>h</h2>\n<p><a href=\"\">x</a><br>\nnext</p>\n"; got != want {
		t.Fatalf("derived: got %q want %q", got, want)
	}
	if got, want := base.HTML(src), "<h2>h</h2>\n<p><a href=\"javascript:alert(1)\">x</a><br />\nnext</p>\n"; got != want {
		t.Fatalf("receiver changed: got %q want %q", got, want)
	}
	if base.With() != base {
		t.Fatal("With() with no options should return the receiver")
	}
}
