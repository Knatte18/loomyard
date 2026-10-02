// parentreview_wiring_test.go pins how wire fills the Discussion-Write parent-review gate:
// the recipe's gate list, the reviewer taken from the seed's parent, and the commit pathspec.

package loomcli

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDiscussionWriteRow_GateListIsDiscussionThenParentReview asserts the embedded recipe's Discussion-Write row lists the discussion gate, then a parent-review gate with pass_on_cap.
func TestDiscussionWriteRow_GateListIsDiscussionThenParentReview(t *testing.T) {
	t.Parallel()

	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) = %v; want nil", err)
	}
	for _, row := range r.Producers {
		if row.Name != "Discussion-Write" {
			continue
		}
		gates, ok := row.Config["gates"].([]any)
		if !ok || len(gates) != 2 {
			t.Fatalf("Discussion-Write gates = %v; want two entries", row.Config["gates"])
		}
		first, _ := gates[0].(map[string]any)
		second, _ := gates[1].(map[string]any)
		if first["name"] != "discussion" {
			t.Errorf("gates[0].name = %v; want discussion", first["name"])
		}
		if second["name"] != "parent-review" {
			t.Errorf("gates[1].name = %v; want parent-review", second["name"])
		}
		if second["pass_on_cap"] != true {
			t.Errorf("gates[1].pass_on_cap = %v; want true", second["pass_on_cap"])
		}
		return
	}
	t.Fatal("no Discussion-Write row in the embedded recipe")
}

// TestNewParentReviewConfig_ReviewerFromSeedParent asserts the reviewer is the seed's Parent and is empty when the seed has none.
func TestNewParentReviewConfig_ReviewerFromSeedParent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, parent, want string
	}{
		{"with parent", "hub:orchestrator", "hub:orchestrator"},
		{"without parent", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			const runID = "child"
			seed := shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo, Parent: tc.parent}
			if err := shedrun.WriteSeed(loc, runID, seed); err != nil {
				t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
			}
			cfg, err := newParentReviewConfig(loc, runID, loomengine.Config{ParentReviewWaitMin: 5}, t.TempDir())
			if err != nil {
				t.Fatalf("newParentReviewConfig = %v; want nil", err)
			}
			if cfg.Reviewer != tc.want {
				t.Errorf("Reviewer = %q; want %q", cfg.Reviewer, tc.want)
			}
			if cfg.Store.Root != loomengine.LoomParentReviewDir(loc) || cfg.Store.LockDir != loomengine.LoomParentReviewLockDir(loc) {
				t.Errorf("Store dirs = %q, %q; want the loomengine accessors", cfg.Store.Root, cfg.Store.LockDir)
			}
			if got := cfg.WaitBound.Minutes(); got != 5 {
				t.Errorf("WaitBound = %v minutes; want 5", got)
			}
		})
	}
}

// TestDiscussionCommitPathspec_IncludesParentReviewDir asserts the discussion commit carries the parent-review round directories.
func TestDiscussionCommitPathspec_IncludesParentReviewDir(t *testing.T) {
	t.Parallel()

	want := []string{loomengine.DiscussionDirRel(), loomengine.LoomParentReviewDirRel()}
	if got := discussionCommitPathspec(); !reflect.DeepEqual(got, want) {
		t.Errorf("discussionCommitPathspec() = %v; want %v", got, want)
	}
}
