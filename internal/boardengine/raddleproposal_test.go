// raddleproposal_test.go — the lookup of a raddle store's approved proposal by a Sketch slug or a filed slug.

package boardengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRaddleProposal asserts RaddleProposal answers the stored proposal for the Sketch slug, a filed slug and the filed project's slug.
// It asserts RaddleProposal answers not found for any other slug.
// It asserts RaddleProposal keeps answering after the Sketch entry is pruned from board.json.
func TestRaddleProposal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const proposal = "proposal: approved\n"
	tests := []struct {
		name      string
		slug      string
		prune     bool
		wantFound bool
	}{
		{"BySketchSlug", "plan", false, true},
		{"ByFiledSlug", "plan-a", false, true},
		{"ByFiledProjectSlug", "plan-proj", false, true},
		{"BySlugTheStoreDoesNotHold", "other", false, false},
		{"AfterTheSketchEntryIsPruned", "plan", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, boardPath := newPromotionBoard(t)
			if tt.prune {
				// The Sketch entry is created and pruned before the store holds its slug, which the store would refuse to reuse.
				mustUpsertEntry(t, b, map[string]any{"slug": "plan", "title": "Plan", "kind": "task", "labels": bugLabels})
				done := boardengine.StatusDone
				if err := b.SetStatus("plan", &done); err != nil {
					t.Fatalf("SetStatus() error = %v; want nil", err)
				}
				if removed, err := b.Prune(true); err != nil || len(removed) != 1 {
					t.Fatalf("Prune() = %v, %v; want the Sketch entry removed", removed, err)
				}
			}
			writeRaddleStore(t, boardPath, "plan", boardengine.RaddleStoreEntry{Hash: "h", Filed: []string{"plan-proj", "plan-a"}, Project: "plan-proj"})
			if err := os.WriteFile(filepath.Join(boardengine.RaddleStoreDir(boardPath, "plan"), "proposal.yaml"), []byte(proposal), 0o644); err != nil {
				t.Fatalf("write proposal.yaml: %v", err)
			}

			got, found, err := b.RaddleProposal(tt.slug)

			if err != nil || found != tt.wantFound {
				t.Fatalf("RaddleProposal(%q) found, error = %t, %v; want found = %t, nil", tt.slug, found, err, tt.wantFound)
			}
			if tt.wantFound && string(got) != proposal {
				t.Errorf("RaddleProposal(%q) = %q; want %q", tt.slug, got, proposal)
			}
		})
	}
}
