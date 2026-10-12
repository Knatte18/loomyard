// begun_test.go pins DispatchScope, the one dispatch scoping begin-batch, run entry and validate share:
// a batch begin-batch recorded counts as begun whether or not it reached a terminal record, only a non-terminal one is forthcoming, and a batch never begun is neither.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
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

func TestLastBegunCard(t *testing.T) {
	tests := []struct {
		name string
		st   *State
		want int
	}{
		{"nil state", nil, 0},
		{"no records", &State{Batches: map[int]*BatchState{}}, 0},
		{"record with cards", &State{Batches: map[int]*BatchState{
			1: {Cards: []string{"01-a", "02-b", "03-c"}},
		}}, 3},
		{"record without cards falls back to its key", &State{Batches: map[int]*BatchState{
			4: {Slug: "old"},
		}}, 4},
		{"done record beside a later non-terminal one", &State{Batches: map[int]*BatchState{
			1: {Cards: []string{"01-a"}, Terminal: true, Status: "done"},
			2: {Cards: []string{"02-b"}},
		}}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := LastBegunCard(tc.st); got != tc.want {
				t.Fatalf("LastBegunCard = %d; want %d", got, tc.want)
			}
		})
	}
}

// TestEditedDoneCards pins what the plan gate's rows do not reach: a done batch's recorded id the plan lacks reports nothing, and a card file is judged through the plan's own source, so an overlaid card counts as edited exactly as the same edit on disk would.
func TestEditedDoneCards(t *testing.T) {
	t.Parallel()

	t.Run("a recorded id the plan lacks reports nothing", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Dir: t.TempDir()}
		st := &State{Batches: map[int]*BatchState{
			1: {Slug: "gone", Terminal: true, Status: "done", Cards: []string{"01-gone"}, CardHashes: map[string]string{"01-gone": "recorded-at-begin"}},
		}}

		if got, err := EditedDoneCards(plan, st); err != nil || len(got) != 0 {
			t.Errorf("EditedDoneCards() = %v, %v; want nothing for an id the plan lacks", got, err)
		}
	})

	t.Run("a card whose proposed text differs from its recorded hash is edited", func(t *testing.T) {
		t.Parallel()
		planDir := t.TempDir()
		plankit.Write(t, planDir, plankit.Plan{
			Approved: true, Language: "go", Framing: "framing",
			Cards: []plankit.Card{{Number: 1, Slug: "one", Summary: "summary"}},
		})
		onDisk, err := os.ReadFile(filepath.Join(planDir, "01-one.md"))
		if err != nil {
			t.Fatalf("ReadFile(01-one.md) failed: %v", err)
		}
		sum := sha256.Sum256(onDisk)
		st := &State{Batches: map[int]*BatchState{
			1: {Slug: "one", Terminal: true, Status: "done", Cards: []string{"01-one"}, CardHashes: map[string]string{"01-one": hex.EncodeToString(sum[:])}},
		}}

		diskPlan, err := planparser.ParsePlan(planDir)
		if err != nil {
			t.Fatalf("ParsePlan(...) returned error: %v", err)
		}
		if got, err := EditedDoneCards(diskPlan, st); err != nil || len(got) != 0 {
			t.Errorf("EditedDoneCards(disk plan) = %v, %v; want nothing for a card that hashes as recorded", got, err)
		}

		overlay, err := planparser.NewOverlay(planDir, map[string]string{"01-one.md": string(onDisk) + "\nproposed edit\n"})
		if err != nil {
			t.Fatalf("NewOverlay(...) returned error: %v", err)
		}
		overlaid, err := overlay.Parse()
		if err != nil {
			t.Fatalf("Overlay.Parse() returned error: %v", err)
		}
		if got, err := EditedDoneCards(overlaid, st); err != nil || !slices.Equal(got, []string{"01-one"}) {
			t.Errorf("EditedDoneCards(overlaid plan) = %v, %v; want [01-one]", got, err)
		}
	})
}
