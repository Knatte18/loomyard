//go:build tier1

// add_local_integration_test.go covers AddLocal, the local-only pair, and a plain Add's adoption of a local pair's marked branches.
// Every hub here has an origin URL pointing at a path that does not exist, in both repos, so any network call fails the add.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// newDeadOriginHub returns a hub whose warp and weft origin URLs name a path that does not exist.
func newDeadOriginHub(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deadOrigin := t.TempDir() + "/no-such-origin"
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "remote", "set-url", "origin", deadOrigin)
	gitkit.MustRun(t, h.PrimeRecords(), "git", "remote", "set-url", "origin", deadOrigin)
	return h
}

// requireMarked asserts branch's local-only mark in the repo at dir is want.
func requireMarked(t *testing.T, dir, branch string, want bool) {
	t.Helper()

	got, err := gitrepo.New(dir).BranchMarkedLocal(branch)
	if err != nil {
		t.Fatalf("BranchMarkedLocal(%q) in %s: %v", branch, dir, err)
	}
	if got != want {
		t.Errorf("BranchMarkedLocal(%q) in %s = %v; want %v", branch, dir, got, want)
	}
}

// removePairWorktreesByHand removes both worktrees of slug's pair with git, leaving both branches.
func removePairWorktreesByHand(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
	gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(slug))
}

// requireNoPairWorktrees asserts neither worktree of slug's pair exists.
func requireNoPairWorktrees(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for _, p := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists; want no worktree (stat err = %v)", p, err)
		}
	}
}

// TestAddLocal covers AddLocal over a hub with a dead origin, and the adopt-clears rule of a plain Add.
// It stays serial (no t.Parallel), since two rows set the package-level hook.
func TestAddLocal(t *testing.T) {
	const slug = "localpair"
	weftBranch := fabricengine.RecordsBranchName(slug)

	t.Run("FreshPairIsMarkedAndUnlisted", func(t *testing.T) {
		h := newDeadOriginHub(t)

		res, err := h.Topology.AddLocal(h.Location, slug)
		if err != nil {
			t.Fatalf("AddLocal: %v", err)
		}
		if !res.Local {
			t.Errorf("AddResult.Local = false; want true")
		}
		requireMarked(t, h.PrimeWorktree(), slug, true)
		requireMarked(t, h.PrimeRecords(), weftBranch, true)
		for _, p := range []string{filepath.Join(h.Location.HubPath, "_portals"), h.PairLauncherDir(slug)} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Errorf("%s exists; want none for a local pair (stat err = %v)", p, err)
			}
		}
	})

	t.Run("RefusesOverOrdinaryBranch", func(t *testing.T) {
		tests := []struct {
			name  string
			setup func(t *testing.T, h *hubforge.Hub)
			want  []string
		}{
			{
				"LocalWarpBranch",
				func(t *testing.T, h *hubforge.Hub) { gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", slug) },
				[]string{"lym fabric checkout " + slug, "git branch -D " + slug},
			},
			{
				"LocalWeftBranch",
				func(t *testing.T, h *hubforge.Hub) { gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", weftBranch) },
				[]string{"lym fabric add " + slug},
			},
			{
				"OriginWarpRef",
				func(t *testing.T, h *hubforge.Hub) {
					gitkit.MustRun(t, h.PrimeWorktree(), "git", "update-ref", "refs/remotes/origin/"+slug, "HEAD")
				},
				[]string{"lym fabric add " + slug},
			},
			{
				"OriginWeftRef",
				func(t *testing.T, h *hubforge.Hub) {
					gitkit.MustRun(t, h.PrimeRecords(), "git", "update-ref", "refs/remotes/origin/"+weftBranch, "HEAD")
				},
				[]string{"lym fabric add " + slug},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := newDeadOriginHub(t)
				tt.setup(t, h)

				_, err := h.Topology.AddLocal(h.Location, slug)
				var refusal *fabricengine.ErrLocalAddOverOrdinary
				if !errors.As(err, &refusal) {
					t.Fatalf("AddLocal error = %v; want *ErrLocalAddOverOrdinary", err)
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("message %q does not name %q", err.Error(), want)
					}
				}
				requireNoPairWorktrees(t, h, slug)
			})
		}
	})

	t.Run("AdoptsMarkedWarpBranch", func(t *testing.T) {
		h := newDeadOriginHub(t)
		if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
			t.Fatalf("setup AddLocal: %v", err)
		}
		removePairWorktreesByHand(t, h, slug)
		gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "-D", weftBranch)

		if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
			t.Fatalf("AddLocal over the kept marked warp branch: %v", err)
		}
		requireMarked(t, h.PrimeWorktree(), slug, true)
		requireMarked(t, h.PrimeRecords(), weftBranch, true)
	})

	t.Run("HookFailureLeavesNothing", func(t *testing.T) {
		h := newDeadOriginHub(t)
		fabricengine.SetAddFailAfterWarpHookForTest(t, func() error { return errors.New("injected failure") })

		if _, err := h.Topology.AddLocal(h.Location, slug); err == nil {
			t.Fatal("AddLocal succeeded; want the hook's error")
		}
		requireNoPairWorktrees(t, h, slug)
		if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
			t.Errorf("warp branch %q exists after the rolled-back add", slug)
		}
		if gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
			t.Errorf("weft branch %q exists after the rolled-back add", weftBranch)
		}
	})

	t.Run("PlainAddAdoptsAndClearsMarks", func(t *testing.T) {
		h := newDeadOriginHub(t)
		if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
			t.Fatalf("setup AddLocal: %v", err)
		}
		removePairWorktreesByHand(t, h, slug)

		res, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("Add over the local pair's kept branches: %v", err)
		}
		for _, want := range []string{slug, weftBranch} {
			if !slices.Contains(res.LocalMarkCleared, want) {
				t.Errorf("LocalMarkCleared = %v; want it to list %q", res.LocalMarkCleared, want)
			}
		}
		requireMarked(t, h.PrimeWorktree(), slug, false)
		requireMarked(t, h.PrimeRecords(), weftBranch, false)
	})

	t.Run("PlainAddHookFailureKeepsAdoptedMarkedBranch", func(t *testing.T) {
		h := newDeadOriginHub(t)
		if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
			t.Fatalf("setup AddLocal: %v", err)
		}
		removePairWorktreesByHand(t, h, slug)
		fabricengine.SetAddFailAfterWarpHookForTest(t, func() error { return errors.New("injected failure") })

		if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
			t.Fatal("Add succeeded; want the hook's error")
		}
		if !gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
			t.Errorf("adopted warp branch %q was deleted by the rollback", slug)
		}
		requireMarked(t, h.PrimeWorktree(), slug, true)
	})
}
