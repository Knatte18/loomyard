//go:build integration

// origin_integration_test.go covers Add's newly-landed provenance step end-to-end: the origin record
// itself, its commit on the weft branch, its rollback behavior on both the created- and
// adopted-weft-branch paths, its mutation-record entries (including the exemption the git-state
// commit kind carries), and the run launcher that lands alongside it in the same batch.
//
// Package fabricengine_test to reuse hubforge.NewHub and the add_rollback_adopt_test.go helpers
// (mustWeftRepoRoot); shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// gitShow returns the content git show'd at rev:path in dir, failing the test on any git error.
func gitShow(t *testing.T, dir, rev, path string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"show", rev + ":" + path}, dir)
	if err != nil {
		t.Fatalf("git show %s:%s in %s: %v", rev, path, dir, err)
	}
	return out
}

// TestAdd_OriginRecord builds one hub and runs Add against it, in this order:
// the origin record is committed on the new pair's weft branch — present in that branch's tree, not
// merely sitting on disk in the working copy;
// Add records the acting worktree's name as parent_worktree, the prime's name for a pair added from
// the prime and the first pair's slug for a pair added from inside that pair;
// two concurrent CommitWeftPaths calls against the same weft worktree serialize on the weft write
// lock rather than racing unlocked: both land, neither corrupts the other's index, and the resulting
// weft history is linear — driven the way TestCommitLock_WarpOnlySerializesConcurrentCommits drives
// its own contention case, since CommitWeftPaths and ensureWeftLockDirAt's lock path are unexported
// and this package cannot import hubforge from inside package fabricengine to reuse that test's own
// fixture directly;
// and Add records the acting warp worktree's actual current branch as parent_branch, not always the
// hub's default branch.
// The last step checks the prime warp worktree out onto a non-default branch, so it runs last.
func TestAdd_OriginRecord(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location

	t.Run("record is committed on the weft branch", func(t *testing.T) {
		const slug = "record-committed"

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

		weftBranch := fabricengine.WeftBranchName(slug)
		weftPath := fabricengine.WeftWorktreePath(l, slug)
		gitRelPath := filepath.ToSlash(filepath.Join(l.AnchorRel, fabricengine.OriginRecordRel()))

		shown := gitShow(t, weftPath, weftBranch, gitRelPath)
		if !strings.Contains(shown, `"parent_branch": "main"`) {
			t.Errorf("git show %s:%s = %q; want it to contain the committed parent_branch content", weftBranch, gitRelPath, shown)
		}
	})

	t.Run("parent worktree is the acting worktree's name", func(t *testing.T) {
		const first = "first-pair"
		const second = "second-pair"

		hubforge.AddPairWith(t, h, first, fabricengine.AddOptions{SkipPush: true})
		firstLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, first))
		if err != nil {
			t.Fatalf("lyxcwd.Resolve(first pair): %v", err)
		}
		if _, err := h.Topology.Add(firstLayout, second, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("Add(%q) from the first pair: %v", second, err)
		}

		for _, tc := range []struct{ slug, want string }{
			{first, l.WorktreeName},
			{second, first},
		} {
			layout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, tc.slug))
			if err != nil {
				t.Fatalf("lyxcwd.Resolve(%q): %v", tc.slug, err)
			}
			origin, ok, err := fabricengine.ReadOrigin(layout)
			if err != nil || !ok {
				t.Fatalf("ReadOrigin(%q) = ok %v, err %v; want a record", tc.slug, ok, err)
			}
			if origin.ParentWorktree != tc.want {
				t.Errorf("%q ParentWorktree = %q; want %q", tc.slug, origin.ParentWorktree, tc.want)
			}
		}
	})

	t.Run("concurrent CommitWeftPaths calls serialize", func(t *testing.T) {
		const slug = "commit-lock-race"

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weftPath := fabricengine.WeftWorktreePath(l, slug)
		before := gitkit.RevListCount(t, weftPath, "HEAD")

		names := [2]string{"race-a.txt", "race-b.txt"}
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(weftPath, l.AnchorRel, name), []byte("race\n"), 0o644); err != nil {
				t.Fatalf("seed %s: %v", name, err)
			}
		}

		type outcome struct {
			committed bool
			err       error
		}
		outcomes := make(chan outcome, len(names))
		for _, name := range names {
			name := name
			go func() {
				rec := fabricengine.NewMutations(l.HubPath)
				_, committed, err := fabricengine.CommitWeftPaths(rec, weftPath, l.AnchorRel, []string{name}, "concurrent commit "+name, fabricengine.SyncOptions{})
				outcomes <- outcome{committed: committed, err: err}
			}()
		}

		landed := 0
		for range names {
			got := <-outcomes
			if got.err != nil {
				t.Fatalf("CommitWeftPaths() error = %v", got.err)
			}
			if got.committed {
				landed++
			}
		}
		if landed != len(names) {
			t.Errorf("landed commit count = %d; want %d (both concurrent CommitWeftPaths calls should land, serialized rather than raced away)", landed, len(names))
		}

		after := gitkit.RevListCount(t, weftPath, "HEAD")
		if after != before+len(names) {
			t.Errorf("weft commit count after = %d; want %d (%d before plus %d landed)", after, before+len(names), before, len(names))
		}
		merges := gitkit.RevListCount(t, weftPath, "--merges", "HEAD")
		if merges != 0 {
			t.Errorf("weft merge commit count = %d; want 0 (concurrent CommitWeftPaths calls should serialize into a linear history, never race into a merge)", merges)
		}
	})

	t.Run("parent branch is the acting worktree's current branch", func(t *testing.T) {
		const parentBranch = "feature-parent"
		const slug = "non-default-parent"

		// Give the weft side a branch to fork from before the warp side ever leaves "main": the weft-side
		// branch existing is what createWeftWorktree's fork-from-parent-weft-branch step needs, and
		// creating it here (rather than via a checkout) needs no worktree of its own.
		gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "branch", fabricengine.WeftBranchName(parentBranch), fabricengine.WeftBranchName("main"))

		// Move the prime warp worktree onto the non-default branch Add will read via rev-parse
		// --abbrev-ref HEAD.
		gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", parentBranch)

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

		// Resolve a Location at the new pair's own warp worktree — the acting worktree ReadOrigin reads
		// through, mirroring how an operator who cd's into the new pair would read it back.
		pairLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
		if err != nil {
			t.Fatalf("lyxcwd.Resolve(pair warp worktree): %v", err)
		}
		origin, ok, err := fabricengine.ReadOrigin(pairLayout)
		if err != nil {
			t.Fatalf("ReadOrigin() error = %v", err)
		}
		if !ok {
			t.Fatalf("ReadOrigin() ok = false; want a record written by Add")
		}
		if origin.ParentBranch != parentBranch {
			t.Errorf("ReadOrigin().ParentBranch = %q; want %q", origin.ParentBranch, parentBranch)
		}
	})
}

// TestAdd_RecordsParentBranch_SubpathAnchoredHub proves the record lands at the anchor-relative path
// inside the new pair's weft worktree, not at the weft worktree root — the case a "." anchor cannot
// distinguish.
//
//testtiming:keep the origin record landing at the anchor-relative path and not at the weft root, with parent_branch main; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_RecordsParentBranch_SubpathAnchoredHub(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, "backend")
	l := h.Location
	const slug = "subpath-parent"

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	wantPath := filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, fabricengine.OriginRecordRel())
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("origin record missing at the anchor-relative path %s: %v", wantPath, err)
	}

	rootPath := filepath.Join(fabricengine.WeftWorktreePath(l, slug), fabricengine.OriginRecordRel())
	if _, err := os.Stat(rootPath); err == nil {
		t.Errorf("origin record also landed at the weft worktree root %s; want it only at the anchor-relative path", rootPath)
	}

	pairLayout, err := lyxcwd.Resolve(filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(pair warp worktree): %v", err)
	}
	origin, ok, err := fabricengine.ReadOrigin(pairLayout)
	if err != nil {
		t.Fatalf("ReadOrigin() error = %v", err)
	}
	if !ok {
		t.Fatalf("ReadOrigin() ok = false; want a record written by Add")
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOrigin().ParentBranch = %q; want %q", origin.ParentBranch, "main")
	}
}

// TestAddRollback_CreatedPathLeavesNoOriginRecord forces Add to fail after the record has been
// written and committed (a broken origin remote fails the nearest later step, the warp push) on the
// created-branch path, and asserts rollback's existing weft-worktree-and-branch removal takes the
// record and its commit with it: nothing survives to be read back.
func TestAddRollback_CreatedPathLeavesNoOriginRecord(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "created-path-no-record"
	weftBranch := fabricengine.WeftBranchName(slug)

	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote"))

	// SkipPush skips Add's pre-flight probes of origin, which would otherwise refuse on the broken URL before any mutation;
	// step 11's warp push ignores SkipPush, so it still fails after the record step.
	_, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err == nil {
		t.Fatalf("Add(%q) should have failed (broken origin remote)", slug)
	}
	if !strings.Contains(err.Error(), "push branch") {
		t.Fatalf("Add(%q) error = %v; want the step-11 push failure, so rollback runs after the record step", slug, err)
	}

	if gitkit.BranchExists(t, mustWeftRepoRoot(t, l), weftBranch) {
		t.Errorf("weft branch %q survived rollback on the created-branch path; want it (and the record commit it carried) removed", weftBranch)
	}
	if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s after rollback", fabricengine.WeftWorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s after rollback", fabricengine.WorktreePath(l, slug))
	}
}

// TestAddRollback_AdoptedPathPreservesOriginRecordCommit forces the same post-record-step failure
// against an adopted pre-existing weft branch, and asserts the rollback's deliberate
// !weftBranchAdopted guard leaves the record's own commit in place on that branch — on top of the
// pre-existing history, which survives untouched alongside it.
func TestAddRollback_AdoptedPathPreservesOriginRecordCommit(t *testing.T) {
	t.Parallel()

	const slug = "adopted-path-keeps-record"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftBranch := fabricengine.WeftBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add, exactly as
	// TestAddRollback_AdoptedWeftBranchSurvives does.
	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.WeftBranchName("main"))
	gitkit.CommitFile(t, seedDir, "precious.txt", "pre-existing weft work\n", "precious pre-existing weft work")
	preciousSHA := gitkit.RevParse(t, seedDir, "HEAD")
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "worktree", "remove", seedDir)

	// Break the warp origin remote so the push (the nearest step after the record's write-and-commit
	// step) fails, triggering rollbackAdd — the same injection
	// TestAddRollback_UnwiresJunctionsOnPostWiringFailure uses for the identical adopted-branch
	// post-wiring failure shape.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote"))

	// SkipPush skips Add's pre-flight probes of origin, which would otherwise refuse on the broken URL before any mutation;
	// step 11's warp push ignores SkipPush, so it still fails after creation.
	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add(%q) should have failed (broken origin remote)", slug)
	}

	weftRoot := mustWeftRepoRoot(t, l)
	if !gitkit.BranchExists(t, weftRoot, weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}

	headSHA := gitkit.RevParse(t, weftRoot, "refs/heads/"+weftBranch)
	if headSHA == preciousSHA {
		t.Fatalf("adopted weft branch %q HEAD = %s (the pre-existing commit); want the record's own commit retained on top of it", weftBranch, headSHA)
	}
	parentSHA := gitkit.RevParse(t, weftRoot, "refs/heads/"+weftBranch+"^")
	if parentSHA != preciousSHA {
		t.Errorf("adopted weft branch %q's retained commit's parent = %s; want the pre-existing commit %s", weftBranch, parentSHA, preciousSHA)
	}

	gitRelPath := filepath.ToSlash(filepath.Join(l.AnchorRel, fabricengine.OriginRecordRel()))
	shown := gitShow(t, weftRoot, weftBranch, gitRelPath)
	if !strings.Contains(shown, `"parent_branch": "main"`) {
		t.Errorf("git show %s:%s = %q; want the retained commit to carry the origin record content", weftBranch, gitRelPath, shown)
	}

	// The pre-existing history survives untouched alongside the retained record commit.
	precious := gitShow(t, weftRoot, weftBranch, "precious.txt")
	if !strings.Contains(precious, "pre-existing weft work") {
		t.Errorf("git show %s:precious.txt = %q; want the pre-existing file content preserved", weftBranch, precious)
	}
}

// TestAdd_AdoptedWeftKeepsItsOriginRecord adopts a live pair's weft branch, which already carries the origin record, from a worktree on another branch:
// the record keeps its recorded parent, no record commit lands on the branch, and the new weft worktree still gets its .weft lock directory.
func TestAdd_AdoptedWeftKeepsItsOriginRecord(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "moved-pair"
	weftBranch := fabricengine.WeftBranchName(slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	recordedTip := gitkit.RevParse(t, mustWeftRepoRoot(t, l), "refs/remotes/origin/"+weftBranch)
	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	gitkit.MustRun(t, mustWeftRepoRoot(t, l), "git", "branch", weftBranch, recordedTip)
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "another-parent")

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	pairLayout, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(pair warp worktree): %v", err)
	}
	origin, ok, err := fabricengine.ReadOrigin(pairLayout)
	if err != nil || !ok {
		t.Fatalf("ReadOrigin() = (ok %v, err %v); want the adopted branch's own record", ok, err)
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOrigin().ParentBranch = %q; want the recorded %q, not the acting worktree's branch", origin.ParentBranch, "main")
	}
	if got := gitkit.RevParse(t, fabricengine.WeftWorktreePath(l, slug), "HEAD"); got != recordedTip {
		t.Errorf("adopted weft HEAD = %s; want the recorded tip %s (no record commit)", got, recordedTip)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.WeftWorktreePath(l, slug), ".weft")); err != nil {
		t.Errorf("adopted weft worktree lacks its .weft lock directory: %v", err)
	}
	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	for _, m := range res.Mutations.Entries() {
		if (m.Kind == fabricengine.KindFileWritten && strings.Contains(recordPath, m.Target)) || m.Kind == fabricengine.KindCommitCreated {
			t.Errorf("record carries %s at %s; want the origin record neither rewritten nor committed", m.Kind, m.Target)
		}
	}
}

// TestAdd_OriginRecordMutationEntries asserts the successful AddResult's mutation snapshot contains
// exactly one KindFileWritten entry for the record and exactly one KindCommitCreated entry for the
// record's commit, whose target is the new pair's weft worktree and whose detail is the sha the
// commit landed at — the only guard the commit entry has, since the live-state mutation oracle
// classifies KindCommitCreated as a git-state kind and exempts it from the commission direction
// entirely.
func TestAdd_OriginRecordMutationEntries(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "record-mutation-entries"

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	weftPath := fabricengine.WeftWorktreePath(l, slug)
	weftBranch := fabricengine.WeftBranchName(slug)
	wantSHA := gitkit.RevParse(t, weftPath, "refs/heads/"+weftBranch)

	var fileWrittenCount, commitCreatedCount int
	for _, m := range res.Mutated().Entries() {
		switch m.Kind {
		case fabricengine.KindFileWritten:
			if strings.HasSuffix(recordPath, m.Target) || strings.Contains(recordPath, m.Target) {
				fileWrittenCount++
			}
		case fabricengine.KindCommitCreated:
			if strings.Contains(weftPath, m.Target) || strings.Contains(m.Target, filepath.Base(weftPath)) {
				commitCreatedCount++
				if m.Detail != wantSHA {
					t.Errorf("KindCommitCreated entry detail = %q; want the record commit sha %q", m.Detail, wantSHA)
				}
			}
		}
	}
	if fileWrittenCount != 1 {
		t.Errorf("KindFileWritten entries naming the origin record = %d; want exactly 1", fileWrittenCount)
	}
	if commitCreatedCount != 1 {
		t.Errorf("KindCommitCreated entries naming the new pair's weft worktree = %d; want exactly 1", commitCreatedCount)
	}
}

// TestAdd_SkipGitWritesRecordWithoutCommit asserts that an Add run with opts.SkipGit set still
// writes the origin record and still carries its KindFileWritten entry, but lands no commit and
// carries no KindCommitCreated entry — the guard that the commit is never claimed on a run that
// deliberately performed none.
func TestAdd_SkipGitWritesRecordWithoutCommit(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "skip-git-record-only"

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true, SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("origin record missing at %s: %v", recordPath, err)
	}

	var fileWrittenCount, commitCreatedCount int
	for _, m := range res.Mutated().Entries() {
		switch m.Kind {
		case fabricengine.KindFileWritten:
			if strings.Contains(recordPath, m.Target) {
				fileWrittenCount++
			}
		case fabricengine.KindCommitCreated:
			commitCreatedCount++
		}
	}
	if fileWrittenCount != 1 {
		t.Errorf("KindFileWritten entries naming the origin record = %d; want exactly 1 even under SkipGit", fileWrittenCount)
	}
	if commitCreatedCount != 0 {
		t.Errorf("KindCommitCreated entries = %d; want 0 under SkipGit (CommitWeftPaths must return before staging or recording anything)", commitCreatedCount)
	}
}

// TestAdd_RunLauncherLifecycle asserts the run launcher this batch adds exists in the per-slug
// launcher directory after `add`, and that after the matching `remove` neither it nor the launcher
// directory survives, with removal succeeding rather than being refused.
func TestAdd_RunLauncherLifecycle(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "run-launcher-lifecycle"

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}
	runPath := filepath.Join(fabricengine.LauncherDir(l, slug), "run"+ext)
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("run launcher missing at %s after add: %v", runPath, err)
	}

	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("Remove(%q): %v", slug, err)
	}

	if _, err := os.Stat(runPath); !os.IsNotExist(err) {
		t.Errorf("run launcher still present at %s after remove", runPath)
	}
	if _, err := os.Stat(fabricengine.LauncherDir(l, slug)); !os.IsNotExist(err) {
		t.Errorf("launcher dir %s still present after remove", fabricengine.LauncherDir(l, slug))
	}
}
