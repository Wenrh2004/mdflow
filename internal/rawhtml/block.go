package rawhtml

import (
	"strings"

	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/token"
)

// Parsing-only accumulator tags encode the one fact a continuation needs: how
// the currently open block terminates. Finalisation projects every one of them
// to the single public BlockTag above.
var (
	accScript      = token.NewTag("raw_html_acc_script")
	accPre         = token.NewTag("raw_html_acc_pre")
	accStyle       = token.NewTag("raw_html_acc_style")
	accTextarea    = token.NewTag("raw_html_acc_textarea")
	accComment     = token.NewTag("raw_html_acc_comment")
	accInstruction = token.NewTag("raw_html_acc_instruction")
	accDeclaration = token.NewTag("raw_html_acc_declaration")
	accCDATA       = token.NewTag("raw_html_acc_cdata")
	accBlank       = token.NewTag("raw_html_acc_blank")
)

var accumulatorTags = [...]token.Tag{
	accScript, accPre, accStyle, accTextarea, accComment,
	accInstruction, accDeclaration, accCDATA, accBlank,
}

type blockStart struct {
	tag       token.Tag
	interrupt bool
}

type blockRule struct{}

func (blockRule) Name() string { return "raw_html_block" }

// InterruptsParagraph is the pure probe used by the block parser's lazy
// continuation decision. CommonMark block types 1-6 interrupt; type 7 does not.
func (blockRule) InterruptsParagraph(line string) bool {
	return (blockRule{}).InterruptsParagraphAt(line, 0)
}

// InterruptsParagraphAt preserves absolute tab stops after container prefixes.
func (blockRule) InterruptsParagraphAt(line string, column int) bool {
	start, ok := scanBlockStartAt(line, column)
	return ok && start.interrupt
}

func (blockRule) Open(s *parser.BlockState, line string) bool {
	start, ok := scanBlockStartAt(line, s.RemainderColumn())
	if !ok || !start.interrupt && len(s.OpenParagraphLines()) > 0 {
		return false
	}
	parser.StartAccumulator(s, start.tag, struct{}{})
	s.AppendLine(line)
	if start.tag != accBlank && blockEnds(start.tag, line) {
		s.CloseLeaf()
	}
	return true
}

func registerBlockFinalisers(p *parser.RuleSet) {
	for _, tag := range accumulatorTags {
		parser.AddFinalise(p, tag, finaliseBlock)
	}
}

func finaliseBlock(s *parser.BlockState, lines []string, _ struct{}) {
	if len(lines) == 0 {
		return
	}
	s.Emit(token.BlockEvent{Type: token.LeafBlock, Leaf: token.Leaf{
		Node:    token.CustomLeaf,
		Tag:     BlockTag,
		Content: strings.Join(lines, "\n") + "\n",
		Literal: true,
	}})
}

func continueBlock(s *parser.BlockState, line string) bool {
	tag := s.AccumulatorTag()
	if !isAccumulatorTag(tag) {
		return false
	}
	if tag == accBlank && parser.IsBlank(line) {
		s.CloseLeaf()
		return false
	}
	s.AppendLine(line)
	if tag != accBlank && blockEnds(tag, line) {
		s.CloseLeaf()
	}
	return true
}

func isAccumulatorTag(tag token.Tag) bool {
	for _, candidate := range accumulatorTags {
		if tag == candidate {
			return true
		}
	}
	return false
}

func scanBlockStartAt(line string, column int) (blockStart, bool) {
	i, ok := htmlBlockIndent(line, column)
	if !ok || i >= len(line) || line[i] != '<' {
		return blockStart{}, false
	}
	tail := line[i+1:]

	for _, candidate := range [...]struct {
		name string
		tag  token.Tag
	}{{"script", accScript}, {"pre", accPre}, {"style", accStyle}, {"textarea", accTextarea}} {
		if hasType1Prefix(tail, candidate.name) {
			return blockStart{tag: candidate.tag, interrupt: true}, true
		}
	}
	switch {
	case strings.HasPrefix(tail, "!--"):
		return blockStart{tag: accComment, interrupt: true}, true
	case strings.HasPrefix(tail, "?"):
		return blockStart{tag: accInstruction, interrupt: true}, true
	case strings.HasPrefix(tail, "![CDATA["):
		return blockStart{tag: accCDATA, interrupt: true}, true
	case len(tail) >= 2 && tail[0] == '!' && isASCIIAlpha(tail[1]):
		return blockStart{tag: accDeclaration, interrupt: true}, true
	case startsType6(tail):
		return blockStart{tag: accBlank, interrupt: true}, true
	case completeType7TagLine(line[i:]):
		return blockStart{tag: accBlank, interrupt: false}, true
	}
	return blockStart{}, false
}

func htmlBlockIndent(line string, column int) (int, bool) {
	indent := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			column++
			indent++
		case '\t':
			width := 4 - column%4
			column += width
			indent += width
		default:
			return i, indent <= 3
		}
		if indent > 3 {
			return 0, false
		}
	}
	return len(line), indent <= 3
}

func hasType1Prefix(tail, name string) bool {
	if len(tail) < len(name) || !equalFoldASCII(tail[:len(name)], name) {
		return false
	}
	if len(tail) == len(name) {
		return true
	}
	c := tail[len(name)]
	return c == ' ' || c == '\t' || c == '>'
}

func blockEnds(tag token.Tag, line string) bool {
	switch tag {
	case accScript, accPre, accStyle, accTextarea:
		// CommonMark type 1 deliberately does not require the closing tag to
		// match the opening tag: any of the four raw-text end tags terminates it.
		for _, endTag := range [...]string{"</pre>", "</script>", "</style>", "</textarea>"} {
			if containsFoldASCII(line, endTag) {
				return true
			}
		}
		return false
	case accComment:
		return strings.Contains(line, "-->")
	case accInstruction:
		return strings.Contains(line, "?>")
	case accDeclaration:
		return strings.ContainsRune(line, '>')
	case accCDATA:
		return strings.Contains(line, "]]>")
	}
	return false
}

func startsType6(tail string) bool {
	if strings.HasPrefix(tail, "/") {
		tail = tail[1:]
	}
	n := 0
	for n < len(tail) && (isASCIIAlpha(tail[n]) || isASCIIDigit(tail[n])) {
		n++
	}
	if n == 0 || !isBlockTag(tail[:n]) {
		return false
	}
	rest := tail[n:]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '>' || strings.HasPrefix(rest, "/>")
}

// HTML block type 6 names from CommonMark 0.31.2, kept sorted for binary search.
var blockTags = [...]string{
	"address", "article", "aside", "base", "basefont", "blockquote", "body",
	"caption", "center", "col", "colgroup", "dd", "details", "dialog", "dir",
	"div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form",
	"frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head",
	"header", "hr", "html", "iframe", "legend", "li", "link", "main", "menu",
	"menuitem", "nav", "noframes", "ol", "optgroup", "option", "p", "param",
	"search", "section", "summary", "table", "tbody", "td", "tfoot", "th",
	"thead", "title", "tr", "track", "ul",
}

func isBlockTag(tag string) bool {
	lo, hi := 0, len(blockTags)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		cmp := compareFoldASCII(tag, blockTags[mid])
		if cmp == 0 {
			return true
		}
		if cmp < 0 {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return false
}

func completeType7TagLine(src string) bool {
	if len(src) < 2 {
		return false
	}
	// Type 7 excludes these four *open* tags. Most spellings are already type 1,
	// but a self-closing spelling such as <script/> does not meet type 1's start
	// condition and must remain an inline tag inside a paragraph. Complete closing
	// tags, including </script>, are still type 7.
	if src[1] != '/' {
		if !isASCIIAlpha(src[1]) {
			return false
		}
		nameEnd := 2
		for nameEnd < len(src) && isTagNameRest(src[nameEnd]) {
			nameEnd++
		}
		name := src[1:nameEnd]
		for _, excluded := range [...]string{"pre", "script", "style", "textarea"} {
			if equalFoldASCII(name, excluded) {
				return false
			}
		}
	}
	end, ok := scanInline(src, 0)
	if !ok {
		return false
	}
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return end == len(src)
}

func equalFoldASCII(a, b string) bool {
	return len(a) == len(b) && compareFoldASCII(a, b) == 0
}

func containsFoldASCII(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalFoldASCII(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func compareFoldASCII(a, b string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		x, y := lowerASCII(a[i]), lowerASCII(b[i])
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
