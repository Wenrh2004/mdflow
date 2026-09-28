package parser

import "github.com/Wenrh2004/mdflow/token"

// Test helpers: whole-document shortcuts over the line-driven machine, which
// production code drives one line at a time through the document driver.

// parseInlineFinal parses a leaf after sealReferences, when no scan can pause.
func (s *BlockState) parseInlineFinal(leaf token.Leaf) []token.Inline {
	if !s.references.sealed {
		panic("mdflow/parser: parseInlineFinal called before sealReferences")
	}
	tokens, cursor := s.startInline(leaf)
	if cursor == nil {
		return tokens
	}
	tokens, complete := cursor.Resume()
	if !complete {
		panic("mdflow/parser: sealed document left an unresolved inline cursor")
	}
	return tokens
}
