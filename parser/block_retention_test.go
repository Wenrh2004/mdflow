package parser

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow/token"
)

func TestBlockStateDropsConsumedDocumentReferences(t *testing.T) {
	state := newBlockState(New())
	line := strings.Repeat("retained", 128)

	state.feedLine("- " + line)
	state.closeAll()
	assertBlockEventsCleared(t, "released list buffer", state.held[:cap(state.held)])
	for i, retained := range state.leafStore.lines[:cap(state.leafStore.lines)] {
		if retained != "" {
			t.Fatalf("closed-leaf slot %d retained consumed input", i)
		}
	}
	state.releaseEvents()
	assertBlockEventsCleared(t, "released line event buffer", state.events[:cap(state.events)])

	state.reset(state.rules)
	assertBlockEventsCleared(t, "line event buffer", state.events[:cap(state.events)])
	for i, retained := range state.leafStore.lines[:cap(state.leafStore.lines)] {
		if retained != "" {
			t.Fatalf("open-leaf slot %d retained consumed input", i)
		}
	}

	state.collectAll("- " + line + "\n")
	state.reset(state.rules)
	assertBlockEventsCleared(t, "parallel collection buffer", state.collected[:cap(state.collected)])

	tag := token.NewTag("block_retention_accumulator")
	rules := New()
	rules.AddLeafRule(retentionAccumulatorRule{tag: tag})
	AddFinalise(rules, tag, func(s *BlockState, _ []string, scratch string) {
		s.EmitLeaf(token.Leaf{Node: token.CustomLeaf, Tag: tag, Content: scratch, Literal: true})
	})
	accumulator := newBlockState(rules)
	accumulator.feedLine(":::" + line)
	if accumulator.leafStore.scratch != nil || accumulator.leafStore.info != "" {
		t.Fatalf("closed accumulator retained scratch: %+v", accumulator.leafStore)
	}
	for i, retained := range accumulator.leafStore.lines[:cap(accumulator.leafStore.lines)] {
		if retained != "" {
			t.Fatalf("closed accumulator line slot %d retained input", i)
		}
	}
}

func TestSetextHeadingDropsConsumedParagraphReferences(t *testing.T) {
	state := newBlockState(New())
	line := strings.Repeat("retained", 128)

	state.feedLine(line)
	state.feedLine("===")
	assertLeafLinesCleared(t, "setext heading", state.leafStore.lines[:cap(state.leafStore.lines)])

	state.feedLine("[label]: /" + line)
	state.feedLine("---")
	assertLeafLinesCleared(t, "definition-only setext candidate", state.leafStore.lines[:cap(state.leafStore.lines)])
}

type retentionAccumulatorRule struct{ tag token.Tag }

func (r retentionAccumulatorRule) Name() string { return "retention_accumulator" }
func (r retentionAccumulatorRule) Open(s *BlockState, in Line) bool {
	line := in.Text
	if !strings.HasPrefix(line, ":::") {
		return false
	}
	StartAccumulator(s, r.tag, line)
	s.AppendLine(line)
	s.CloseLeaf()
	return true
}

func assertBlockEventsCleared(t *testing.T, name string, events []token.BlockEvent) {
	t.Helper()
	for i, event := range events {
		if event != (token.BlockEvent{}) {
			t.Fatalf("%s slot %d retained event %+v", name, i, event)
		}
	}
}

func assertLeafLinesCleared(t *testing.T, name string, lines []string) {
	t.Helper()
	for i, line := range lines {
		if line != "" {
			t.Fatalf("%s slot %d retained consumed input", name, i)
		}
	}
}
