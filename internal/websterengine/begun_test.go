// begun_test.go pins begunCards, run-entry validation's resume scope: a batch begin-batch recorded
// counts whether or not it reached a terminal record, and a batch never begun does not.

package websterengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestBegunCards(t *testing.T) {
	batch := func(number int, slug string) batcher.Batch {
		return batcher.Batch{Cards: []planparser.Card{{Number: number, Slug: slug}}}
	}
	batches := []batcher.Batch{batch(1, "done"), batch(2, "begun-unrecorded"), batch(3, "unstarted")}
	st := &State{Batches: map[int]*BatchState{
		1: {Slug: "done", Terminal: true, Status: "done"},
		2: {Slug: "begun-unrecorded"},
	}}

	got := begunCards(batches, st)
	if len(got) != 2 || got[0].Slug != "done" || got[1].Slug != "begun-unrecorded" {
		t.Fatalf("begunCards = %+v; want the cards of batches 1 and 2", got)
	}
	if len(completedCards(batches, st, 0)) != 1 {
		t.Fatalf("completedCards should still count only the terminal batch")
	}
	if begunCards(batches, nil) != nil {
		t.Fatalf("begunCards(nil state) should be nil")
	}
}
