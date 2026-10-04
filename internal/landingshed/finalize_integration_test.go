//go:build integration

// finalize_integration_test.go drives Finalize against a real two-worktree pair on both the task
// side and the parent side, built by the standard hub fixture (internal/hubforge), with only the
// model seam faked: fakeResolutionShuttle stands in for a real conflict-resolution session, writing
// real content to the conflicted file rather than asking a model. No test in this file contacts a
// real service or a real model.
//
// The scenario deliberately conflicts on the code side only -- the byte-identical conflict shape
// across both sides is already covered exhaustively by internal/fabricengine's own mergein_integration_test.go,
// and duplicating that matrix here would couple this file to another package's own check set (see
// this batch's own decision that the integration tier here stays small and purposeful). A second,
// non-conflicting file diverges on the records side instead, so "the records side is not a merge participant"
// (the records-local-only-files task's own Fabric.Merge/MergeIn change) has something concrete to
// assert: the task pair's records-only file must NOT reach the parent pair's records side.

package landingshed_test

import (
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
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
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

// resolutionShuttle is the model seam this file fakes: on every Run call it overwrites each of
// paths (joined onto worktreeRoot) with resolved, marker-free content, standing in for a real
// conflict-resolution session's own file edits.
func resolutionShuttle(worktreeRoot, resolved string, paths ...string) *shedfake.MergeShuttle {
	return &shedfake.MergeShuttle{RunFn: func(shuttleengine.Spec) (shuttleengine.Result, error) {
		for _, p := range paths {
			if err := os.WriteFile(filepath.Join(worktreeRoot, p), []byte(resolved), 0o644); err != nil {
				return shuttleengine.Result{}, err
			}
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
	}}
}

// openFabricAtLanding opens a *fabricengine.Fabric on the code worktree at path, via
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

// TestFinalize_ResolvesConflictAndSquashMergesIntoParent builds a task pair and a parent pair off
// the same hub, diverges both on conflict.txt (a genuine code-side conflict) and diverges the task
// pair alone on a second, clean records-side file, runs Finalize with a fake session that writes a real
// resolution to the conflicted file, and asserts that the parent pair's code side afterward carries the
// task's resolved content and the squash setting took effect there, that the parent pair's records side is
// left byte-identical -- the records side is not a merge participant, per this task's own
// no-caller-facing-signature-change-in-fabric-shaped change to Fabric.Merge -- and that no merge
// record is left behind on either pair.
func TestFinalize_ResolvesConflictAndSquashMergesIntoParent(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	hubforge.AddPair(t, h, "task")
	hubforge.AddPair(t, h, "parent")

	taskCode, taskRecords := h.PairWarpWorktree("task"), h.PairWeftSibling("task")
	parentCode, parentRecords := h.PairWarpWorktree("parent"), h.PairWeftSibling("parent")

	// The genuine code-side conflict: both branches add conflict.txt independently, off a common
	// ancestor where it does not exist.
	gitkit.CommitFile(t, taskCode, "conflict.txt", "task content\n", "task: add conflict.txt")
	gitkit.CommitFile(t, parentCode, "conflict.txt", "parent content\n", "parent: add conflict.txt")

	// A clean, non-conflicting records-side divergence on the task pair alone, so "the parent pair
	// carries the task's content on both sides" has a concrete records-side fact to assert.
	gitkit.CommitFile(t, taskRecords, "task-note.txt", "task records note\n", "task: add task-note.txt")

	shuttle := resolutionShuttle(taskCode, "resolved content\n", "conflict.txt")

	deps := landingshed.NewTestDeps(t)
	deps.WorktreeRoot = taskCode
	deps.TaskBranch = "task"
	deps.ParentBranch = "parent"
	deps.StencilsDir = seedConflictStencil(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskCode), nil }
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, parentCode), nil }
	deps.Shuttle = shuttle
	deps.Config = landingshed.Config{
		Squash:             true,
		Conflict:           "claude:test-model",
		ConflictTimeoutMin: 1,
		CoAuthoredBy:       "Test Author <test@example.com>",
	}

	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		t.Fatalf("NewFinalize() error = %v; want nil", err)
	}

	// Captured before Call so the records-side-not-a-merge-participant assertion below has a concrete
	// before/after pair to compare, mirroring internal/fabricengine's own before/HEAD
	// convention for its records side (see e.g. its merge-local integration test).
	parentRecordsBefore := gitkit.RevParse(t, parentRecords, "HEAD")

	shedfake.RequireOutcome(t, fz, shedengine.Done)
	if len(shuttle.Specs) != 1 {
		t.Errorf("fake shuttle Run() called %d time(s); want exactly 1 (one conflict-resolution session)", len(shuttle.Specs))
	}

	// The parent pair's code side carries the task's resolved content.
	codeContent, err := os.ReadFile(filepath.Join(parentCode, "conflict.txt"))
	if err != nil {
		t.Fatalf("read parent code conflict.txt: %v", err)
	}
	if string(codeContent) != "resolved content\n" {
		t.Errorf("parent code conflict.txt = %q; want the resolved content %q", codeContent, "resolved content\n")
	}

	// The parent pair's records side is left byte-identical: it is not a merge participant, so the
	// task pair's records-only task-note.txt never reaches it, on either the file-tree or the commit
	// graph -- mirroring internal/fabricengine's own "records side is not a merge participant" assertion
	// shape (see e.g. its merge-local integration test's byte-identical check).
	if _, err := os.Stat(filepath.Join(parentRecords, "task-note.txt")); !os.IsNotExist(err) {
		t.Errorf("os.Stat(parent records task-note.txt) error = %v; want a not-exist error -- the records side is not a merge participant", err)
	}
	if got := gitkit.RevParse(t, parentRecords, "HEAD"); got != parentRecordsBefore {
		t.Errorf("parent records HEAD = %q; want byte-identical %q -- the records side is not a merge participant", got, parentRecordsBefore)
	}

	// The squash setting took effect on the parent pair's code side: a single-parent commit.
	codeHEAD := gitkit.RevParse(t, parentCode, "HEAD")
	if got := len(strings.Fields(gitkit.Git(t, parentCode, "log", "-1", "--format=%P", codeHEAD))); got != 1 {
		t.Errorf("parent code HEAD %s has %d parents; want exactly 1 (a squash commit)", codeHEAD, got)
	}

	// The landing commit's subject is the description title, its body is the description body, and
	// exactly one Co-Authored-By trailer carries the configured value.
	msgCmd := exec.Command("git", "log", "-1", "--format=%B")
	msgCmd.Dir = parentCode
	msgOut, err := msgCmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%B in %s: %v", parentCode, err)
	}
	wantMsg := "A landing title\n\nA landing body.\n\nCo-Authored-By: Test Author <test@example.com>"
	if got := strings.TrimSpace(string(msgOut)); got != wantMsg {
		t.Errorf("landing commit message = %q; want %q", got, wantMsg)
	}
	if n := strings.Count(string(msgOut), "Co-Authored-By:"); n != 1 {
		t.Errorf("landing commit carries %d Co-Authored-By lines; want exactly 1", n)
	}

	// No merge record is left behind on either pair. fabricengine's own MergeRecordExistsForTest is
	// a test-only export reachable only from fabricengine's own test binary, so this package checks
	// the same fact through the public MergeInProgress accessor instead.
	taskHandle := openFabricAtLanding(t, taskCode)
	if inProgress, err := taskHandle.MergeInProgress(); err != nil || inProgress {
		t.Errorf("task pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}
	parentHandle := openFabricAtLanding(t, parentCode)
	if inProgress, err := parentHandle.MergeInProgress(); err != nil || inProgress {
		t.Errorf("parent pair MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}
}

// TestFinalize_AlreadyLandedParentIsIdempotent lands a task once, then calls Finalize again on the
// same pair, and asserts the second call is Done with no second landing commit. A fresh Finalize
// over a parent that already carries the task's squashed diff behaves the same way.
func TestFinalize_AlreadyLandedParentIsIdempotent(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	hubforge.AddPair(t, h, "task")
	hubforge.AddPair(t, h, "parent")

	taskCode := h.PairWarpWorktree("task")
	parentCode := h.PairWarpWorktree("parent")

	gitkit.CommitFile(t, taskCode, "feature.txt", "task feature\n", "task: add feature.txt")

	newFinalize := func() *landingshed.Finalize {
		deps := landingshed.NewTestDeps(t)
		deps.WorktreeRoot = taskCode
		deps.TaskBranch = "task"
		deps.ParentBranch = "parent"
		deps.StencilsDir = seedConflictStencil(t)
		deps.OpenFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskCode), nil }
		deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, parentCode), nil }
		deps.Shuttle = resolutionShuttle(taskCode, "")
		deps.Config = landingshed.Config{
			Squash:             true,
			Conflict:           "claude:test-model",
			ConflictTimeoutMin: 1,
			CoAuthoredBy:       "Test Author <test@example.com>",
		}
		fz, err := landingshed.NewFinalize(deps)
		if err != nil {
			t.Fatalf("NewFinalize() error = %v; want nil", err)
		}
		return fz
	}

	shedfake.RequireOutcome(t, newFinalize(), shedengine.Done)
	headAfterFirst := gitkit.RevParse(t, parentCode, "HEAD")

	// A second Finalize over the now already-landed parent.
	shedfake.RequireOutcome(t, newFinalize(), shedengine.Done)
	if got := gitkit.RevParse(t, parentCode, "HEAD"); got != headAfterFirst {
		t.Errorf("parent code HEAD = %q after second Finalize; want unchanged %q (no second landing commit)", got, headAfterFirst)
	}
}
