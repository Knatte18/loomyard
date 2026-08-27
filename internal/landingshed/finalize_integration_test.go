//go:build integration

// finalize_integration_test.go drives Finalize against a real two-worktree pair on both the task
// side and the parent side, built by the standard hub fixture (internal/hubforge), with only the
// model seam faked: fakeResolutionShuttle stands in for a real conflict-resolution session, writing
// real content to the conflicted file rather than asking a model. No test in this file contacts a
// real service or a real model.
//
// The scenario deliberately conflicts on the warp side only -- the byte-identical warp/weft conflict
// shape is already covered exhaustively by internal/fabricengine's own mergein_integration_test.go,
// and duplicating that matrix here would couple this file to another package's own check set (see
// this batch's own decision that the integration tier here stays small and purposeful). A second,
// non-conflicting file diverges on the weft side instead, so "the weft is not a merge participant"
// (the weft-local-only-files task's own Fabric.Merge/MergeIn change) has something concrete to
// assert: the task pair's weft-only file must NOT reach the parent pair's weft.

package landingshed_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// finalizeConflictStencilFixture is a minimal, valid conflict stencil carrying exactly the two
// markers mergeresolve's own spec builder fills, mirroring mergeresolve_test.go's own fixture --
// this package's own conflict-resolution session never spawns a real model, so the stencil's prose
// content is irrelevant to this test, only its two placeholders.
const finalizeConflictStencilFixture = "# Conflict\n\nPaths:\n{{.conflicted_paths}}\n\nReport: {{.report_path}}\n"

// seedConflictStencil writes finalizeConflictStencilFixture at the layout mergeresolve's spec
// builder reads (stencilstore.Path's own baseDir/landing/<name>.md shape) and returns the baseDir a
// Deps.StencilsDir field should carry.
func seedConflictStencil(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "landing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(stencils dir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "landing-template-conflict.md"), []byte(finalizeConflictStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict stencil): %v", err)
	}
	return root
}

// fakeResolutionShuttle is the model seam this file fakes: on every Run call it overwrites each of
// paths (joined onto worktreeRoot) with resolved, marker-free content, standing in for a real
// conflict-resolution session's own file edits.
type fakeResolutionShuttle struct {
	worktreeRoot string
	paths        []string
	resolved     string
	calls        int
}

func (f *fakeResolutionShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	f.calls++
	for _, p := range f.paths {
		full := filepath.Join(f.worktreeRoot, p)
		if err := os.WriteFile(full, []byte(f.resolved), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

var _ mergeresolve.Shuttle = (*fakeResolutionShuttle)(nil)

// commitOnCurrentBranchLanding writes filename with content in dir, stages it, and commits msg on
// whatever branch is currently checked out -- this package's own local copy of
// fabricengine_test's identically-shaped helper, since the two test packages cannot share
// unexported test code.
func commitOnCurrentBranchLanding(t *testing.T, dir, filename, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", filename, err)
	}
	gitkit.MustRun(t, dir, "git", "add", filename)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", msg)
}

// openFabricAtLanding opens a *fabricengine.Fabric on the warp worktree at path, via
// lyxcwd.ResolveWorktree + fabricengine.Open -- the only production constructor either producer's
// pair-opener closures may legally wrap.
func openFabricAtLanding(t *testing.T, path string) *fabricengine.Fabric {
	t.Helper()
	l, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", path, err)
	}
	f, err := fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open(%s): %v", path, err)
	}
	return f
}

// gitParentCountLanding returns the number of parents rev has in dir, via `git log -1 --format=%P`
// -- this package's own local copy of fabricengine_test's identically-shaped helper.
func gitParentCountLanding(t *testing.T, dir, rev string) int {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%P", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%P %s in %s: %v", rev, dir, err)
	}
	return len(strings.Fields(strings.TrimSpace(string(out))))
}

// currentSHALanding returns dir's current HEAD SHA, via `git rev-parse HEAD` -- this package's own
// local stand-in for fabricengine's own test-only CurrentSHAForTest, which is not reachable from a
// different package's test binary.
func currentSHALanding(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// TestFinalize_ResolvesConflictAndSquashMergesIntoParent builds a task pair and a parent pair off
// the same hub, diverges both on conflict.txt (a genuine warp-side conflict) and diverges the task
// pair alone on a second, clean weft-side file, runs Finalize with a fake session that writes a real
// resolution to the conflicted file, and asserts that the parent pair's warp afterward carries the
// task's resolved content and the squash setting took effect there, that the parent pair's weft is
// left byte-identical -- the weft is not a merge participant, per this task's own
// no-caller-facing-signature-change-in-fabric-shaped change to Fabric.Merge -- and that no merge
// record is left behind on either pair.
func TestFinalize_ResolvesConflictAndSquashMergesIntoParent(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	hubforge.AddPair(t, h, "task")
	hubforge.AddPair(t, h, "parent")

	taskWarp, taskWeft := h.PairWarpWorktree("task"), h.PairWeftSibling("task")
	parentWarp, parentWeft := h.PairWarpWorktree("parent"), h.PairWeftSibling("parent")

	// The genuine warp-side conflict: both branches add conflict.txt independently, off a common
	// ancestor where it does not exist.
	commitOnCurrentBranchLanding(t, taskWarp, "conflict.txt", "task content\n", "task: add conflict.txt")
	commitOnCurrentBranchLanding(t, parentWarp, "conflict.txt", "parent content\n", "parent: add conflict.txt")

	// A clean, non-conflicting weft-side divergence on the task pair alone, so "the parent pair
	// carries the task's content on both sides" has a concrete weft-side fact to assert.
	commitOnCurrentBranchLanding(t, taskWeft, "task-note.txt", "task weft note\n", "task: add task-note.txt")

	scratchDir := filepath.Join(t.TempDir(), "scratch")
	shuttle := &fakeResolutionShuttle{worktreeRoot: taskWarp, paths: []string{"conflict.txt"}, resolved: "resolved content\n"}

	deps := landingshed.Deps{
		WorktreeRoot:     taskWarp,
		TaskBranch:       "task",
		ParentBranch:     "parent",
		StencilsDir:      seedConflictStencil(t),
		ScratchDir:       scratchDir,
		OpenFabric:       func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskWarp), nil },
		OpenParentFabric: func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, parentWarp), nil },
		Shuttle:          shuttle,
		Config: landingshed.Config{
			Squash:             true,
			Conflict:           "claude:test-model",
			ConflictTimeoutMin: 1,
		},
	}

	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		t.Fatalf("NewFinalize() error = %v; want nil", err)
	}

	// Captured before Call so the weft-not-a-merge-participant assertion below has a concrete
	// before/after pair to compare, mirroring internal/fabricengine's own weftBefore/weftHEAD
	// convention (see e.g. mergeweftlocal_integration_test.go).
	parentWeftBefore := currentSHALanding(t, parentWeft)

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if shuttle.calls != 1 {
		t.Errorf("fake shuttle Run() called %d time(s); want exactly 1 (one conflict-resolution session)", shuttle.calls)
	}

	// The parent pair's warp carries the task's resolved content.
	warpContent, err := os.ReadFile(filepath.Join(parentWarp, "conflict.txt"))
	if err != nil {
		t.Fatalf("read parent warp conflict.txt: %v", err)
	}
	if string(warpContent) != "resolved content\n" {
		t.Errorf("parent warp conflict.txt = %q; want the resolved content %q", warpContent, "resolved content\n")
	}

	// The parent pair's weft is left byte-identical: the weft is not a merge participant, so the
	// task pair's weft-only task-note.txt never reaches it, on either the file-tree or the commit
	// graph -- mirroring internal/fabricengine's own "weft is not a merge participant" assertion
	// shape (see e.g. mergeweftlocal_integration_test.go's weftBefore/weftHEAD byte-identical check).
	if _, err := os.Stat(filepath.Join(parentWeft, "task-note.txt")); !os.IsNotExist(err) {
		t.Errorf("os.Stat(parent weft task-note.txt) error = %v; want a not-exist error -- the weft is not a merge participant", err)
	}
	if got := currentSHALanding(t, parentWeft); got != parentWeftBefore {
		t.Errorf("parent weft HEAD = %q; want byte-identical %q -- the weft is not a merge participant", got, parentWeftBefore)
	}

	// The squash setting took effect on the parent pair's warp: a single-parent commit.
	warpHEAD := currentSHALanding(t, parentWarp)
	if got := gitParentCountLanding(t, parentWarp, warpHEAD); got != 1 {
		t.Errorf("parent warp HEAD %s has %d parents; want exactly 1 (a squash commit)", warpHEAD, got)
	}

	// No merge record is left behind on either pair. fabricengine's own MergeRecordExistsForTest is
	// a test-only export reachable only from fabricengine's own test binary, so this package checks
	// the same fact through the public MergeInProgress accessor instead.
	taskHandle := openFabricAtLanding(t, taskWarp)
	if inProgress, err := taskHandle.MergeInProgress(); err != nil || inProgress {
		t.Errorf("task pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}
	parentHandle := openFabricAtLanding(t, parentWarp)
	if inProgress, err := parentHandle.MergeInProgress(); err != nil || inProgress {
		t.Errorf("parent pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}
}
