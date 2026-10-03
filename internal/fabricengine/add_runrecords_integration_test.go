//go:build integration

// add_runrecords_integration_test.go proves Add forks a new pair's weft branch without the parent's shed run records:
// the records never reach the new worktree's disk, the pair's first weft commit records their deletion,
// and the adopt path, the parent branch and Add's rollback are untouched by the drop.
//
// Package fabricengine_test to reuse hubforge.NewHub and the add_rollback_adopt_test.go helper mustWeftRepoRoot;
// it shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// runRecordFiles are the two run directories' record files every test seeds, each joined under shedrun's own run-records root.
var runRecordFiles = []string{
	filepath.Join(shedrun.RunsRootRel(), "run-a", "seed.json"),
	filepath.Join(shedrun.RunsRootRel(), "run-b", "status.json"),
}

// commitRunRecords writes runRecordFiles under l's anchor in the weft worktree at weftDir and commits them there.
func commitRunRecords(t *testing.T, l *lyxcwd.Location, weftDir string) {
	t.Helper()

	for _, rel := range runRecordFiles {
		full := filepath.Join(weftDir, l.AnchorRel, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	gitkit.Git(t, weftDir, "add", "-A")
	gitkit.Git(t, weftDir, "commit", "-m", "seed run records")
}

// trackedUnderRoot returns what branch tracks under l's run-records root, read in the repo at dir.
func trackedUnderRoot(t *testing.T, l *lyxcwd.Location, dir, branch string) string {
	t.Helper()

	root := filepath.ToSlash(filepath.Join(l.AnchorRel, shedrun.RunsRootRel()))
	return gitkit.Git(t, dir, "ls-tree", "-r", "--name-only", branch, "--", root)
}

// TestAdd_DropsParentRunRecords seeds two run directories on the parent weft branch and asserts the pair's weft branch is one commit ahead of the fork point, tracks nothing under the root, has no such directory on disk, carries the origin record, and is pushed, while the parent still tracks both.
func TestAdd_DropsParentRunRecords(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "drops-records"
	parentWeft := fabricengine.WeftBranchName("main")
	weftRoot := mustWeftRepoRoot(t, l)

	commitRunRecords(t, l, weftRoot)
	forkPoint := gitkit.RevParse(t, weftRoot, parentWeft)

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	weftBranch := fabricengine.WeftBranchName(slug)
	weftPath := fabricengine.WeftWorktreePath(l, slug)

	if got := gitkit.RevListCount(t, weftRoot, forkPoint+".."+weftBranch); got != 1 {
		t.Errorf("weft branch is %d commits ahead of the fork point; want exactly 1", got)
	}
	if got := trackedUnderRoot(t, l, weftRoot, weftBranch); got != "" {
		t.Errorf("pair weft branch still tracks run records:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(weftPath, l.AnchorRel, shedrun.RunsRootRel())); !os.IsNotExist(err) {
		t.Errorf("run-records root exists on the pair's weft disk (stat err = %v)", err)
	}
	if shown := gitShow(t, weftPath, weftBranch, filepath.ToSlash(filepath.Join(l.AnchorRel, fabricengine.OriginRecordRel()))); !strings.Contains(shown, `"parent_branch": "main"`) {
		t.Errorf("origin record = %q; want it committed with parent_branch main", shown)
	}
	if !gitkit.BranchExists(t, weftRoot, weftBranch) {
		t.Fatalf("weft branch %q missing", weftBranch)
	}
	if pushed, remote := gitkit.RevParse(t, weftRoot, "refs/remotes/origin/"+weftBranch), gitkit.RevParse(t, weftRoot, weftBranch); pushed != remote {
		t.Errorf("origin/%s = %s; want the pair's tip %s", weftBranch, pushed, remote)
	}

	if got := gitkit.RevParse(t, weftRoot, parentWeft); got != forkPoint {
		t.Errorf("parent weft tip moved: %s -> %s", forkPoint, got)
	}
	if got := trackedUnderRoot(t, l, weftRoot, parentWeft); strings.Count(got, "\n") != len(runRecordFiles)-1 || got == "" {
		t.Errorf("parent weft branch tracks %q; want both run records still tracked", got)
	}
}

// TestAdd_DropsParentRunRecords_SubpathAnchor repeats the drop in a hub anchored at a subpath, where the run-records root is joined under AnchorRel both for the fork's index drop and for the first weft commit, so the two joins must name the same tree.
func TestAdd_DropsParentRunRecords_SubpathAnchor(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, "backend")
	l := h.Location
	const slug = "drops-records-subpath"
	weftRoot := mustWeftRepoRoot(t, l)

	commitRunRecords(t, l, weftRoot)
	forkPoint := gitkit.RevParse(t, weftRoot, fabricengine.WeftBranchName("main"))

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	weftBranch := fabricengine.WeftBranchName(slug)
	if got := gitkit.RevListCount(t, weftRoot, forkPoint+".."+weftBranch); got != 1 {
		t.Errorf("weft branch is %d commits ahead of the fork point; want exactly 1", got)
	}
	if got := trackedUnderRoot(t, l, weftRoot, weftBranch); got != "" {
		t.Errorf("pair weft branch still tracks run records:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, shedrun.RunsRootRel())); !os.IsNotExist(err) {
		t.Errorf("run-records root exists on the pair's weft disk (stat err = %v)", err)
	}
}

// TestAdd_NoRunRecordsToDrop asserts a parent tracking nothing under the root gives the same single origin-record commit as before the drop existed.
func TestAdd_NoRunRecordsToDrop(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const slug = "nothing-to-drop"
	weftRoot := mustWeftRepoRoot(t, l)
	forkPoint := gitkit.RevParse(t, weftRoot, fabricengine.WeftBranchName("main"))

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	weftBranch := fabricengine.WeftBranchName(slug)
	if got := gitkit.RevListCount(t, weftRoot, forkPoint+".."+weftBranch); got != 1 {
		t.Errorf("weft branch is %d commits ahead of the fork point; want exactly 1", got)
	}
	out, err := gitexec.Run([]string{"diff", "--name-status", forkPoint, weftBranch}, weftRoot)
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "A\t") {
		t.Errorf("fork-to-tip diff = %q; want only the added origin record", out)
	}
}

// TestAdd_AdoptKeepsRunRecords asserts adopting an existing weft branch that tracks run records leaves them tracked and on disk.
func TestAdd_AdoptKeepsRunRecords(t *testing.T) {
	t.Parallel()

	const slug = "adopt-keeps-records"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftBranch := fabricengine.WeftBranchName(slug)
	weftRoot := mustWeftRepoRoot(t, l)

	seedDir := filepath.Join(t.TempDir(), "seed")
	gitkit.MustRun(t, weftRoot, "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.WeftBranchName("main"))
	commitRunRecords(t, l, seedDir)
	gitkit.MustRun(t, weftRoot, "git", "worktree", "remove", seedDir)

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	if got := trackedUnderRoot(t, l, weftRoot, weftBranch); strings.Count(got, "\n") != len(runRecordFiles)-1 || got == "" {
		t.Errorf("adopted branch tracks %q; want both run records", got)
	}
	for _, rel := range runRecordFiles {
		if _, err := os.Stat(filepath.Join(fabricengine.WeftWorktreePath(l, slug), l.AnchorRel, rel)); err != nil {
			t.Errorf("adopted run record missing on disk: %v", err)
		}
	}
}

// TestAdd_FromTaskPairDropsParentRunRecords asserts adding from a non-main parent whose own weft branch tracks run records drops them from the child.
func TestAdd_FromTaskPairDropsParentRunRecords(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	const parentSlug, childSlug = "task-parent", "task-child"
	weftRoot := mustWeftRepoRoot(t, l)

	if _, err := h.Topology.Add(l, parentSlug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", parentSlug, err)
	}
	commitRunRecords(t, l, fabricengine.WeftWorktreePath(l, parentSlug))
	if got := trackedUnderRoot(t, l, weftRoot, fabricengine.WeftBranchName(parentSlug)); got == "" {
		t.Fatalf("setup: parent pair's weft branch tracks no run records")
	}

	parentL, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, parentSlug))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(parent pair): %v", err)
	}
	if _, err := h.Topology.Add(parentL, childSlug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) from the task pair: %v", childSlug, err)
	}

	if got := trackedUnderRoot(t, l, weftRoot, fabricengine.WeftBranchName(childSlug)); got != "" {
		t.Errorf("child weft branch still tracks run records:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(fabricengine.WeftWorktreePath(l, childSlug), l.AnchorRel, shedrun.RunsRootRel())); !os.IsNotExist(err) {
		t.Errorf("run-records root exists on the child's weft disk (stat err = %v)", err)
	}
}

// TestAdd_DropRollsBackFully injects a failure after the drop and asserts the pair is rolled back completely and the parent weft branch's tip is unchanged.
func TestAdd_DropRollsBackFully(t *testing.T) {
	t.Parallel()

	const slug = "drop-rollback"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	parentWeft := fabricengine.WeftBranchName("main")

	commitRunRecords(t, l, weftRoot)
	tipBefore := gitkit.RevParse(t, weftRoot, parentWeft)

	portalLink := filepath.Join(fabricengine.PortalsDir(l), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
		t.Fatalf("mkdir portal parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	if _, err := h.Topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	if _, err := os.Stat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree still exists after rollback (stat err = %v)", err)
	}
	if gitkit.BranchExists(t, weftRoot, fabricengine.WeftBranchName(slug)) {
		t.Errorf("weft branch %q survived the rollback", fabricengine.WeftBranchName(slug))
	}
	if got := gitkit.RevParse(t, weftRoot, parentWeft); got != tipBefore {
		t.Errorf("parent weft tip moved: %s -> %s", tipBefore, got)
	}
}
