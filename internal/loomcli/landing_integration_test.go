//go:build integration

// landing_integration_test.go pins the regression this task exists to fix: two tasks landing in
// sequence off the same parent, with the second landing's parent-side merge asserted conflict-free
// on loom's own status file. It lives in package loomcli, not internal/landingshed or
// internal/fabricengine, because it needs both loom's own status-file path constructors
// (internal/loomengine) and landingshed.Finalize, and internal/loomcli is the one layer that
// legitimately imports both -- see the plan's "the regression test lives in internal/loomcli,
// integration-tagged" decision.
//
// One task landing alone never conflicts: the divergence the bug needs requires both sides of the
// parent-side merge to have rewritten the status file since their merge base, which only the second
// of two sequential landings can produce. This file declares no TestMain of its own -- the package's
// existing untagged internal/loomcli/testmain_test.go already runs gitkit.HermeticGitEnv() once for
// the whole test binary, and a second declaration in the same package would not compile.

package loomcli

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
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// landingConflictStencilFixture is a minimal, valid conflict stencil carrying exactly the two markers
// mergeresolve's own spec builder fills -- this test's own conflict-resolution seam is a strict fake
// that must never actually be called, so the stencil's prose content is irrelevant, only its two
// placeholders. Copied from internal/landingshed's own finalizeConflictStencilFixture: the two test
// packages cannot share unexported test code.
const landingConflictStencilFixture = "# Conflict\n\nPaths:\n{{.conflicted_paths}}\n\nReport: {{.report_path}}\n"

// seedLandingConflictStencil writes landingConflictStencilFixture at the layout mergeresolve's spec
// builder reads (stencilstore.Path's own baseDir/landing/<name>.md shape) and returns the baseDir a
// Deps.StencilsDir field should carry. Copied, renamed, from internal/landingshed's own
// seedConflictStencil.
func seedLandingConflictStencil(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "landing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(stencils dir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "landing-template-conflict.md"), []byte(landingConflictStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict stencil): %v", err)
	}
	return root
}

// openLandingFabric opens a *fabricengine.Fabric on the worktree at path, via
// lyxcwd.ResolveWorktree + fabricengine.Open -- the only production constructor a pair-opener
// closure may legally wrap. Copied, renamed, from internal/landingshed's own openFabricAtLanding.
func openLandingFabric(t *testing.T, path string) *fabricengine.Fabric {
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

// commitOnCurrentBranchForLanding writes filename with content in dir, stages it, and commits msg on
// whatever branch is currently checked out. Copied, renamed, from internal/landingshed's own
// commitOnCurrentBranchLanding: the two test packages cannot share unexported test code.
func commitOnCurrentBranchForLanding(t *testing.T, dir, filename, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", filename, err)
	}
	gitkit.MustRun(t, dir, "git", "add", filename)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", msg)
}

// strictNoConflictShuttle is the load-bearing assertion of this test, not a stub: a
// conflict-resolution session is spawned only when the parent-side merge actually conflicts, so a
// fake that fails outright on any call turns "the second landing conflicted" into a precise test
// failure rather than a downstream symptom.
type strictNoConflictShuttle struct {
	t *testing.T
}

func (s *strictNoConflictShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	s.t.Fatalf("strictNoConflictShuttle.Run called; a conflict-resolution session must never spawn when the two landings' status-file divergence is handled correctly")
	return shuttleengine.Result{}, nil
}

var _ mergeresolve.Shuttle = (*strictNoConflictShuttle)(nil)

// seedDivergentStatus seeds loc's status file via loomshed.Seed and then rewrites it in place with
// distinct content -- a different CurrentProducer and a non-empty History -- standing in for the
// per-transition persists a real Shed run would have made. Divergent content is the whole point:
// byte-identical files on both sides of a merge cannot conflict, so a fixture that skipped this would
// pass even against the pre-fix code.
func seedDivergentStatus(t *testing.T, loc *lyxcwd.Location, slug, parentBranch, producer string) {
	t.Helper()

	statusPath := loomengine.LoomStatusFile(loc)
	statusLockPath := loomengine.LoomStatusLock(loc)

	if err := loomshed.Seed(statusPath, statusLockPath, slug, parentBranch); err != nil {
		t.Fatalf("loomshed.Seed(%s): %v", slug, err)
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("state.ReadJSONStrict(%s): %v", statusPath, err)
	}
	if !found {
		t.Fatalf("state.ReadJSONStrict(%s): not found immediately after Seed", statusPath)
	}

	got.CurrentProducer = producer
	got.History = []shedengine.HistoryEntry{
		{Producer: "Preflight", Outcome: shedengine.Done, Output: "", At: "2026-08-26T00:00:00Z"},
	}

	if err := state.UpdateJSON(statusPath, statusLockPath, func(_ shedengine.Status, _ bool) (shedengine.Status, error) {
		return got, nil
	}); err != nil {
		t.Fatalf("state.UpdateJSON(%s): %v", statusPath, err)
	}
}

// TestFinalize_TwoSequentialLandingsNeverConflictOnTheStatusFile builds a parent pair and two task
// pairs off the same hub, seeds and diverges each task's status file, lands each task in turn, and
// asserts both that neither landing ever needed a conflict-resolution session and that the parent
// pair's weft branch tracks no loom/status.json afterward.
//
// The second landing's catch-up merge is exactly where the old bug surfaced: a single landing alone
// never conflicts, since the divergence the bug needs requires both sides of the parent-side merge to
// have rewritten the status file since their merge base -- which only the second of two sequential
// landings, off a parent pair that already carries the first landing's status file, can produce.
func TestFinalize_TwoSequentialLandingsNeverConflictOnTheStatusFile(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	hubforge.AddPair(t, h, "parent")
	hubforge.AddPair(t, h, "task1")
	hubforge.AddPair(t, h, "task2")

	parentWarp := h.PairWarpWorktree("parent")
	task1Warp := h.PairWarpWorktree("task1")
	task2Warp := h.PairWarpWorktree("task2")

	task1Loc, err := lyxcwd.ResolveWorktree(task1Warp)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(task1): %v", err)
	}
	task2Loc, err := lyxcwd.ResolveWorktree(task2Warp)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(task2): %v", err)
	}

	seedDivergentStatus(t, task1Loc, "task1", "parent", "Discussion")
	seedDivergentStatus(t, task2Loc, "task2", "parent", "Plan")

	// A small, non-conflicting ordinary commit per task pair, so each landing has real content to
	// carry and the merge is not a no-op.
	commitOnCurrentBranchForLanding(t, task1Warp, "task1-note.txt", "task1 content\n", "task1: add task1-note.txt")
	commitOnCurrentBranchForLanding(t, task2Warp, "task2-note.txt", "task2 content\n", "task2: add task2-note.txt")

	stencilsDir := seedLandingConflictStencil(t)

	landTask := func(taskWarp, taskBranch string) {
		t.Helper()

		deps := landingshed.Deps{
			WorktreeRoot:     taskWarp,
			TaskBranch:       taskBranch,
			ParentBranch:     "parent",
			StencilsDir:      stencilsDir,
			ScratchDir:       filepath.Join(t.TempDir(), "scratch"),
			OpenFabric:       func() (*fabricengine.Fabric, error) { return openLandingFabric(t, taskWarp), nil },
			OpenParentFabric: func() (*fabricengine.Fabric, error) { return openLandingFabric(t, parentWarp), nil },
			Shuttle:          &strictNoConflictShuttle{t: t},
			Config: landingshed.Config{
				Squash:             true,
				Conflict:           "claude:test-model",
				ConflictTimeoutMin: 1,
			},
		}

		fz, err := landingshed.NewFinalize(deps)
		if err != nil {
			t.Fatalf("NewFinalize(%s) error = %v; want nil", taskBranch, err)
		}

		outcome, _, err := fz.Call(context.Background())
		if err != nil {
			t.Fatalf("Call(%s) error = %v; want nil", taskBranch, err)
		}
		if outcome != shedengine.Done {
			t.Fatalf("Call(%s) outcome = %q; want %q", taskBranch, outcome, shedengine.Done)
		}
	}

	// task1 lands first, catching the parent pair up with its own content and its own status file.
	landTask(task1Warp, "task1")
	// task2's pair forked from the parent before task1 landed, so task2's catch-up merge against the
	// now-advanced parent, followed by the parent-side merge, is exactly where the old bug surfaced:
	// both the parent's and task2's status files have been rewritten since their shared merge base.
	landTask(task2Warp, "task2")

	// The second assertion the discussion asks for: the parent pair's weft branch tracks no
	// loom/status.json at any depth, pinning the junk-on-parent half of the fix. The conflict-free
	// assertion above alone does not cover this.
	parentWeft := h.PairWeftSibling("parent")
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = parentWeft
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in parent weft %s: %v", parentWeft, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if filepath.Base(line) == "status.json" && filepath.Base(filepath.Dir(line)) == "loom" {
			t.Errorf("parent weft tracks %q; want no loom/status.json anywhere in the tree", line)
		}
	}
}
