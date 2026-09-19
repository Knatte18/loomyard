//go:build integration

// lifecycle_integration_test.go is the end-to-end suite over a real hub built by
// internal/hubforge through its fabric fixture entry point, per the hubforge Fabric-Fixture
// Invariant. It stays a white-box "package battencli" test, not an external "_test" package,
// because it stubs Env.InnerRun.Spawn and Env.InnerRun.ReadStatus at the field level after a real
// wire() call -- a no-op spawn and a read-status answering a chosen state -- so the real poll logic
// (Env.InnerRun.ResolveStatus, the persisted-state branching) is exercised rather than bypassed, and
// that stubbing needs the unexported wire method and the battenCLI receiver.
//
// It lives at the integration tier rather than Tier 1 because the prime-name lookup this package's
// own pre-run refusal performs reaches a real git worktree listing, and getting there at all needs
// the resolver -- both barred from untagged files by the Test Tier Purity Invariant.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// wireForHub builds a *battenCLI wired for real against h's prime Location and slug -- a real
// CreateWorktree and a real Teardown, both driving fabricengine's topology holder against h's own
// hub -- then overrides Env.InnerRun.Spawn and Env.InnerRun.ReadStatus with readStatus, per this
// file's own header.
func wireForHub(t *testing.T, h *hubforge.Hub, slug string, readStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)) *battenCLI {
	t.Helper()
	c := &battenCLI{}
	if err := c.wire(h.Location, slug); err != nil {
		t.Fatalf("wire(%s): %v", slug, err)
	}
	c.env.InnerRun.Spawn = func(ctx context.Context) error { return nil }
	c.env.InnerRun.ReadStatus = readStatus
	return c
}

// seedEntryStatus writes c's status file with CurrentProducer/State as given, an empty non-nil
// History, under c's own StatusPath/StatusLockPath.
func seedEntryStatus(t *testing.T, c *battenCLI, producer string, rowState shedengine.State, history []shedengine.HistoryEntry) {
	t.Helper()
	if history == nil {
		history = []shedengine.HistoryEntry{}
	}
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
		CurrentProducer: producer,
		State:           rowState,
		History:         history,
	}); err != nil {
		t.Fatalf("seed status: %v", err)
	}
}

// pathExists reports whether path exists on disk.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestLifecycleIntegration_CreateThenTeardown_DoneRemovesThePair drives a spawn stub plus a
// read-status answering StateDone through the whole three-row list one row at a time, asserting the
// pair exists on disk after the create row and is gone after the teardown row.
func TestLifecycleIntegration_CreateThenTeardown_DoneRemovesThePair(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "lifecycle-done"

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	ctx := context.Background()
	pairPath := h.PairWarpWorktree(slug)

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (create row): %v", err)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after the create row: %s", pairPath)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (loom-run row): %v", err)
	}
	if !pathExists(pairPath) {
		t.Fatalf("pair does not exist after the loom-run row: %s", pairPath)
	}

	if _, err := shed.Step(ctx); err != nil {
		t.Fatalf("Step (teardown row): %v", err)
	}
	if pathExists(pairPath) {
		t.Errorf("pair still exists after the teardown row: %s", pairPath)
	}
}

// TestLifecycleIntegration_LoomRunBlocked_LeavesThePairIntact drives a read-status answering
// StateBlocked, asserting the run halts blocked with the task worktree still present -- the safety
// property the whole design turns on.
func TestLifecycleIntegration_LoomRunBlocked_LeavesThePairIntact(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "lifecycle-blocked"

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "loom-side-producer", Error: "loom session blocked"}, true, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if !pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("pair does not exist after a blocked run; want it left intact: %s", h.PairWarpWorktree(slug))
	}
}

// TestLifecycleIntegration_DirtyPrime_CreateRowBlocksBeforeAnythingCreated dirties a tracked file
// in the hub's prime worktree, asserting the create row halts blocked before anything is created --
// this refusal fires on every lifecycle run and is invisible to the unit tests' fakes.
func TestLifecycleIntegration_DirtyPrime_CreateRowBlocksBeforeAnythingCreated(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "lifecycle-dirty-prime"

	readmePath := filepath.Join(h.PrimeWorktree(), "README")
	if err := os.WriteFile(readmePath, []byte("dirtied for the test\n"), 0o644); err != nil {
		t.Fatalf("dirty prime README: %v", err)
	}

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		t.Fatal("ReadStatus must not be called: the create row must block before the poll row ever runs")
		return shedengine.Status{}, false, nil
	})
	seedEntryStatus(t, c, battenrecipe.NameWorktreeCreate, shedengine.StateRunning, nil)

	shed, err := battenrecipe.New(c.env, c.shedPaths)
	if err != nil {
		t.Fatalf("battenrecipe.New: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != battenrecipe.NameWorktreeCreate {
		t.Errorf("HaltedProducer = %q; want %q", result.HaltedProducer, battenrecipe.NameWorktreeCreate)
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("pair exists even though the create row blocked before creating anything: %s", h.PairWarpWorktree(slug))
	}
}

// TestLifecycleIntegration_MidListResume_SkipsTheCompletedCreateRow proves mid-list resume: it
// creates the pair directly (standing in for a create row that already completed before a crash),
// seeds a status file whose current producer is the poll row, and re-invokes the run verb --
// asserting CreateWorktree is never called again and the pair is torn down once the poll row
// answers Done.
func TestLifecycleIntegration_MidListResume_SkipsTheCompletedCreateRow(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	slug := "lifecycle-resume"
	hubforge.AddPair(t, h, slug)

	c := wireForHub(t, h, slug, func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
		return shedengine.Status{State: shedengine.StateDone}, true, nil
	})
	c.env.CreateWorktree = func(ctx context.Context) error {
		t.Fatal("CreateWorktree must not run again: the create row already completed before the crash this test simulates")
		return nil
	}
	seedEntryStatus(t, c, battenrecipe.NameRunShed, shedengine.StateBlocked, []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "run"), &out, []string{slug})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("run() output missing ok:true envelope; got: %q", out.String())
	}
	if pathExists(h.PairWarpWorktree(slug)) {
		t.Errorf("pair still exists after the resumed run's teardown row completed: %s", h.PairWarpWorktree(slug))
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}
	// history_length is new in this task: the resumed run's persisted history already carries the
	// pre-seeded create-row entry plus whatever this invocation appended, so it must be at least 2.
	gotLen, ok := envelope["history_length"].(float64)
	if !ok {
		t.Fatalf("envelope[\"history_length\"] = %v (%T); want a number", envelope["history_length"], envelope["history_length"])
	}
	if gotLen < 2 {
		t.Errorf("envelope[\"history_length\"] = %v; want at least 2 (the pre-seeded entry plus this run's own)", gotLen)
	}
}

// TestLifecycleIntegration_NonPrimeRefusal covers both verbs' non-prime refusal, driven through
// RunCLIIn with an injected cwd pointing at a real task worktree -- the runtime check standing in
// for the Bookend invariant's missing enforcing test.
func TestLifecycleIntegration_NonPrimeRefusal(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	taskSlug := "lifecycle-task-cwd"
	hubforge.AddPair(t, h, taskSlug)

	taskCwd := h.PairWarpWorktree(taskSlug)
	primeName := h.Location.WorktreeName

	for _, verb := range []string{"run", "status"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLIIn(taskCwd, &out, []string{verb, "some-slug"})

			if exitCode != 1 {
				t.Fatalf("RunCLIIn(%s) exit code = %d; want 1; output: %s", verb, exitCode, out.String())
			}
			if !strings.Contains(out.String(), taskSlug) {
				t.Errorf("%s refusal = %q; want it to name the task worktree %q", verb, out.String(), taskSlug)
			}
			if !strings.Contains(out.String(), primeName) {
				t.Errorf("%s refusal = %q; want it to name the prime worktree %q", verb, out.String(), primeName)
			}
		})
	}
}
