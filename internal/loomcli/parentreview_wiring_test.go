// parentreview_wiring_test.go pins how wire fills the Discussion-Write parent-review gate:
// the recipe's gate list, the reviewer taken from the parent resolver and its liveness seam, and the commit pathspec.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestDiscussionWriteRow_GateListIsDiscussionThenParentReview asserts the embedded recipe's Discussion-Write row lists the discussion gate, then a parent-review gate with attempts 1 and pass_on_cap: one scope review whose rewrite goes on to the perch.
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
		if second["attempts"] != 1 {
			t.Errorf("gates[1].attempts = %v; want 1", second["attempts"])
		}
		if second["pass_on_cap"] != true {
			t.Errorf("gates[1].pass_on_cap = %v; want true", second["pass_on_cap"])
		}
		return
	}
	t.Fatal("no Discussion-Write row in the embedded recipe")
}

// TestNewParentReviewConfig_ReviewerFromResolver asserts the reviewer is the resolver's answer:
// a pair whose origin names no parent worktree gives no reviewer, with or without a recorded shortname, and a nil ReviewerLive.
func TestNewParentReviewConfig_ReviewerFromResolver(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, shortname, want string
	}{
		{"no origin parent", "hub", ""},
		{"no shortname", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tc.shortname != "" {
				boardDir := fabricengine.BoardDir(loc.HubPath)
				if err := os.MkdirAll(boardDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := fabricengine.WriteShortname(boardDir, tc.shortname); err != nil {
					t.Fatalf("WriteShortname = %v; want nil", err)
				}
			}
			cfg, err := newParentReviewConfig(loc, reedengine.Config{}, loomengine.Config{ParentReviewWaitMin: 5}, t.TempDir())
			if err != nil {
				t.Fatalf("newParentReviewConfig = %v; want nil", err)
			}
			if cfg.Reviewer != tc.want {
				t.Errorf("Reviewer = %q; want %q", cfg.Reviewer, tc.want)
			}
			if cfg.ReviewerLive != nil {
				t.Errorf("ReviewerLive is set; want nil for a parent that names no worktree")
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
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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

// TestStrandLive asserts a strand named for the reviewer must be live for the reviewer to count as live,
// that an absent reed session is not live and not an error, and that any other status error is returned.
func TestStrandLive(t *testing.T) {
	t.Parallel()

	const reviewer = "hub:orch"
	statusErr := errors.New("tmux unreachable")
	for _, tc := range []struct {
		name     string
		result   reedengine.StatusResult
		err      error
		wantLive bool
		wantErr  error
	}{
		{"live strand with the reviewer's name", reedengine.StatusResult{Strands: []reedengine.StrandStatus{{Name: reviewer, Live: true}}}, nil, true, nil},
		{"dead strand with the reviewer's name", reedengine.StatusResult{Strands: []reedengine.StrandStatus{{Name: reviewer}}}, nil, false, nil},
		{"live strand with another name", reedengine.StatusResult{Strands: []reedengine.StrandStatus{{Name: "hub:other", Live: true}}}, nil, false, nil},
		{"no strands", reedengine.StatusResult{}, nil, false, nil},
		{"no session", reedengine.StatusResult{}, fmt.Errorf("status: %w", reedengine.ErrNoSession), false, nil},
		{"other status error", reedengine.StatusResult{}, statusErr, false, statusErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			live, err := strandLive(reviewer, func() (reedengine.StatusResult, error) { return tc.result, tc.err })
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("strandLive error = %v; want %v", err, tc.wantErr)
			}
			if live != tc.wantLive {
				t.Errorf("strandLive = %v; want %v", live, tc.wantLive)
			}
		})
	}
}
