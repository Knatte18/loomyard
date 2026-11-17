// editeddone_test.go pins what the plan gate's rows do not reach of EditedDoneCards.
// Tier 0: plan files in a temp dir.

package runplan

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEditedDoneCards pins what the plan gate's rows do not reach:
// a done batch's recorded id that the plan lacks reports nothing,
// and a card file is judged through the plan's own source,
// so an overlaid card counts as edited exactly as the same edit on disk would.
func TestEditedDoneCards(t *testing.T) {
	tiergate.Require(t, 0)
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
