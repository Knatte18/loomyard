//go:build integration

// adoptremote_integration_test.go drives the fabric CLI verbs that take a pair's weft branch from origin when it exists only there, and `add`'s adoption of a live pair's branches, against one real hub as an ordered scenario.
// Origin-only branches are pushed into the hub's bare from a scratch clone,
// and an unreachable origin is simulated by pointing the weft repo's origin URL at a missing path.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// originMarkerFile is the file an origin-only weft branch carries,
// so a test can tell the branch adopted from origin from one forked locally.
const originMarkerFile = "origin-marker.txt"

// pushOriginOnlyWeftBranch pushes weftBranch to the hub's weft bare from a scratch clone, carrying originMarkerFile, and returns its tip.
// With archiveTag set, it also pushes an archive/<slug>/<tip> tag covering that tip.
func pushOriginOnlyWeftBranch(t *testing.T, h *hubforge.Hub, slug, weftBranch string, archiveTag bool) string {
	t.Helper()

	clone := t.TempDir()
	gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.RecordsBare, ".")
	gitkit.MustRun(t, clone, "git", "checkout", "--quiet", "-b", weftBranch)
	tip := gitkit.CommitFile(t, clone, originMarkerFile, "from origin\n", "origin-only weft branch")
	gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)
	if archiveTag {
		tag := "archive/" + slug + "/" + tip[:12]
		gitkit.MustRun(t, clone, "git", "tag", tag, tip)
		gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", "refs/tags/"+tag)
	}
	return tip
}

// addPairWithLocalWarpBranch adds a pair for slug and creates the local warp branch branch, whose weft counterpart the caller controls.
func addPairWithLocalWarpBranch(t *testing.T, h *hubforge.Hub, slug, branch string) {
	t.Helper()

	if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
		t.Fatalf("add %s exit = %d; output: %s", slug, code, output)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", branch)
}

// requireOnOriginalBranches asserts the pair's warp and weft worktrees are still on the pair's own branches.
func requireOnOriginalBranches(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if got := gitkit.CurrentBranch(t, h.PairCodeWorktree(slug)); got != slug {
		t.Errorf("warp branch = %q; want %q (unchanged)", got, slug)
	}
	if got, want := gitkit.CurrentBranch(t, h.PairRecordsSibling(slug)), fabricengine.RecordsBranchName(slug); got != want {
		t.Errorf("weft branch = %q; want %q (unchanged)", got, want)
	}
}

// weftLockDirName is the lock directory every weft worktree carries.
const weftLockDirName = ".weft"

// addRawWarpWorktree creates a warp worktree for slug outside lyx, on a new branch slug, at the pair's warp path.
// It removes the worktree and branch again at cleanup when removeAtCleanup is set,
// so a pair left unrepairable does not fail later reconciles.
func addRawWarpWorktree(t *testing.T, h *hubforge.Hub, slug string, removeAtCleanup bool) {
	t.Helper()

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "add", "-b", slug, h.PairCodeWorktree(slug))
	if removeAtCleanup {
		t.Cleanup(func() {
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "-D", slug)
		})
	}
}

// reconcilePair runs `fabric reconcile` from the prime worktree and returns the exit code and the report of slug's pair.
func reconcilePair(t *testing.T, h *hubforge.Hub, slug string) (int, map[string]any) {
	t.Helper()

	code, output := runFabric(t, h.PrimeWorktree(), "reconcile")
	env, err := envelope.Parse(output)
	if err != nil {
		t.Fatalf("parse reconcile output: %v\noutput: %s", err, output)
	}
	pairs, _ := env.Raw["pairs"].([]any)
	for _, raw := range pairs {
		pair, _ := raw.(map[string]any)
		if warpPath, _ := pair["code_worktree"].(string); filepath.Base(warpPath) == slug {
			return code, pair
		}
	}
	t.Fatalf("reconcile report has no pair for %s\noutput: %s", slug, output)
	return code, nil
}

// requireAdoptedFromOrigin asserts slug's pair is wired to a weft worktree whose branch tracks origin and which carries its lock directory.
func requireAdoptedFromOrigin(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	weft := h.PairRecordsSibling(slug)
	weftBranch := fabricengine.RecordsBranchName(slug)
	if got := gitkit.CurrentBranch(t, weft); got != weftBranch {
		t.Errorf("weft branch = %q; want %q", got, weftBranch)
	}
	if got := gitkit.Git(t, weft, "rev-parse", "--abbrev-ref", weftBranch+"@{upstream}"); got != "origin/"+weftBranch {
		t.Errorf("upstream of %s = %q; want origin/%s", weftBranch, got, weftBranch)
	}
	if !strings.Contains(strings.Join(gitkit.LsFiles(t, weft), "\n"), originMarkerFile) {
		t.Errorf("weft worktree does not track %s; want the origin branch's content", originMarkerFile)
	}
	for _, path := range []string{
		filepath.Join(weft, weftLockDirName),
		filepath.Join(h.PairCodeWorktree(slug), h.Location.AnchorRel, lyxdirs.LyxDirName),
		h.PairPortalLink(slug),
		h.PairLauncherDir(slug),
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s missing after reconcile: %v", path, err)
		}
	}
}

// TestRunCLI_AdoptRemoteWeftScenario runs the checkout and reconcile checks over one hub.
// Steps run serially, each on its own pair;
// the steps that remove the weft repo's origin come last.
// The scenario calls t.Parallel as a whole;
// no step does, because they share the one hub.
func TestRunCLI_AdoptRemoteWeftScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"CheckoutAdoptsOriginOnlyWeftBranch", func(t *testing.T) {
			// A weft branch present only on origin is adopted as a local branch tracking it, with or without an archive tag covering its tip.
			for _, tc := range []struct {
				slug, branch string
				archiveTag   bool
			}{
				{"co-adopt", "co-adopt-b", false},
				{"co-archived", "co-archived-b", true},
			} {
				addPairWithLocalWarpBranch(t, h, tc.slug, tc.branch)
				weftBranch := fabricengine.RecordsBranchName(tc.branch)
				tip := pushOriginOnlyWeftBranch(t, h, tc.slug, weftBranch, tc.archiveTag)

				if code, output := runFabric(t, h.PairCodeWorktree(tc.slug), "checkout", tc.branch); code != 0 {
					t.Fatalf("checkout %s (archiveTag=%v) exit = %d; output: %s", tc.branch, tc.archiveTag, code, output)
				}

				weft := h.PairRecordsSibling(tc.slug)
				if got := gitkit.CurrentBranch(t, weft); got != weftBranch {
					t.Errorf("weft branch = %q; want %q", got, weftBranch)
				}
				if got := gitkit.RevParse(t, weft, "HEAD"); got != tip {
					t.Errorf("weft HEAD = %s; want origin's tip %s", got, tip)
				}
				if got := gitkit.Git(t, weft, "rev-parse", "--abbrev-ref", weftBranch+"@{upstream}"); got != "origin/"+weftBranch {
					t.Errorf("upstream of %s = %q; want origin/%s", weftBranch, got, weftBranch)
				}
				if !strings.Contains(gitkit.Git(t, weft, "ls-files"), originMarkerFile) {
					t.Errorf("weft worktree does not track %s; want the origin branch's content", originMarkerFile)
				}
			}
		}},
		{"CheckoutRefusesUnreachableOrigin", func(t *testing.T) {
			// With origin unreachable and no local weft branch, checkout stops naming origin instead of forking, leaving both sides where they were.
			const slug, branch = "co-unreachable", "co-unreachable-b"
			addPairWithLocalWarpBranch(t, h, slug, branch)

			weftRepo := h.PrimeRecords()
			gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", filepath.Join(h.Container, "missing-origin"))
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", h.RecordsBare)

			code, output := runFabric(t, h.PairCodeWorktree(slug), "checkout", branch)
			if code == 0 {
				t.Fatalf("checkout with an unreachable origin exit = 0; want non-zero\noutput: %s", output)
			}
			if !strings.Contains(output, "origin") {
				t.Errorf("output %q does not name origin", output)
			}
			requireOnOriginalBranches(t, h, slug)
			if gitkit.BranchExists(t, weftRepo, fabricengine.RecordsBranchName(branch)) {
				t.Errorf("local weft branch %q exists after the refused checkout; want none", fabricengine.RecordsBranchName(branch))
			}
		}},
		{"ReconcileAdoptsOriginWeftForRawWarp", func(t *testing.T) {
			// A raw warp worktree whose weft branch exists only on origin ends wired as weft_recreated, with or without an archive tag covering the origin tip.
			for _, tc := range []struct {
				slug       string
				archiveTag bool
			}{
				{"rc-adopt", false},
				{"rc-archived", true},
			} {
				addRawWarpWorktree(t, h, tc.slug, false)
				pushOriginOnlyWeftBranch(t, h, tc.slug, fabricengine.RecordsBranchName(tc.slug), tc.archiveTag)

				code, pair := reconcilePair(t, h, tc.slug)
				if code != 0 {
					t.Fatalf("reconcile (archiveTag=%v) exit = %d; pair: %v", tc.archiveTag, code, pair)
				}
				if got := pair["action"]; got != string(fabricengine.ReconcileActionWeftRecreated) {
					t.Errorf("action (archiveTag=%v) = %v; want %s", tc.archiveTag, got, fabricengine.ReconcileActionWeftRecreated)
				}
				requireAdoptedFromOrigin(t, h, tc.slug)
			}
		}},
		{"ReconcileAdoptsOriginWeftForManagedPair", func(t *testing.T) {
			// A managed pair whose weft worktree and local weft branch are gone adopts the branch origin keeps and has its wiring repaired.
			const slug = "rc-managed"
			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("add %s exit = %d; output: %s", slug, code, output)
			}
			weftBranch := fabricengine.RecordsBranchName(slug)
			weftSibling := h.PairRecordsSibling(slug)
			gitkit.MustRun(t, weftSibling, "git", "push", "--quiet", "origin", weftBranch)
			gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", weftSibling)
			gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "-D", weftBranch)

			code, pair := reconcilePair(t, h, slug)
			if code != 0 {
				t.Fatalf("reconcile exit = %d; pair: %v", code, pair)
			}
			if got := pair["action"]; got != string(fabricengine.ReconcileActionWeftRecreated) {
				t.Errorf("action = %v; want %s", got, fabricengine.ReconcileActionWeftRecreated)
			}
			if got := gitkit.Git(t, weftSibling, "rev-parse", "--abbrev-ref", weftBranch+"@{upstream}"); got != "origin/"+weftBranch {
				t.Errorf("upstream of %s = %q; want origin/%s", weftBranch, got, weftBranch)
			}
			for _, path := range []string{
				filepath.Join(weftSibling, weftLockDirName),
				filepath.Join(h.PairCodeWorktree(slug), h.Location.AnchorRel, lyxdirs.LyxDirName),
				h.PairPortalLink(slug),
				h.PairLauncherDir(slug),
			} {
				if _, err := os.Lstat(path); err != nil {
					t.Errorf("%s missing after reconcile: %v", path, err)
				}
			}
		}},
		{"ReconcileRefusesUnreachableOrigin", func(t *testing.T) {
			// With origin unreachable and no local weft branch, reconcile reports the pair's error naming origin instead of forking a dormant weft.
			const slug = "rc-unreachable"
			addRawWarpWorktree(t, h, slug, true)

			weftRepo := h.PrimeRecords()
			gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", filepath.Join(h.Container, "missing-origin"))
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", h.RecordsBare)

			code, pair := reconcilePair(t, h, slug)
			if code == 0 {
				t.Errorf("reconcile with an unreachable origin exit = 0; want non-zero")
			}
			if reason, _ := pair["error"].(string); !strings.Contains(reason, "origin") {
				t.Errorf("pair error = %q; want it to name origin", reason)
			}
			if gitkit.BranchExists(t, weftRepo, fabricengine.RecordsBranchName(slug)) {
				t.Errorf("local weft branch exists after the refused reconcile; want none")
			}
		}},
		{"ReconcileRollsBackBranchWhenAdoptFails", func(t *testing.T) {
			// A weft branch created from origin is deleted again when its worktree cannot be created,
			// so no local branch outlives the failure;
			// origin keeps its tip.
			const slug = "rc-rollback"
			addRawWarpWorktree(t, h, slug, true)
			weftBranch := fabricengine.RecordsBranchName(slug)
			tip := pushOriginOnlyWeftBranch(t, h, slug, weftBranch, false)

			blocker := h.PairRecordsSibling(slug)
			if err := os.WriteFile(blocker, []byte("a file where the worktree belongs"), 0o644); err != nil {
				t.Fatalf("plant blocker: %v", err)
			}
			defer os.Remove(blocker)

			code, pair := reconcilePair(t, h, slug)
			if code == 0 {
				t.Errorf("reconcile with a failing adopt exit = 0; want non-zero")
			}
			if reason, _ := pair["error"].(string); reason == "" {
				t.Errorf("pair carries no error; want the adopt failure")
			}
			if gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
				t.Errorf("local weft branch %q survives the failed adopt; want it deleted", weftBranch)
			}
			if got := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch); got != tip {
				t.Errorf("origin %s = %s; want unchanged %s", weftBranch, got, tip)
			}
		}},
		{"AddAdoptsLivePairFromOrigin", func(t *testing.T) {
			// Origin holds a live pair's warp and weft branches, with commits the hub's HEAD lacks and a run record,
			// and the hub has neither locally:
			// add builds the pair on both origin tips and pushes only fast-forwards.
			const slug = "add-live"
			weftBranch := fabricengine.RecordsBranchName(slug)
			runRecord := filepath.Join(h.Location.AnchorRel, shedrun.RunsRootRel(), "run-1", "note.txt")

			warpClone := t.TempDir()
			gitkit.MustRun(t, warpClone, "git", "clone", "--quiet", h.CodeBare, ".")
			gitkit.MustRun(t, warpClone, "git", "checkout", "--quiet", "-b", slug)
			warpTip := gitkit.CommitFile(t, warpClone, "warp-origin.txt", "from origin\n", "origin-only warp work")
			gitkit.MustRun(t, warpClone, "git", "push", "--quiet", "origin", slug)

			weftClone := t.TempDir()
			gitkit.MustRun(t, weftClone, "git", "clone", "--quiet", h.RecordsBare, ".")
			gitkit.MustRun(t, weftClone, "git", "checkout", "--quiet", "-b", weftBranch)
			gitkit.CommitFile(t, weftClone, originMarkerFile, "from origin\n", "origin-only weft work")
			weftTip := gitkit.CommitFile(t, weftClone, runRecord, "run record\n", "origin-only run record")
			gitkit.MustRun(t, weftClone, "git", "push", "--quiet", "origin", weftBranch)

			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("add %s exit = %d; output: %s", slug, code, output)
			}

			warp, weft := h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)
			if got := gitkit.RevParse(t, warp, "HEAD"); got != warpTip {
				t.Errorf("warp HEAD = %s; want origin's tip %s", got, warpTip)
			}
			if got := gitkit.RevParse(t, h.CodeBare, "refs/heads/"+slug); got != warpTip {
				t.Errorf("origin warp tip = %s; want unchanged %s", got, warpTip)
			}
			for _, tc := range []struct{ dir, branch string }{{warp, slug}, {weft, weftBranch}} {
				if got := gitkit.Git(t, tc.dir, "rev-parse", "--abbrev-ref", tc.branch+"@{upstream}"); got != "origin/"+tc.branch {
					t.Errorf("upstream of %s = %q; want origin/%s", tc.branch, got, tc.branch)
				}
			}
			if n := gitkit.RevListCount(t, weft, weftTip+"..HEAD"); n != 1 {
				t.Errorf("weft HEAD is %d commits past the scratch clone's tip; want 1 (the origin record)", n)
			}
			if got, want := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch), gitkit.RevParse(t, weft, "HEAD"); got != want {
				t.Errorf("origin weft tip = %s; want the pair's weft HEAD %s", got, want)
			}
			for _, path := range []string{filepath.Join(weft, runRecord), filepath.Join(weft, weftLockDirName)} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("%s missing after add: %v", path, err)
				}
			}
		}},
		{"CheckoutForksWithoutOriginRemote", func(t *testing.T) {
			// A weft repo with no origin remote skips the origin step and forks the branch as before.
			const slug, branch = "co-noorigin", "co-noorigin-b"
			addPairWithLocalWarpBranch(t, h, slug, branch)

			weftRepo := h.PrimeRecords()
			gitkit.MustRun(t, weftRepo, "git", "remote", "remove", "origin")
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "add", "origin", h.RecordsBare)

			if code, output := runFabric(t, h.PairCodeWorktree(slug), "checkout", branch); code != 0 {
				t.Fatalf("checkout without an origin remote exit = %d; output: %s", code, output)
			}
			weftBranch := fabricengine.RecordsBranchName(branch)
			if got := gitkit.CurrentBranch(t, h.PairRecordsSibling(slug)); got != weftBranch {
				t.Errorf("weft branch = %q; want the forked %q", got, weftBranch)
			}
			if strings.Contains(strings.Join(gitkit.LsFiles(t, h.PairRecordsSibling(slug)), "\n"), originMarkerFile) {
				t.Errorf("forked weft branch tracks %s; want a fork of the pair's own branch", originMarkerFile)
			}
		}},
		{"ReconcileForksDormantWeftWithoutOriginRemote", func(t *testing.T) {
			// A weft repo with no origin remote keeps reconcile's dormant fork for a raw warp worktree,
			// and the dormant weft carries its lock directory.
			const slug = "rc-noorigin"
			addRawWarpWorktree(t, h, slug, false)

			weftRepo := h.PrimeRecords()
			gitkit.MustRun(t, weftRepo, "git", "remote", "remove", "origin")
			defer gitkit.MustRun(t, weftRepo, "git", "remote", "add", "origin", h.RecordsBare)

			// The board shares the weft repo's remotes, so its pushes fail without an origin and the verb exits non-zero; only the pair's report is asserted here.
			_, pair := reconcilePair(t, h, slug)
			if got := pair["action"]; got != string(fabricengine.ReconcileActionRawAdopted) {
				t.Errorf("action = %v; want %s", got, fabricengine.ReconcileActionRawAdopted)
			}
			if _, err := os.Stat(filepath.Join(h.PairRecordsSibling(slug), weftLockDirName)); err != nil {
				t.Errorf("dormant weft lacks its lock directory: %v", err)
			}
		}},
	}

	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			break
		}
	}
}
