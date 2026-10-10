// sequence_test.go covers CheckBatchOrder through its exported surface: a batch list whose every dependency edge (a Uses entry naming another card's Targets entry, or two cards writing one Targets entry in card order) runs forward is accepted, and one with an edge to an earlier batch, including a cycle, is refused with an error that wraps ErrBatchOrder and names both batch numbers, the card, the ref and the way forward.
// Tier 1: package websterengine_test, no git, no disk — every fixture is a hand-built []batcher.Batch
// literal; nothing goes through planparser.ParsePlan.

package websterengine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// oneCardBatch builds a single-card batcher.Batch from a number, slug, Targets list, and Uses list,
// so the table rows below stay readable.
func oneCardBatch(number int, slug string, targets, uses []string) batcher.Batch {
	return batcher.Batch{
		Cards: []planparser.Card{
			{
				Number:  number,
				Slug:    slug,
				Targets: targets,
				Uses:    uses,
			},
		},
	}
}

// TestCheckBatchOrder asserts which batch lists CheckBatchOrder accepts and the findings it names for each refused shape of Targets/Uses relation.
func TestCheckBatchOrder(t *testing.T) {
	t.Parallel()

	const wayForward = "way forward: fix the plan's card order so every card follows the cards whose targets it uses, then `lyx webster rebaseline --card NN` naming each card you edited, or `lyx webster run --fresh` for a run that has not begun a batch"

	tests := []struct {
		name string
		in   []batcher.Batch
		// wantFindings lists the substrings a refusal's message holds;
		// none means the list is accepted.
		wantFindings []string
		// step is the way forward the call is told; a set step replaces the manual verbs.
		step string
	}{
		{
			name: "an empty list is accepted",
		},
		{
			name: "a Uses entry naming an earlier batch's target is accepted",
			in: []batcher.Batch{
				oneCardBatch(1, "producer", []string{"FooFunc"}, nil),
				oneCardBatch(2, "consumer", nil, []string{"FooFunc"}),
			},
		},
		{
			name: "two writers of one target in card order are accepted",
			in: []batcher.Batch{
				oneCardBatch(1, "first", []string{"internal/foo/foo.go"}, nil),
				oneCardBatch(2, "second", []string{"internal/foo/foo.go"}, nil),
			},
		},
		{
			name: "a shared Uses entry and blank refs make no edge",
			in: []batcher.Batch{
				oneCardBatch(1, "reader-a", []string{" "}, []string{"FooFunc"}),
				oneCardBatch(2, "reader-b", nil, []string{"FooFunc", ""}),
			},
		},
		{
			name: "a dependency between cards of one batch is accepted",
			in: []batcher.Batch{{Cards: []planparser.Card{
				{Number: 1, Slug: "producer", Targets: []string{"FooFunc"}},
				{Number: 2, Slug: "consumer", Uses: []string{"FooFunc"}},
			}}},
		},
		{
			name: "a card using a later batch's target is refused naming both batches, the card and the ref",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"FooFunc"}),
				oneCardBatch(2, "producer", []string{"FooFunc"}, nil),
			},
			wantFindings: []string{`batch 1 (card 1) uses "FooFunc", which batch 2 (card 2) targets, but batch 1 runs first`},
		},
		{
			name: "two writers of one target out of card order are refused",
			in: []batcher.Batch{
				oneCardBatch(2, "second", []string{"internal/foo/foo.go"}, nil),
				oneCardBatch(1, "first", []string{"internal/foo/foo.go"}, nil),
			},
			wantFindings: []string{`batch 1 (card 1) and batch 2 (card 2) both target "internal/foo/foo.go" and card 1 comes first, but batch 2 runs first`},
		},
		{
			name: "a two-batch cycle is refused",
			in: []batcher.Batch{
				oneCardBatch(1, "a", []string{"AFunc"}, []string{"BFunc"}),
				oneCardBatch(2, "b", []string{"BFunc"}, []string{"AFunc"}),
			},
			wantFindings: []string{`batch 1 (card 1) uses "BFunc", which batch 2 (card 2) targets, but batch 1 runs first`},
		},
		{
			name: "a set step replaces the manual verbs in the way forward",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"FooFunc"}),
				oneCardBatch(2, "producer", []string{"FooFunc"}, nil),
			},
			wantFindings: []string{`batch 1 (card 1) uses "FooFunc"`},
			step:         "re-step the webster row",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := websterengine.CheckBatchOrder(tc.in, tc.step)
			if len(tc.wantFindings) == 0 {
				if err != nil {
					t.Fatalf("CheckBatchOrder() error = %v; want nil", err)
				}
				return
			}
			if !errors.Is(err, websterengine.ErrBatchOrder) {
				t.Fatalf("CheckBatchOrder() error = %v; want errors.Is(err, ErrBatchOrder)", err)
			}
			clause := wayForward
			if tc.step != "" {
				clause = "way forward: fix the plan's card order so every card follows the cards whose targets it uses, then " + tc.step
				for _, manual := range []string{"lyx webster rebaseline", "lyx webster run"} {
					if strings.Contains(err.Error(), manual) {
						t.Errorf("CheckBatchOrder() error = %q; want it to name neither manual verb, found %q", err.Error(), manual)
					}
				}
			}
			for _, want := range append(tc.wantFindings, clause) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckBatchOrder() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}
