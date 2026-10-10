//go:build tmux

// smoke_bootstrapwiring_test.go covers bootstrap behaviours that are correct in their own helper
// and were wrong in production anyway, which is the one shape a Tier 1 test over that helper can
// never catch: a helper nothing calls.
//
// The defects were found by crucible rounds driving a real hub, and none is visible from a
// hermetic fixture. ensureFrictionDirAfterSeed had four green unit tests while being called from
// nowhere at all, so only a test that goes through the real bootstrap can tell the two states apart.
//
// Like its siblings, every test here spawns ZERO real LLM subprocesses: the fixture wires
// providerlessShuttleConfig (see its doc comment), and each test dispatches at most the two pure-Go
// precondition rows, never an LLM row.
package loomcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// plantFrictionNote writes a friction note named name inside loc's Tier 2 friction directory --
// composed through the production accessor, never a hand-built literal -- creating the directory if
// it is absent, and returns the path it wrote.
func plantFrictionNote(t *testing.T, loc *lyxcwd.Location, name string) string {
	t.Helper()
	path := filepath.Join(loomengine.LoomFrictionDir(loc), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create friction directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# a friction note\n"), 0o644); err != nil {
		t.Fatalf("write friction note: %v", err)
	}
	return path
}

// TestSmokeStepBootstrapWiring drives `lyx loom step` over one go-seeded pair, in this order:
// a genuine first seed, the handoff voucher that step left, then a re-entry over the same task.
// Each step builds on the state the one before left.
//
// Every behavior here is the regression guard for a helper that was correct and unit-tested and still wrong in production, because nothing called it;
// only a test through the real bootstrap tells the two states apart.
func TestSmokeStepBootstrapWiring(t *testing.T) {
	exe := lyxbin.Build(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	stalePath := plantFrictionNote(t, loc, "left-over-from-an-earlier-task.md")

	// The regression guard for the once-per-task friction clear that never shipped.
	//
	// ensureFrictionDirAfterSeed was written, documented, and unit-tested in start.go, and called from
	// nowhere: neither startCmd's RunE nor seedAndCommitBootstrap reached it. Its four tests were green
	// over an orphan. The consequence, reproduced live against a real hub in crucible round 1: notes
	// left in .lyx/loom/friction/ by an earlier task, or by an earlier run that never reached a
	// reflection trigger, survived a genuine first seed and were handed to the NEXT task's reflection
	// agent as that task's own friction -- which then filed a GitHub issue about them.
	//
	// Both branches are asserted through the real bootstrap, because the branch is not the thing that
	// was broken; reaching it was. `lyx loom step` is the driving verb rather than `lyx loom start`
	// precisely because `step` spawns no driver: `start` delegates to `run`, which calls
	// friction.EnsureDir itself and would mask an unwired clear behind a directory that exists anyway.
	//
	// The first `step` is a genuine first seed.
	// It dispatches Preflight, a pure-Go row, and nothing else.
	t.Run("first seed clears friction notes", func(t *testing.T) {
		stdout, _, err := runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "step")
		if err != nil {
			t.Fatalf("first loom step: %v; output: %s", err, stdout)
		}

		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Errorf("stale friction note still present after a genuine first seed (stat err=%v); want it cleared -- a fresh task must not inherit an earlier one's notes", statErr)
		}
		frictionDir := loomengine.LoomFrictionDir(loc)
		if info, statErr := os.Stat(frictionDir); statErr != nil || !info.IsDir() {
			t.Errorf("friction directory %q after first seed: stat err=%v; want it recreated as a directory", frictionDir, statErr)
		}
	})

	// The wiring guard for the handoff voucher (crucible round 2, R2-F1): recordHandoffVoucher and its
	// consume/detect halves are unit-tested in handoffvoucher_test.go, but a helper nothing calls stays
	// green over an orphan, so this asserts the voucher landed beside the ephemeral tree's other loom
	// files, matching the persisted status.
	//
	// Without the voucher, a completed step leaves state running with a live history and a free run
	// lock, which is byte-identical to a mid-run driver death: the next `lyx loom run` with
	// Tier 2 on then writes a spurious crash-resume note for a task in which nothing crashed.
	//
	// Relies on the first step's completed `step`.
	// It must run before the re-entry step changes the status again.
	t.Run("step records a handoff voucher matching the persisted status", func(t *testing.T) {
		persisted, found, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !found {
			t.Fatalf("read persisted status after step: found=%v err=%v", found, err)
		}

		voucher, found, err := state.ReadJSONStrict[handoffVoucher](loomengine.LoomHandoffVoucher(loc), loomengine.LoomHandoffVoucherLock(loc))
		if err != nil {
			t.Fatalf("read handoff voucher: %v", err)
		}
		if !found {
			t.Fatalf("no handoff voucher at %s after a completed step; want one matching the persisted status -- without it the next run files a spurious crash-resume", loomengine.LoomHandoffVoucher(loc))
		}
		if voucher.HistoryLength != len(persisted.History) || voucher.State != string(persisted.State) {
			t.Errorf("handoff voucher = {history %d, state %q}; want {history %d, state %q} to match the persisted status",
				voucher.HistoryLength, voucher.State, len(persisted.History), persisted.State)
		}
	})

	// The second `step` is an ErrSeedExists re-entry over the same task.
	// A resume's notes are the ones most worth reading, so this branch must leave them exactly where they are.
	t.Run("re-entry keeps friction notes", func(t *testing.T) {
		resumePath := plantFrictionNote(t, loc, "written-during-this-task.md")
		stdout, _, err := runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "step")
		if err != nil {
			t.Fatalf("second loom step: %v; output: %s", err, stdout)
		}

		if _, statErr := os.Stat(resumePath); statErr != nil {
			t.Errorf("friction note written during this task is gone after a re-entry (stat err=%v); want it kept -- only a genuine first seed clears", statErr)
		}
	})
}
