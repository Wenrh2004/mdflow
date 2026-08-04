package mdflow

import (
	"io"
	"iter"
	"unsafe"

	"github.com/Wenrh2004/mdflow/token"
)

// The []byte-input twins of the terminal operations, in the manner of regexp's
// Match / MatchString pair: one spelling per input type, one implementation
// underneath. A caller holding a []byte — an mmap'd file, an HTTP body, an
// LLM's decoded buffer — reaches these without a defensive string(src) copy.
//
// Aliasing contract. Each of these views src as a string with no copy, so the
// same rule the parser already applies to a string input applies here: the
// tokens a parse produces may alias src's bytes, so the caller must not mutate
// src until it is done with the result. HTMLBytes, TextBytes and HeadingsBytes
// each finish materialising before they return, so their *results* never alias
// src; only the lazy EventsBytes / BlocksBytes and the direct RenderBytes hold
// that reference across the caller's own code.

// asString views b as a string without copying. See the aliasing contract
// above: the returned string shares b's backing array, so b must not be mutated
// while the string is in use.
func asString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// HTMLBytes is [Parser.HTML] for a []byte source.
func (p *Parser) HTMLBytes(src []byte) string { return p.HTML(asString(src)) }

// RenderBytes is [Parser.Render] for a []byte source.
func (p *Parser) RenderBytes(w io.Writer, src []byte) error { return p.Render(w, asString(src)) }

// EventsBytes is [Parser.Events] for a []byte source. The returned sequence
// aliases src; see the aliasing contract on this file.
func (p *Parser) EventsBytes(src []byte) iter.Seq[Event] { return p.Events(asString(src)) }

// BlocksBytes is [Parser.Blocks] for a []byte source. The returned sequence
// aliases src; see the aliasing contract on this file.
func (p *Parser) BlocksBytes(src []byte) iter.Seq[token.BlockEvent] {
	return p.Blocks(asString(src))
}

// TextBytes is [Parser.Text] for a []byte source.
func (p *Parser) TextBytes(src []byte) string { return p.Text(asString(src)) }

// HeadingsBytes is [Parser.Headings] for a []byte source.
func (p *Parser) HeadingsBytes(src []byte) []Heading { return p.Headings(asString(src)) }
