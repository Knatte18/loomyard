// begun_test.go pins DispatchScope, the one dispatch scoping begin-batch, run entry and validate share:
// a batch begin-batch recorded counts as begun whether or not it reached a terminal record, only a non-terminal one is forthcoming, and a batch never begun is neither.

package websterengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestDispatchScope(t *testing.T) {
	batch := func(number int, slug string) batcher.Batch {
		return batcher.Batch{Cards: []planparser.Card{{Number: number, Slug: slug}}}
	}
	batches := []batcher.Batch{batch(1, "done"), batch(2, "begun-unrecorded"), batch(3, "unstarted")}
	st := &State{Batches: map[int]*BatchState{
		1: {Slug: "done", Terminal: true, Status: "done"},
		2: {Slug: "begun-unrecorded"},
	}}

	begun, forthcoming := DispatchScope(batches, st)
	if len(begun) != 2 || begun[0].Slug != "done" || begun[1].Slug != "begun-unrecorded" {
		t.Fatalf("begun = %+v; want the cards of batches 1 and 2", begun)
	}
	if len(forthcoming) != 1 || forthcoming[0].Slug != "begun-unrecorded" {
		t.Fatalf("forthcoming = %+v; want only the card of batch 2", forthcoming)
	}
	if len(completedCards(batches, st, 0)) != 1 {
		t.Fatalf("completedCards should still count only the terminal batch")
	}

	begun, forthcoming = DispatchScope(batches, nil)
	if begun != nil || forthcoming != nil {
		t.Fatalf("DispatchScope(nil state) = %+v, %+v; want nil, nil", begun, forthcoming)
	}
}

// TestEditedDoneCards pins what the plan gate's rows do not reach: a done batch's recorded id the plan lacks reports nothing, and a nil state reports nothing.
func TestEditedDoneCards(t *testing.T) {
	t.Parallel()

	planDir := t.TempDir()
	plan := &planparser.Plan{Dir: planDir}
	st := &State{Batches: map[int]*BatchState{
		1: {Slug: "gone", Terminal: true, Status: "done", Cards: []string{"01-gone"}, CardHashes: map[string]string{"01-gone": "recorded-at-begin"}},
	}}

	if got, err := EditedDoneCards(plan, st, planDir); err != nil || len(got) != 0 {
		t.Errorf("EditedDoneCards() = %v, %v; want nothing for an id the plan lacks", got, err)
	}
	if got, err := EditedDoneCards(plan, nil, planDir); err != nil || got != nil {
		t.Errorf("EditedDoneCards(nil state) = %v, %v; want nil, nil", got, err)
	}
}
