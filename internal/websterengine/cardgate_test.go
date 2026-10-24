//go:build tier0

package websterengine

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// gateTestConfig declares tier0 and tier1 and lists them in the `card` and `batch` keys, tier0 alone in `webster-fix`.
func gateTestConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}, "tier1": {About: "integration"}},
		Gates: map[string]gateslot.GateKey{
			"card":        {Tiers: []int{0}, Scope: "card"},
			"batch":       {Tiers: []int{0, 1}, Scope: "card"},
			"webster-fix": {Tiers: []int{0}, Scope: "diff"},
		},
	}
}

func gateCard(groups ...planparser.TargetGroup) planparser.Card {
	return planparser.Card{Number: 1, Slug: "gate", SourcePath: "_lyx/plan/01-gate.md", TargetGroups: groups}
}

func TestCardGateCommand(t *testing.T) {
	t.Parallel()

	const (
		lintComments = "lyx loom lint-comments"
		lintPattern  = "lyx loom lint-pattern"
	)

	t.Run("a card's gate call is followed by the two lints", func(t *testing.T) {
		t.Parallel()
		got, err := cardGateCommand(gateCard(), gateTestConfig())
		if err != nil {
			t.Fatalf("cardGateCommand() error = %v; want nil", err)
		}
		want := []string{"lyx gate test --wait --gate card --card 1-gate", lintComments, lintPattern}
		if !slices.Equal(got, want) {
			t.Errorf("cardGateCommand() = %q; want %q", got, want)
		}
	})

	t.Run("a dangling card list is refused naming lyx config gate", func(t *testing.T) {
		t.Parallel()
		config := gateTestConfig()
		config.Gates["card"] = gateslot.GateKey{Tiers: []int{7}, Scope: "card"}
		if _, err := cardGateCommand(gateCard(), config); err == nil || !strings.Contains(err.Error(), "lyx config gate") {
			t.Errorf("cardGateCommand() error = %v; want a refusal naming lyx config gate", err)
		}
	})
}

func numberedGateCard(number int, groups ...planparser.TargetGroup) planparser.Card {
	card := gateCard(groups...)
	card.Number = number
	return card
}

//testtiming:keep pins the per-card gate bullet shape re-rooted onto the plan display, one sub-bullet per step; the begin-batch prompt test checks one card's steps
func TestRenderCardGates_OneBulletPerCardWithAStepSubBulletEach(t *testing.T) {
	t.Parallel()

	cards := []planparser.Card{gateCard(), {Number: 2, Slug: "docs", SourcePath: "_lyx/plan/02-docs.md"}}
	got, err := renderCardGates(cards, "_lyx/plan", gateTestConfig())
	if err != nil {
		t.Fatalf("renderCardGates() error = %v; want nil", err)
	}
	want := "- `_lyx/plan/01-gate.md`:\n" +
		"  - `lyx gate test --wait --gate card --card 1-gate`\n" +
		"  - `lyx loom lint-comments`\n" +
		"  - `lyx loom lint-pattern`\n" +
		"- `_lyx/plan/02-docs.md`:\n" +
		"  - `lyx gate test --wait --gate card --card 2-docs`\n" +
		"  - `lyx loom lint-comments`\n" +
		"  - `lyx loom lint-pattern`"
	if got != want {
		t.Errorf("renderCardGates() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderBatchGate(t *testing.T) {
	t.Parallel()

	t.Run("a two-card batch renders one call over every card", func(t *testing.T) {
		t.Parallel()
		cards := []planparser.Card{numberedGateCard(1), numberedGateCard(2)}
		got, err := renderBatchGate(cards, gateTestConfig())
		if err != nil {
			t.Fatalf("renderBatchGate() error = %v; want nil", err)
		}
		want := "- `lyx gate test --wait --gate batch --card 1-gate --card 2-gate`"
		if got != want {
			t.Errorf("renderBatchGate() =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("a dangling batch list is refused naming lyx config gate", func(t *testing.T) {
		t.Parallel()
		config := gateTestConfig()
		delete(config.Gates, "batch")
		if _, err := renderBatchGate([]planparser.Card{numberedGateCard(1)}, config); err == nil || !strings.Contains(err.Error(), "lyx config gate") {
			t.Errorf("renderBatchGate() error = %v; want a refusal naming lyx config gate", err)
		}
	})
}

// TestRenameBoundary_BatchesCheckAndGateTheirOwnTree runs the shape a rename-after-create plan has end to end:
// the cost batchifier splits before the rename, each batch's done-checks read a tree that still holds its own cards' targets, and each batch's gate call names only its own cards.
func TestRenameBoundary_BatchesCheckAndGateTheirOwnTree(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	cards := []planparser.Card{
		numberedGateCard(1, planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"plan:internal/a#Foo"}}),
		numberedGateCard(2, planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"plan:internal/a#Bar"}}),
		numberedGateCard(3, planparser.TargetGroup{
			Type:  planparser.CardTypeRename,
			Refs:  []string{"internal/a/a.go#", "internal/n/a.go#"},
			Pairs: []planparser.MovePair{{Old: "internal/a/a.go#", New: "internal/n/a.go#"}},
		}),
	}
	const source = "package a\n\nfunc Foo() {}\n\nfunc Bar() {}\n"
	firstTree := plankit.Repo(t, map[string]string{"internal/a/a.go": source})
	secondTree := plankit.Repo(t, map[string]string{"internal/n/a.go": source})

	weights := batcher.Weights{Orientation: 1, ForkMessages: 1, MessageContext: 1, TargetMessages: 1, ContextPerLine: 1}
	costBatcher := batcher.NewCost("test", batcher.CostParams{Budget: 1e9, MaxCards: 5, Weights: weights})
	batches, err := costBatcher.Batch(plan, cards, batcher.DiskSizes(firstTree), 0, batcher.StartBase{})
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(batches) != 2 || len(batches[0].Cards) != 2 || len(batches[1].Cards) != 1 {
		t.Fatalf("batch sizes = %d batches; want [2 1]", len(batches))
	}

	for i, tree := range []struct{ root, wantGate string }{
		{firstTree, "- `lyx gate test --wait --gate batch --card 1-gate --card 2-gate`"},
		{secondTree, "- `lyx gate test --wait --gate batch --card 3-gate`"},
	} {
		findings, err := planglyph.DoneChecks(plan, batches[i].Cards, tree.root)
		if err != nil {
			t.Fatalf("batch %d: DoneChecks: %v", i+1, err)
		}
		if len(findings) != 0 {
			t.Errorf("batch %d: DoneChecks = %+v; want no finding", i+1, findings)
		}
		gate, err := renderBatchGate(batches[i].Cards, gateTestConfig())
		if err != nil {
			t.Fatalf("batch %d: renderBatchGate() error = %v; want nil", i+1, err)
		}
		if gate != tree.wantGate {
			t.Errorf("batch %d: renderBatchGate() = %q; want %q", i+1, gate, tree.wantGate)
		}
	}
}
