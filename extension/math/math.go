// Package math implements inline `$x$` and `$$` fenced math blocks as an mdflow
// extension.
//
// Both halves are verbatim: an inline math span holds literal content like a
// code span, and a math block is a literal leaf that closes on its terminating
// `$$`, so the parser inline-parses neither. The block rule uses the literal
// seam ([parser.BlockState.StartLiteral]) rather than a finaliser, which is the
// same shape a fenced code block uses.
package math

import (
	"html"
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

// mathTag is inline (atomic: a single token, no closing counterpart);
// mathBlockTag is a literal leaf block. Both are allocated by name.
var (
	mathTag      = token.NewAtomicTag("github.com/Wenrh2004/mdflow/extension/math.math")
	mathBlockTag = token.NewTag("github.com/Wenrh2004/mdflow/extension/math.math_block")
)

// Math is the math capability: inline `$x$` and `$$` blocks with their HTML.
var Math = extension.Capability{
	Name:   "math",
	Syntax: Syntax,
	Output: output,
}

// Syntax registers the math rules on p, independent of any renderer.
func Syntax(p *parser.RuleSet) {
	p.AddLeafRule(mathBlockRule{})
	p.AddContinuation(continueMathBlock)
	p.AddInlineRule(mathRule{})
}

// IsMath reports whether e is inline math or a math block.
func IsMath(e mdflow.Event) bool {
	return (e.Node == token.Custom && e.Tag == mathTag) ||
		(e.Node == token.CustomLeaf && e.Tag == mathBlockTag)
}

// ---- inline: $x$ ----

// mathRule handles `$x$`. Content is literal, like a code span.
type mathRule struct{}

func (mathRule) Name() string     { return "math" }
func (mathRule) Triggers() []byte { return []byte{'$'} }
func (mathRule) Match(s *parser.InlineState) bool {
	src, i := s.Src(), s.Pos()
	if i+1 >= len(src) || src[i+1] == '$' {
		return false
	}
	end := strings.IndexByte(src[i+1:], '$')
	if end <= 0 {
		return false
	}
	body := src[i+1 : i+1+end]
	if strings.IndexByte(body, '\n') >= 0 {
		return false
	}
	s.Emit(token.Inline{Node: token.Custom, Tag: mathTag, Text: body})
	s.Advance(end + 2)
	return true
}

// ---- block: $$ ----

// mathBlockRule opens a `$$` fenced math block on a line that is exactly `$$`.
type mathBlockRule struct{}

func (mathBlockRule) Name() string { return "math_block" }
func (mathBlockRule) InterruptsParagraph(line string) bool {
	return isMathBlockFence(line)
}
func (mathBlockRule) Open(s *parser.BlockState, line string) bool {
	if s.AccumulatorTag() == mathBlockTag {
		return false // an open block's lines are claimed by continueMathBlock
	}
	if !isMathBlockFence(line) {
		return false
	}
	s.StartLiteral(mathBlockTag)
	return true
}

// continueMathBlock folds one more line into the open math block, closing it on
// the terminating `$$`.
func continueMathBlock(s *parser.BlockState, line string) bool {
	if s.AccumulatorTag() != mathBlockTag {
		return false
	}
	if isMathBlockFence(line) {
		s.CloseLeaf()
		return true
	}
	s.AppendLine(line)
	return true
}

func isMathBlockFence(line string) bool { return strings.TrimSpace(line) == "$$" }

// ---- output (HTML) ----

func output(r renderer.Renderer) {
	renderer.RegisterCustom(r, mathTag, func(w renderer.Writer, t token.Inline) {
		w.WriteString(`<code class="language-math">`)
		w.WriteString(html.EscapeString(t.Text))
		w.WriteString("</code>")
	})
	renderer.RegisterCustomLeaf(r, mathBlockTag,
		func(w renderer.Writer, leaf token.Leaf, _ []token.Inline, _ renderer.Renderer) {
			w.WriteString(`<pre><code class="language-math">`)
			w.WriteString(html.EscapeString(leaf.Content))
			w.WriteString("</code></pre>\n")
		})
}
