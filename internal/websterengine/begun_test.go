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
