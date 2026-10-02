// parentreview_wiring_test.go pins how wire fills the Discussion-Write parent-review gate:
// the recipe's gate list, the reviewer taken from the seed's parent, and the commit pathspec.

package loomcli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDiscussionWriteRow_GateListIsDiscussionThenParentReview asserts the embedded recipe's Discussion-Write row lists the discussion gate, then a parent-review gate with attempts 3 and no pass_on_cap.
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
		if second["attempts"] != 3 {
			t.Errorf("gates[1].attempts = %v; want 3", second["attempts"])
		}
		if _, set := second["pass_on_cap"]; set {
			t.Errorf("gates[1].pass_on_cap = %v; want it absent", second["pass_on_cap"])
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

// TestDiscussionCommitPathspec_IncludesParentReviewDir asserts the discussion commit carries the parent-review round directories once they hold a file,
// and leaves them out while they are absent or hold only an empty round directory, since git refuses a pathspec that matches no file.
func TestDiscussionCommitPathspec_IncludesParentReviewDir(t *testing.T) {
	t.Parallel()

	withoutDir := []string{loomengine.DiscussionDirRel()}
	withDir := []string{loomengine.DiscussionDirRel(), loomengine.LoomParentReviewDirRel()}
	for _, tc := range []struct {
		name  string
		setup func(root string)
		want  []string
	}{
		{"absent", func(string) {}, withoutDir},
		{"empty round", func(root string) { mustMkdir(t, filepath.Join(root, "round-1")) }, withoutDir},
		{"request written", func(root string) {
			mustMkdir(t, filepath.Join(root, "round-1"))
			if err := os.WriteFile(filepath.Join(root, "round-1", "request.json"), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, withDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			tc.setup(loomengine.LoomParentReviewDir(loc))
			if got := discussionCommitPathspec(loc); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("discussionCommitPathspec() = %v; want %v", got, tc.want)
			}
		})
	}
}

// mustMkdir creates dir and its parents, failing the test on error.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
