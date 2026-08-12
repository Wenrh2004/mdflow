package mdflow_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/parser"
	"github.com/Wenrh2004/mdflow/renderer/html"
	"github.com/Wenrh2004/mdflow/token"
)

var provisionalMemoTag = token.NewTag("reference_stream_provisional_memo")

type provisionalMemo int

func (m *provisionalMemo) CloneInlineMemo() parser.InlineMemo {
	clone := *m
	return &clone
}

type provisionalMemoRule struct{}

func (provisionalMemoRule) Name() string     { return "reference_stream_provisional_memo" }
func (provisionalMemoRule) Triggers() []byte { return []byte{'@'} }
func (provisionalMemoRule) Match(s *parser.InlineState) bool {
	memo := s.Memo(provisionalMemoTag, func() parser.InlineMemo {
		return new(provisionalMemo)
	}).(*provisionalMemo)
	s.AddText(strconv.Itoa(int(*memo)))
	*memo++
	s.Advance(1)
	return true
}

func TestStreamReferenceDefinitionUnlocksEarliestPendingSuffix(t *testing.T) {
	s := mdflow.New().Stream()
	if got := s.Feed("[foo]\n\n"); got != "" {
		t.Fatalf("unresolved reference committed early: %q", got)
	}
	if got := s.Feed("[bar]: /bar\n\n"); got != "" {
		t.Fatalf("unrelated definition unlocked the suffix: %q", got)
	}
	if got, want := s.Feed("[foo]: /url\n\n"), "<p><a href=\"/url\">foo</a></p>\n"; got != want {
		t.Fatalf("matching definition did not unlock the earliest suffix\n got: %q\nwant: %q", got, want)
	}
	if got := s.Close(); got != "" {
		t.Fatalf("definition-only tail rendered at close: %q", got)
	}
}

func TestStreamReferenceProvisionalFinalisesOnlyItsClone(t *testing.T) {
	s := mdflow.New().Stream()
	if got := s.Feed("[foo]\n\n[foo]: /url"); got != "" {
		t.Fatalf("unterminated definition committed early: %q", got)
	}
	want := "<p><a href=\"/url\">foo</a></p>\n"
	if got := s.Provisional(); got != want {
		t.Fatalf("provisional did not resolve the definition at speculative EOF\n got: %q\nwant: %q", got, want)
	}
	if got := s.Provisional(); got != want {
		t.Fatalf("repeated provisional mutated live state\n got: %q\nwant: %q", got, want)
	}
	if got := s.Close(); got != want {
		t.Fatalf("live close was polluted by provisional state\n got: %q\nwant: %q", got, want)
	}
}

func TestRepeatedProvisionalClonesInlineMemoState(t *testing.T) {
	rules := parser.New()
	rules.AddInlineRule(provisionalMemoRule{})
	s := mdflow.NewWith(rules, html.NewRenderer()).Stream()
	if got := s.Feed("@[ref]@\n\n"); got != "" {
		t.Fatalf("unresolved reference committed early: %q", got)
	}

	const provisional = "<p>0[ref]1</p>\n"
	for attempt := 1; attempt <= 3; attempt++ {
		if got := s.Provisional(); got != provisional {
			t.Fatalf("provisional attempt %d mutated live memo state\n got: %q\nwant: %q", attempt, got, provisional)
		}
	}

	const committed = "<p>0<a href=\"/url\">ref</a>1</p>\n"
	if got := s.Feed("[ref]: /url\n\n"); got != committed {
		t.Fatalf("live cursor inherited provisional memo mutations\n got: %q\nwant: %q", got, committed)
	}
	if got := s.Close(); got != "" {
		t.Fatalf("definition-only tail rendered at close: %q", got)
	}
}

func TestRepeatedProvisionalClonesBacktickFrontier(t *testing.T) {
	s := mdflow.New().Stream()
	const source = "``before [ref] `a` and `b`\n\n"
	if got := s.Feed(source); got != "" {
		t.Fatalf("unresolved reference committed early: %q", got)
	}

	const provisional = "<p>``before [ref] <code>a</code> and <code>b</code></p>\n"
	for attempt := 1; attempt <= 3; attempt++ {
		if got := s.Provisional(); got != provisional {
			t.Fatalf("provisional attempt %d mutated live backtick frontier\n got: %q\nwant: %q", attempt, got, provisional)
		}
	}

	const committed = "<p>``before <a href=\"/url\">ref</a> <code>a</code> and <code>b</code></p>\n"
	if got := s.Feed("[ref]: /url\n\n"); got != committed {
		t.Fatalf("live cursor inherited provisional backtick frontier\n got: %q\nwant: %q", got, committed)
	}
	if got := s.Close(); got != "" {
		t.Fatalf("definition-only tail rendered at close: %q", got)
	}
}

func TestStreamMissingReferenceStaysTentativeUntilEOF(t *testing.T) {
	s := mdflow.New().Stream()
	if got := s.Feed("[missing]\n\n"); got != "" {
		t.Fatalf("missing reference committed before EOF: %q", got)
	}
	want := "<p>[missing]</p>\n"
	if got := s.Provisional(); got != want {
		t.Fatalf("provisional missing-reference fallback\n got: %q\nwant: %q", got, want)
	}
	if got := s.Close(); got != want {
		t.Fatalf("EOF missing-reference fallback\n got: %q\nwant: %q", got, want)
	}
}

type referenceSentinelRule struct{ opened *int }

func (r referenceSentinelRule) Name() string { return "reference_test_sentinel" }
func (r referenceSentinelRule) Open(s *parser.BlockState, line string) bool {
	if line != "SENTINEL" {
		return false
	}
	*r.opened++
	s.EmitLeaf(token.Leaf{Node: token.Paragraph, Content: line})
	return true
}

func TestReferenceEventsPullOnlyThroughTheNeededDefinition(t *testing.T) {
	opened := 0
	rules := parser.New()
	rules.AddLeafRule(referenceSentinelRule{opened: &opened})
	p := mdflow.NewWith(rules, html.NewRenderer())

	for range p.Events("[foo]\n\n[foo]: /url\n\nSENTINEL\n") {
		break
	}
	if opened != 0 {
		t.Fatalf("event consumer stopped, but parser read the later sentinel %d time(s)", opened)
	}
}

func TestParallelReferencesUseTheSealedDocumentResolver(t *testing.T) {
	// Keep the definition after enough ordinary blocks to force both the
	// document driver's suffix compaction and the parallel render path.
	src := "[foo]\n\n" + strings.Repeat("ordinary paragraph\n\n", 900) + "[foo]: /url\n"
	p := mdflow.New()
	want := p.HTML(src)
	got := p.Workers(4).HTML(src)
	if got != want {
		t.Fatalf("parallel reference output differs from sequential output\n got prefix: %q\nwant prefix: %q", prefix(got, 160), prefix(want, 160))
	}
	if !strings.HasPrefix(got, `<p><a href="/url">foo</a></p>`+"\n") {
		t.Fatalf("forward reference was not resolved: %q", prefix(got, 160))
	}
	got, err := p.Workers(4).HTMLContext(context.Background(), src)
	if err != nil || got != want {
		t.Fatalf("parallel context reference output = %q, %v; want sequential output", prefix(got, 160), err)
	}
}

func TestReferenceContextDriversMatchPlain(t *testing.T) {
	const src = "[foo]\n\n[foo]: /url\n"
	p := mdflow.New()
	want := p.HTML(src)
	got, err := p.HTMLContext(context.Background(), src)
	if err != nil || got != want {
		t.Fatalf("HTMLContext reference output = %q, %v; want %q", got, err, want)
	}
	text, err := p.TextContext(context.Background(), src)
	if err != nil || text != "foo" {
		t.Fatalf("TextContext reference output = %q, %v; want foo", text, err)
	}
}

func prefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
