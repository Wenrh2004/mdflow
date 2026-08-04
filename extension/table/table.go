// Package table implements GFM tables with column alignment as an mdflow
// extension. It lives in its own module: the core imports none of it, and a
// binary links it only by naming table.Table (or a bundle that includes it).
//
// A table is the one construct that needs a line of look-back — a header row is
// indistinguishable from a paragraph until the delimiter row arrives — so it is
// also the proof that the parser's public seam is enough to write a multi-line,
// multi-event block entirely out of tree. [Syntax] registers a leaf rule that
// promotes the open paragraph, a continuation that claims body rows, and a
// finaliser that expands the buffered rows into container/cell events; none of
// it reaches into unexported parser state.
package table

import (
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// The tags this extension allocates. NewTag is idempotent by name, so the rule
// half and the render half agree without a shared constant, across modules.
// TagTable doubles as the accumulating leaf's tag and the outer container's tag:
// finalisers are keyed by the leaf's tag, so closing the leaf runs finaliseTable.
var (
	tableTag     = token.NewTag("table")
	tableHeadTag = token.NewTag("table_head")
	tableBodyTag = token.NewTag("table_body")
	tableRowTag  = token.NewTag("table_row")
	tableCellTag = token.NewTag("table_cell")
)

// tableScratch is the per-table state a rule writes at open and a finaliser
// reads back at close: one alignment per column, parsed from the delimiter row.
// It is written once and only read afterwards, so [parser.BlockState.Clone]'s
// shallow copy of it is safe.
type tableScratch struct {
	aligns []token.Align
}

// Table is the GFM table capability: the syntax and the HTML it produces.
var Table = extension.Capability{
	Name:   "table",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers the table rules on p, independent of any renderer. A second
// output format reuses this half and supplies its own [extension.Capability].
func Syntax(p *parser.RuleSet) {
	p.AddLeafRule(tableRule{})
	p.AddContinuation(continueTable)
	parser.AddFinalise(p, tableTag, finaliseTable)
}

// IsTable reports whether e enters or leaves any part of a table.
func IsTable(e mdflow.Event) bool {
	switch e.Node {
	case token.CustomContainer:
		return e.Tag == tableTag || e.Tag == tableHeadTag ||
			e.Tag == tableBodyTag || e.Tag == tableRowTag
	case token.CustomLeaf:
		return e.Tag == tableCellTag
	}
	return false
}

// ---- syntax ----

// tableRule starts a table when the current line is a delimiter row agreeing
// with a single-line open paragraph above it. It peeks at that paragraph rather
// than buffering the document, which keeps the parser single-pass.
type tableRule struct{}

func (tableRule) Name() string { return "table" }

func (tableRule) Open(s *parser.BlockState, line string) bool {
	if s.AccumulatorTag() == tableTag {
		return false // an open table's rows are claimed by continueTable
	}
	lines := s.OpenParagraphLines()
	if len(lines) != 1 || !strings.ContainsRune(lines[0], '|') {
		return false
	}
	aligns, ok := parseDelimiterRow(line)
	if !ok {
		return false
	}
	if countCells(lines[0]) != len(aligns) {
		return false
	}
	// Absorb the paragraph as the header, then accumulate body rows onto it.
	parser.PromoteParagraph(s, tableTag, tableScratch{aligns: aligns})
	return true
}

// continueTable folds one more body row into the open table. It returns false
// at the line that ends the table, having closed the leaf first so the rule
// loop then sees a clean slate.
func continueTable(s *parser.BlockState, line string) bool {
	if s.AccumulatorTag() != tableTag {
		return false
	}
	if parser.IsBlank(line) {
		return false // blankRule closes it
	}
	if !strings.ContainsRune(line, '|') {
		s.CloseLeaf()
		return false
	}
	s.AppendLine(line)
	return true
}

// finaliseTable expands the buffered rows into the container/leaf event stream
// when the table closes. lines[0] is the header; the rest are body rows.
func finaliseTable(s *parser.BlockState, lines []string, scratch tableScratch) {
	if len(lines) == 0 {
		return
	}
	aligns := scratch.aligns
	s.Emit(token.BlockEvent{Type: token.OpenBlock, Container: token.CustomContainer, Tag: tableTag})
	s.Emit(token.BlockEvent{Type: token.OpenBlock, Container: token.CustomContainer, Tag: tableHeadTag})
	emitRow(s, lines[0], aligns, true)
	s.Emit(token.BlockEvent{Type: token.CloseBlock, Container: token.CustomContainer, Tag: tableHeadTag})
	if len(lines) > 1 {
		s.Emit(token.BlockEvent{Type: token.OpenBlock, Container: token.CustomContainer, Tag: tableBodyTag})
		for _, row := range lines[1:] {
			emitRow(s, row, aligns, false)
		}
		s.Emit(token.BlockEvent{Type: token.CloseBlock, Container: token.CustomContainer, Tag: tableBodyTag})
	}
	s.Emit(token.BlockEvent{Type: token.CloseBlock, Container: token.CustomContainer, Tag: tableTag})
}

func emitRow(s *parser.BlockState, row string, aligns []token.Align, header bool) {
	s.Emit(token.BlockEvent{Type: token.OpenBlock, Container: token.CustomContainer, Tag: tableRowTag})
	cells := splitRow(nil, row)
	for i, cell := range cells {
		if i >= len(aligns) {
			break // ignore surplus cells, as GFM requires
		}
		s.Emit(token.BlockEvent{Type: token.LeafBlock, Leaf: token.Leaf{
			Node:    token.CustomLeaf,
			Tag:     tableCellTag,
			Content: cell,
			Align:   aligns[i],
			Header:  header,
		}})
	}
	// Short rows are padded with empty cells so every row has the same width.
	for i := len(cells); i < len(aligns); i++ {
		s.Emit(token.BlockEvent{Type: token.LeafBlock, Leaf: token.Leaf{
			Node: token.CustomLeaf, Tag: tableCellTag, Align: aligns[i], Header: header,
		}})
	}
	s.Emit(token.BlockEvent{Type: token.CloseBlock, Container: token.CustomContainer, Tag: tableRowTag})
}

// splitRow splits a table row on unescaped pipes, dropping the optional leading
// and trailing pipe. Cells are subslices of row.
func splitRow(dst []string, row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	if strings.HasSuffix(row, "|") && !strings.HasSuffix(row, "\\|") {
		row = row[:len(row)-1]
	}
	start := 0
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' {
			i++
			continue
		}
		if row[i] == '|' {
			dst = append(dst, strings.TrimSpace(row[start:i]))
			start = i + 1
		}
	}
	return append(dst, strings.TrimSpace(row[start:]))
}

// countCells counts the cells a row would split into, without allocating.
func countCells(row string) int {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	if strings.HasSuffix(row, "|") && !strings.HasSuffix(row, "\\|") {
		row = row[:len(row)-1]
	}
	n := 1
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' {
			i++
			continue
		}
		if row[i] == '|' {
			n++
		}
	}
	return n
}

// parseDelimiterRow recognises `| --- | :-: | ---: |` and returns one alignment
// per column.
func parseDelimiterRow(line string) ([]token.Align, bool) {
	if !strings.ContainsRune(line, '-') || !strings.ContainsRune(line, '|') {
		return nil, false
	}
	cells := splitRow(nil, line)
	aligns := make([]token.Align, 0, len(cells))
	for _, c := range cells {
		a, ok := parseDelimiterCell(c)
		if !ok {
			return nil, false
		}
		aligns = append(aligns, a)
	}
	if len(aligns) == 0 {
		return nil, false
	}
	return aligns, true
}

func parseDelimiterCell(c string) (token.Align, bool) {
	if c == "" {
		return token.AlignDefault, false
	}
	left := c[0] == ':'
	right := c[len(c)-1] == ':'
	body := c
	if left {
		body = body[1:]
	}
	if right && len(body) > 0 {
		body = body[:len(body)-1]
	}
	if body == "" {
		return token.AlignDefault, false
	}
	for i := 0; i < len(body); i++ {
		if body[i] != '-' {
			return token.AlignDefault, false
		}
	}
	switch {
	case left && right:
		return token.AlignCenter, true
	case left:
		return token.AlignLeft, true
	case right:
		return token.AlignRight, true
	default:
		return token.AlignDefault, true
	}
}

// ---- output (HTML) ----

var alignAttr = [4]string{"", ` align="left"`, ` align="center"`, ` align="right"`}

func output(r renderer.Renderer) {
	extension.Container(r, tableTag, "<table>\n", "</table>\n")
	extension.Container(r, tableHeadTag, "<thead>\n", "</thead>\n")
	extension.Container(r, tableBodyTag, "<tbody>\n", "</tbody>\n")
	extension.Container(r, tableRowTag, "<tr>\n", "</tr>\n")

	renderer.RegisterCustomLeaf(r, tableCellTag,
		func(w renderer.Writer, leaf token.Leaf, inlines []token.Inline, rd renderer.Renderer) {
			tag := "td"
			if leaf.Header {
				tag = "th"
			}
			w.WriteByte('<')
			w.WriteString(tag)
			if a := alignAttr[leaf.Align]; a != "" {
				w.WriteString(a)
			}
			w.WriteByte('>')
			rd.RenderInlines(w, inlines)
			w.WriteString("</")
			w.WriteString(tag)
			w.WriteString(">\n")
		})
}
