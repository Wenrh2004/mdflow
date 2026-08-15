package parser

import (
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

var inlineMemoTestKey = token.NewTag("parser_inline_memo_test")

type inlineMemoProbe struct {
	seen  []int
	inits int
}

type inlineMemoCounter int

func (m *inlineMemoCounter) CloneInlineMemo() InlineMemo {
	clone := *m
	return &clone
}

func (*inlineMemoProbe) Name() string     { return "inline_memo_probe" }
func (*inlineMemoProbe) Triggers() []byte { return []byte{'@'} }
func (r *inlineMemoProbe) Match(s *InlineState) bool {
	value := s.Memo(inlineMemoTestKey, func() InlineMemo {
		r.inits++
		return new(inlineMemoCounter)
	}).(*inlineMemoCounter)
	r.seen = append(r.seen, int(*value))
	*value++
	s.AddByte('@')
	s.Advance(1)
	return true
}

func TestInlineMemoIsScopedToOneParse(t *testing.T) {
	rules := New()
	probe := new(inlineMemoProbe)
	rules.AddInlineRule(probe)

	rules.Inline().Parse("@@")
	rules.Inline().Parse("@")

	if got, want := probe.seen, []int{0, 1, 0}; !slicesEqual(got, want) {
		t.Fatalf("memo values crossed a parse boundary: got %v want %v", got, want)
	}
	if probe.inits != 2 {
		t.Fatalf("initializer ran %d times, want once per parse", probe.inits)
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
