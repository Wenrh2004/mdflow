package mdflow

import (
	"errors"
	"io"
)

// ErrClosed is returned by a [Writer] used after Close.
var ErrClosed = errors.New("mdflow: write to closed Writer")

// Writer is an incremental renderer in the shape of gzip.Writer and
// csv.Writer: Markdown written to it arrives in the underlying io.Writer as
// HTML the moment that HTML becomes final, and Close flushes the rest.
//
//	w := md.NewWriter(resp)
//	io.Copy(w, modelOutput) // an LLM token stream, a tailed file, a socket
//	w.Close()
//
// It is a [Stream] adapted to io.WriteCloser, so everything that holds for a
// Stream holds here: chunk boundaries never affect the result, and the output
// is byte-identical to [Parser.HTML] of the concatenated input.
//
// Errors are sticky, as in bufio.Writer: once the underlying writer fails,
// every later call returns that error without writing.
//
// A Writer is not safe for concurrent use.
type Writer struct {
	s   *Stream
	w   io.Writer
	err error
}

// NewWriter returns a Writer that renders Markdown written to it into w.
func (p *Parser) NewWriter(w io.Writer, opts ...StreamOption) *Writer {
	return &Writer{s: p.Stream(opts...), w: w}
}

// Write feeds p and writes the HTML that became final to the underlying
// writer. It reports len(p) on success: every byte was consumed, even when the
// HTML it completes is still pending.
func (w *Writer) Write(p []byte) (int, error) {
	if _, err := w.WriteString(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteString is [Writer.Write] for a string, avoiding the []byte copy;
// io.WriteString and io.Copy from a strings.Reader use it.
func (w *Writer) WriteString(s string) (int, error) {
	if err := w.check(); err != nil {
		return 0, err
	}
	if err := w.emit(w.s.Feed(s)); err != nil {
		return 0, err
	}
	return len(s), nil
}

// Provisional renders the not-yet-final tail without writing it, for a UI
// that shows a tentative view between writes. See [Stream.Provisional].
func (w *Writer) Provisional() string {
	if w.err != nil {
		return ""
	}
	return w.s.Provisional()
}

// Close closes every open block and writes the remaining HTML. It does not
// close the underlying writer. Closing twice returns ErrClosed.
func (w *Writer) Close() error {
	if err := w.check(); err != nil {
		return err
	}
	err := w.emit(w.s.Finish())
	if err == nil {
		w.err = ErrClosed
	}
	return err
}

func (w *Writer) check() error { return w.err }

func (w *Writer) emit(html string) error {
	if html == "" {
		return nil
	}
	if _, err := io.WriteString(w.w, html); err != nil {
		w.err = err
		return err
	}
	return nil
}

// Interface checks: a Writer is an io.WriteCloser and an io.StringWriter.
var (
	_ io.WriteCloser  = (*Writer)(nil)
	_ io.StringWriter = (*Writer)(nil)
)
