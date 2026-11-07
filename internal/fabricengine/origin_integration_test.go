// origin_integration_test.go covers the origin record that Add writes, end-to-end.
// It checks the record itself, its commit on the weft branch, and rollback on the created and adopted paths.
// It also checks the mutation-record entries and the run launcher.
//
// It is package fabricengine_test so it can import hubforge, which an in-package test cannot import.
// It shares the single TestMain in testmain_test.go.

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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gitShow returns the content of path at rev in dir, and fails the test on any git error.
func gitShow(t *testing.T, dir, rev, path string) string {
	t.Helper()

	out, err := gitexec.Run([]string{"show", rev + ":" + path}, dir)
	if err != nil {
		t.Fatalf("git show %s:%s in %s: %v", rev, path, dir, err)
	}
	return out
}

// TestAdd_OriginRecord builds one hub and runs Add against it, then checks four things in order.
// The origin record is committed on the new pair's weft branch, present in that branch's tree and not only on disk.
// Add records the acting worktree's name as parent_worktree.
// It is the prime's name when added from the prime, and the first pair's slug when added from inside that pair.
// Concurrent record commits on one weft worktree serialize on the weft write lock: both land.
// Neither corrupts the other's index, and the weft history stays linear.
// Add records the acting warp worktree's current branch as parent_branch, not always the hub's default branch.
// The last check moves the prime warp worktree onto a non-default branch, so it runs last.
func TestAdd_OriginRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	t.Run("record is committed on the weft branch", func(t *testing.T) {
		const slug = "record-committed"

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

		weftBranch := fabricengine.RecordsBranchName(slug)
		weftPath := fabricengine.RecordsWorktreePath(l, slug)
		gitRelPath := filepath.ToSlash(filepath.Join(l.AnchorRel, fabricengine.OriginRecordRel()))

		shown := gitShow(t, weftPath, weftBranch, gitRelPath)
		if !strings.Contains(shown, `"parent_branch": "main"`) {
			t.Errorf("git show %s:%s = %q; want it to contain the committed parent_branch content", weftBranch, gitRelPath, shown)
		}

		subject, err := gitexec.Run([]string{"log", "-1", "--format=%s", weftBranch}, weftPath)
		if err != nil {
			t.Fatalf("git log -1 %s in %s: %v", weftBranch, weftPath, err)
		}
		subject = strings.TrimSpace(subject)
		if want := fabricengine.OriginRecordSubject(slug); subject != want {
			t.Errorf("records commit subject = %q; want %q", subject, want)
		}
		if want := "fabric: record parent branch for " + slug; subject != want {
			t.Errorf("records commit subject = %q; want the literal %q", subject, want)
		}
	})

	t.Run("parent worktree is the acting worktree's name", func(t *testing.T) {
		const first = "first-pair"
		const second = "second-pair"

		hubforge.AddPairWith(t, h, first, fabricengine.AddOptions{SkipPush: true})
		firstLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, first))
		if err != nil {
			t.Fatalf("lymcwd.Resolve(first pair): %v", err)
		}
		if _, err := h.Topology.Add(firstLayout, second, fabricengine.AddOptions{SkipPush: true}); err != nil {
			t.Fatalf("Add(%q) from the first pair: %v", second, err)
		}

		for _, tc := range []struct{ slug, want string }{
			{first, l.WorktreeName},
			{second, first},
		} {
			layout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, tc.slug))
			if err != nil {
				t.Fatalf("lymcwd.Resolve(%q): %v", tc.slug, err)
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

	t.Run("concurrent CommitRecordsPaths calls serialize", func(t *testing.T) {
		const slug = "commit-lock-race"

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weftPath := fabricengine.RecordsWorktreePath(l, slug)
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
				_, committed, err := fabricengine.CommitRecordsPaths(rec, weftPath, l.AnchorRel, []string{name}, "concurrent commit "+name, fabricengine.SyncOptions{})
				outcomes <- outcome{committed: committed, err: err}
			}()
		}

		landed := 0
		for range names {
			got := <-outcomes
			if got.err != nil {
				t.Fatalf("CommitRecordsPaths() error = %v", got.err)
			}
			if got.committed {
				landed++
			}
		}
		if landed != len(names) {
			t.Errorf("landed commit count = %d; want %d (both concurrent CommitRecordsPaths calls should land, serialized rather than raced away)", landed, len(names))
		}

		after := gitkit.RevListCount(t, weftPath, "HEAD")
		if after != before+len(names) {
			t.Errorf("weft commit count after = %d; want %d (%d before plus %d landed)", after, before+len(names), before, len(names))
		}
		merges := gitkit.RevListCount(t, weftPath, "--merges", "HEAD")
		if merges != 0 {
			t.Errorf("weft merge commit count = %d; want 0 (concurrent CommitRecordsPaths calls should serialize into a linear history, never race into a merge)", merges)
		}
	})

	t.Run("parent branch is the acting worktree's current branch", func(t *testing.T) {
		const parentBranch = "feature-parent"
		const slug = "non-default-parent"

		// Create the parent's weft branch before the prime warp worktree leaves "main":
		// the new pair's weft branch forks from it, and creating it with git branch needs no worktree of its own.
		gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", fabricengine.RecordsBranchName(parentBranch), fabricengine.RecordsBranchName("main"))

		// Move the prime warp worktree onto the non-default branch Add will read via rev-parse
		// --abbrev-ref HEAD.
		gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", parentBranch)

		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

		// Resolve a Location at the new pair's own warp worktree, as an operator inside the new pair would.
		// ReadOrigin reads through this acting worktree.
		pairLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
		if err != nil {
			t.Fatalf("lymcwd.Resolve(pair warp worktree): %v", err)
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
// inside the new pair's weft worktree, and not at the weft worktree root.
// A "." anchor would put both paths at the root, so only a subdirectory anchor tells them apart.
//
//testtiming:keep the origin record landing at the anchor-relative path and not at the weft root, with parent_branch main; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_RecordsParentBranch_SubpathAnchoredHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"})
	l := h.Location
	const slug = "subpath-parent"

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	wantPath := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, fabricengine.OriginRecordRel())
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("origin record missing at the anchor-relative path %s: %v", wantPath, err)
	}

	rootPath := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), fabricengine.OriginRecordRel())
	if _, err := os.Stat(rootPath); err == nil {
		t.Errorf("origin record also landed at the weft worktree root %s; want it only at the anchor-relative path", rootPath)
	}

	pairLayout, err := lymcwd.Resolve(filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(pair warp worktree): %v", err)
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

// TestAddRollback_CreatedPathLeavesNoOriginRecord refuses the records commit, so Add fails after writing the record.
// Rollback removes the weft worktree and branch on the created-branch path, and with them the record:
// nothing survives to be read back.
func TestAddRollback_CreatedPathLeavesNoOriginRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "created-path-no-record"
	weftBranch := fabricengine.RecordsBranchName(slug)

	refuseRecordsCommits(t, l)

	_, err := h.Topology.Add(l, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add(%q) should have failed (records commit refused)", slug)
	}
	if !strings.Contains(err.Error(), "commit parent branch record") {
		t.Fatalf("Add(%q) error = %v; want the record commit failure, so rollback runs after the record is written", slug, err)
	}

	if gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), weftBranch) {
		t.Errorf("weft branch %q survived rollback on the created-branch path; want it (and the record commit it carried) removed", weftBranch)
	}
	if _, err := os.Stat(fabricengine.RecordsWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s after rollback", fabricengine.RecordsWorktreePath(l, slug))
	}
	if _, err := os.Stat(fabricengine.WorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("warp worktree dir still exists at %s after rollback", fabricengine.WorktreePath(l, slug))
	}
}

// TestAdd_AdoptedWeftKeepsItsOriginRecord adopts a live pair's weft branch from a worktree on another branch.
// The branch already carries the origin record, which keeps its recorded parent and gets no new record commit,
// and the new weft worktree still gets its .weft lock directory.
func TestAdd_AdoptedWeftKeepsItsOriginRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "moved-pair"
	weftBranch := fabricengine.RecordsBranchName(slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	recordedTip := gitkit.RevParse(t, mustRecordsRepoRoot(t, l), "refs/remotes/origin/"+weftBranch)
	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", weftBranch, recordedTip)
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "another-parent")

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	pairLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(pair warp worktree): %v", err)
	}
	origin, ok, err := fabricengine.ReadOrigin(pairLayout)
	if err != nil || !ok {
		t.Fatalf("ReadOrigin() = (ok %v, err %v); want the adopted branch's own record", ok, err)
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOrigin().ParentBranch = %q; want the recorded %q, not the acting worktree's branch", origin.ParentBranch, "main")
	}
	if got := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(l, slug), "HEAD"); got != recordedTip {
		t.Errorf("adopted weft HEAD = %s; want the recorded tip %s (no record commit)", got, recordedTip)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.RecordsWorktreePath(l, slug), ".weft")); err != nil {
		t.Errorf("adopted weft worktree lacks its .weft lock directory: %v", err)
	}
	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	for _, m := range res.Mutations.Entries() {
		if (m.Kind == fabricengine.KindFileWritten && strings.Contains(recordPath, m.Target)) || m.Kind == fabricengine.KindCommitCreated {
			t.Errorf("record carries %s at %s; want the origin record neither rewritten nor committed", m.Kind, m.Target)
		}
	}
}

// TestAdd_AdoptedWeftTakesTheBeamMarkOnly adopts a live pair's weft branch with AddOptions.Beam set.
// The adoption runs from a worktree on another branch.
// The branch carries an unmarked origin record.
// Add gives it the beam mark in a record commit, and the record keeps its recorded parent.
func TestAdd_AdoptedWeftTakesTheBeamMarkOnly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "marked-on-adopt"
	weftBranch := fabricengine.RecordsBranchName(slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	recordedTip := gitkit.RevParse(t, mustRecordsRepoRoot(t, l), "refs/remotes/origin/"+weftBranch)
	if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", weftBranch, recordedTip)
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "another-parent")

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{Beam: true}); err != nil {
		t.Fatalf("Add(%q, Beam): %v", slug, err)
	}

	origin, found, err := fabricengine.ReadOriginFor(l, slug)
	if err != nil || !found {
		t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want the adopted branch's record", slug, origin, found, err)
	}
	if !origin.Beam {
		t.Errorf("ReadOriginFor(%q).Beam = false; want the beam mark", slug)
	}
	if origin.ParentBranch != "main" {
		t.Errorf("ReadOriginFor(%q).ParentBranch = %q; want the recorded %q", slug, origin.ParentBranch, "main")
	}
	if got := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(l, slug), "HEAD"); got == recordedTip {
		t.Errorf("adopted weft HEAD = %s; want a record commit past the recorded tip", got)
	}
}

// TestAdd_OriginRecordMutationEntries asserts the mutation snapshot of a successful Add.
// It holds exactly one KindFileWritten entry for the record and one KindCommitCreated entry for its commit.
// The commit entry's target is the new pair's weft worktree, and its detail is the sha the commit landed at.
// It is the only guard on the commit:
// AssertRecordMatchesDiff classifies KindCommitCreated as a git-state kind and exempts it from the commission direction.
func TestAdd_OriginRecordMutationEntries(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const slug = "record-mutation-entries"

	res, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	recordPath := fabricengine.OriginRecordPathFor(l, slug)
	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	weftBranch := fabricengine.RecordsBranchName(slug)
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

// TestAdd_SkipGitWritesRecordWithoutCommit asserts that an Add run with opts.SkipGit set writes the origin record.
// The run still carries its KindFileWritten entry, but it lands no commit and carries no KindCommitCreated entry.
// The test guards that no commit is claimed on a run that performs none.
func TestAdd_SkipGitWritesRecordWithoutCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
		t.Errorf("KindCommitCreated entries = %d; want 0 under SkipGit (CommitRecordsPaths must return before staging or recording anything)", commitCreatedCount)
	}
}

// TestAdd_RunLauncherLifecycle asserts that the run launcher exists in the per-slug launcher directory after `add`.
// After the matching `remove`, neither the launcher nor the launcher directory survives.
// Removal succeeds rather than being refused.
func TestAdd_RunLauncherLifecycle(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
