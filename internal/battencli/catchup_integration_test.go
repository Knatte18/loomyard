// catchup_integration_test.go drives the hub's catch-up seams over real hub fixtures with local pairs.
// No test here calls t.Parallel: registering this machine writes the per-user machine file the whole test process shares.

package battencli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// catchUpOver runs the catch-up of the hub over the pairs of slugs, through the production seams.
func catchUpOver(h *hubforge.Hub, slugs ...string) (dobbyengine.CatchUpReport, error) {
	return dobbyengine.CatchUp(HubCatchUp(h.Location, func() ([]string, error) { return slugs, nil }))
}

// recordsTip returns the tip of the records branch on the hub's records origin.
func recordsTip(t *testing.T, h *hubforge.Hub, branch string) string {
	t.Helper()
	return gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+branch)
}

func TestHubCatchUp(t *testing.T) {
	tiergate.Require(t, 1)

	t.Run("A_scope_of_one_slug_pushes_that_pairs_records_commit_and_resets_nothing", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		alpha := hubforge.AddPair(t, h, "alpha")
		beta := hubforge.AddPair(t, h, "beta")
		alphaBranch, betaBranch := fabriclayout.RecordsBranchName(alpha.Branch), fabriclayout.RecordsBranchName(beta.Branch)
		gitkit.CommitFile(t, h.PairRecordsSibling("alpha"), "alpha.txt", "alpha\n", "alpha transition")
		gitkit.CommitFile(t, h.PairRecordsSibling("beta"), "beta.txt", "beta\n", "beta transition")
		alphaHead := gitkit.RevParse(t, h.PairRecordsSibling("alpha"), "HEAD")
		betaTipBefore := recordsTip(t, h, betaBranch)

		report, err := catchUpOver(h, "alpha")

		if err != nil {
			t.Fatalf("CatchUp() error = %v; want nil", err)
		}
		if len(report.Pushed) != 1 || report.Pushed[0] != "alpha" {
			t.Errorf("CatchUp() pushed = %q; want alpha alone", report.Pushed)
		}
		if got := recordsTip(t, h, alphaBranch); got != alphaHead {
			t.Errorf("origin's records tip of alpha = %s; want the local commit %s", got, alphaHead)
		}
		if got := gitkit.RevParse(t, h.PairRecordsSibling("alpha"), "HEAD"); got != alphaHead {
			t.Errorf("local records HEAD of alpha = %s; want %s unchanged", got, alphaHead)
		}
		if got := recordsTip(t, h, betaBranch); got != betaTipBefore {
			t.Errorf("origin's records tip of beta = %s; want %s, since the scope named alpha alone", got, betaTipBefore)
		}
	})

	t.Run("A_pair_directory_with_no_worktree_registration_reads_as_no_pair_here", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		stray := fabricengine.WorktreePath(h.Location, "ghost")
		if err := os.MkdirAll(stray, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stray, "left-over.txt"), []byte("left\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		report, err := catchUpOver(h, "ghost")

		if err != nil {
			t.Fatalf("CatchUp() error = %v; want nil", err)
		}
		if len(report.OwnedHere) != 1 || report.OwnedHere[0] != "ghost" {
			t.Errorf("CatchUp() owned here = %q; want ghost, read as a run with no pair here", report.OwnedHere)
		}
		if _, err := os.Stat(filepath.Join(stray, "left-over.txt")); err != nil {
			t.Errorf("the left-over directory = %v; want it untouched", err)
		}
	})

	t.Run("An_unreachable_origin_returns_ErrOriginUnreachable_and_pushes_nothing", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		alpha := hubforge.AddPair(t, h, "alpha")
		branch := fabriclayout.RecordsBranchName(alpha.Branch)
		gitkit.CommitFile(t, h.PairRecordsSibling("alpha"), "alpha.txt", "alpha\n", "alpha transition")
		tipBefore := recordsTip(t, h, branch)
		gitkit.Git(t, h.PairRecordsSibling("alpha"), "remote", "set-url", "origin", "http://127.0.0.1:1/x")

		report, err := catchUpOver(h, "alpha")

		if !errors.Is(err, dobbyengine.ErrOriginUnreachable) {
			t.Errorf("CatchUp() error = %v; want it to wrap ErrOriginUnreachable", err)
		}
		if len(report.Pushed) != 0 {
			t.Errorf("CatchUp() pushed = %q; want nothing", report.Pushed)
		}
		if got := recordsTip(t, h, branch); got != tipBefore {
			t.Errorf("origin's records tip = %s; want %s unchanged", got, tipBefore)
		}
	})

	t.Run("A_records_store_that_lost_its_fetch_refspecs_returns_its_layout_error_not_ErrOriginUnreachable", func(t *testing.T) {
		// The records store's config is shared by the Bolt checkout, whose pull would fail first on the lost refspecs.
		t.Setenv("BOARD_SKIP_GIT", "1")
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		// With no push the pair's records worktree is never read for its layout, so the answer is read after the refspecs went.
		hubforge.AddPairWith(t, h, "alpha", fabricengine.AddOptions{SkipPush: true})
		gitkit.Git(t, h.PairRecordsSibling("alpha"), "config", "--unset-all", "remote.origin.fetch")

		_, err := catchUpOver(h, "alpha")

		const wantErrorHas = "config --replace-all remote.origin.fetch"
		if err == nil || !strings.Contains(err.Error(), wantErrorHas) {
			t.Fatalf("CatchUp() error = %v; want one naming %q", err, wantErrorHas)
		}
		if errors.Is(err, dobbyengine.ErrOriginUnreachable) {
			t.Errorf("CatchUp() error = %v; want no ErrOriginUnreachable for a layout the fetch cannot read", err)
		}
	})

	t.Run("A_run_with_no_pair_here_over_a_records_store_that_lost_its_fetch_refspecs_returns_its_layout_error", func(t *testing.T) {
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		gitkit.Git(t, h.PrimeRecords(), "config", "--unset-all", "remote.origin.fetch")
		// Building the hub read the prime's layout, which the process caches by path,
		// so the hub is read from a path the process has not read.
		moved := h.Path + "-unread"
		if err := os.Rename(h.Path, moved); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Rename(moved, h.Path) })
		prime, err := lymcwd.ResolveWorktree(filepath.Join(moved, filepath.Base(h.PrimeWorktree())))
		if err != nil {
			t.Fatalf("ResolveWorktree() = %v; want nil", err)
		}

		err = fetchRunForCatchUp(prime, "ghost")

		if !errors.Is(err, fabricengine.ErrRecordsLayout) {
			t.Errorf("fetchRunForCatchUp() error = %v; want it to wrap ErrRecordsLayout", err)
		}
	})

	t.Run("HubRunSlugsListsTheRunsUnderThePrimeAndEveryLocalBeam", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
		hubforge.AddPairWith(t, h, "under-prime", fabricengine.AddOptions{SkipPush: true})
		hubforge.AddPairWith(t, h, "under-beam", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
		plantBattenRun(t, h, "under-prime")
		plantBattenRun(t, h, "under-beam")

		got, err := HubRunSlugs(h.Location)

		if err != nil {
			t.Fatalf("HubRunSlugs() error = %v; want nil", err)
		}
		if want := []string{"under-beam", "under-prime"}; !slices.Equal(got, want) {
			t.Errorf("HubRunSlugs() = %q; want %q", got, want)
		}
	})

	t.Run("The_launcher_names_the_hubs_board_directory_and_recorded_shortname", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

		got := HubCatchUp(h.Location, func() ([]string, error) { return nil, nil }).Launcher

		want := dobbyengine.Launcher{BoardDir: fabriclayout.BoardDir(h.Location.HubPath), Shortname: hubforge.TestShortname}
		if got != want {
			t.Errorf("HubCatchUp().Launcher = %+v; want %+v", got, want)
		}
	})

	t.Run("HubRunSlugsOfAHubWithNoRunListsNone", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

		got, err := HubRunSlugs(h.Location)

		if err != nil {
			t.Fatalf("HubRunSlugs() error = %v; want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("HubRunSlugs() = %q; want none", got)
		}
	})
}

// plantBattenRun writes the seed and a running status of a batten run named slug under the hub's prime.
func plantBattenRun(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()
	if err := shedrun.WriteSeed(h.Location, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed(%s) = %v; want nil", slug, err)
	}
	status, err := json.Marshal(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"})
	if err != nil {
		t.Fatalf("Marshal(status) = %v; want nil", err)
	}
	if err := os.WriteFile(shedrun.StatusFile(h.Location, slug), status, 0o644); err != nil {
		t.Fatalf("WriteFile(status of %s) = %v; want nil", slug, err)
	}
}
