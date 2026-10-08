//go:build integration

// parity_test.go asserts byte-identical envelopes from the same fixture across the two invocation paths -- "lyx <module> <verb> [<run-id>]" and "lyx shed <verb> [<run-id>]" -- for every verb/ recipe pair this table supports, plus the lightweight-wiring proof.
//
// Every case here is tier 2: RunCLIIn reaches each module's PersistentPreRunE and therefore lyxcwd.Resolve, which spawns git through internal/gitexec, and the Test Tier Purity Invariant bans gitexec.Run outside tier-tagged files -- loom's own run additionally calls c.reed.Up() and fabricengine.Open. internal/loomcli/parity_test.go is the precedent for the comparison shape but not for the tier -- its own header states no test there calls RunCLIIn, so it stays tier 1.
//
// Every case that drives a verb through "lyx shed" first writes the addressed run-id's own seed via shedrun.WriteSeed: shedcli's own pre-run reads a seed before it can resolve which recipe arms the invocation, and both module paths (loomcli's own resolveRunID, battencli's own armSeed) apply their own seed-presence check too, so a missing seed would refuse before either side ever reaches the fixture behaviour a case means to compare.
//
// The comparison itself is a plain byte-for-byte equality of the two captured stdout buffers, rather than loomcli's own three-way producer-vs-CLI verdict mapping: this suite compares two already- materialized CLI outputs against each other, not a producer's own outcome against a CLI's envelope, so there is no second vocabulary to map onto a shared verdict type -- byte equality is the direct and correct check, and it is what "byte-identical" means to a supervisor parsing the line.
//
// Every fixture below is pinned to an arm that refuses or completes strictly above the substrate:
// no parity case here may reach reed, tmux, an LLM producer, or shed.Run/shed.Step's own producer call. run's arm is a hub with no loom status file, refusing at the very first statement in loom's PreRun;
// step's arm is a hub whose run lock is already held, refusing at the early run-lock probe above seedAndCommitBootstrap and reed Up;
// status and pause are read-only and never reach the substrate on any path, so they are driven against a seeded status file to exercise the success envelope; and batten's run is driven against a slug whose persisted status is StateDone, which refuses inside batten's own PreRun before BuildShed is ever called.
// That bound is what keeps this suite in the integration tier rather than pushing it to tmux, and it is also why it proves what it needs to: the two paths' divergence risk lives entirely in arming and pre-run resolution, which every one of these arms exercises in full.

package shedcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// lyxcwdResolveWorktreeForTest resolves cwd -- a pair's code worktree root -- into a *lyxcwd.Location
// via lyxcwd.ResolveWorktree, which applies no cwd gate: the caller here holds a worktree root, not
// an acting cwd, exactly as internal/battencli's own taskWorktreeLocation does.
func lyxcwdResolveWorktreeForTest(t *testing.T, cwd string) (*lyxcwd.Location, error) {
	t.Helper()
	return lyxcwd.ResolveWorktree(cwd)
}

// seedRunForTest writes runID's seed under the pair rooted at cwd, naming recipe, t.Fatal-ing on
// failure. Every case driving a verb through "lyx shed" calls this first: shedcli's own pre-run
// reads a seed before it can resolve which recipe arms the invocation.
func seedRunForTest(t *testing.T, cwd, runID, recipe string) {
	t.Helper()
	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	if err := shedrun.WriteSeed(location, runID, shedrun.Seed{Recipe: recipe, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("seed run %q (recipe %q): %v", runID, recipe, err)
	}
}

// seedLoomStatus writes st as loom's own status file for the pair rooted at cwd, addressed at
// shedrun.SelfRunID.
func seedLoomStatus(t *testing.T, cwd string, st shedengine.Status) {
	t.Helper()
	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	lockPath := shedrun.StatusLock(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	if err := state.WriteJSON(shedrun.StatusFile(location, shedrun.SelfRunID), lockPath, st); err != nil {
		t.Fatalf("seed loom status: %v", err)
	}
}

// runBoth invokes moduleFn and shedFn -- each a thin wrapper around a <module>cli.RunCLIIn and a
// shedcli.RunCLIIn call over the identical cwd and equivalent arguments -- and asserts their
// captured stdout is byte-identical and their exit codes agree. A mismatch reports both sides in
// full, so a diverging byte is never left for the reader to spot by eye.
func runBoth(t *testing.T, label string, moduleFn, shedFn func() (exitCode int, out string)) {
	t.Helper()

	moduleExit, moduleOut := moduleFn()
	shedExit, shedOut := shedFn()

	if moduleExit != shedExit {
		t.Errorf("%s: exit code mismatch: module path = %d, shed path = %d; module output: %q; shed output: %q", label, moduleExit, shedExit, moduleOut, shedOut)
	}
	if moduleOut != shedOut {
		t.Errorf("%s: envelope mismatch:\nmodule path: %q\nshed path:   %q", label, moduleOut, shedOut)
	}
}

// parityLoomRunNoStatusFile drives "lyx loom run" and "lyx shed run" over a fresh pair seeded at "self" for loom but with no loom status file, which refuses at the very first statement in loomPreRun -- well above reed.Up() and fabricengine.Open.
func parityLoomRunNoStatusFile(t *testing.T, h *hubforge.Hub) {
	hubforge.AddPair(t, h, "parity-run")
	cwd := h.PairCodeWorktree("parity-run")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeLoom)

	runBoth(t, "loom run (no status file)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"run"})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"run"})
			return code, out.String()
		},
	)
}

// readRecordedEnvelope decodes the full step envelope at the printed short envelope's envelope_path,
// reporting a test failure when the path is empty or unreadable.
func readRecordedEnvelope(t *testing.T, label string, printed map[string]any) map[string]any {
	t.Helper()
	path, _ := printed["envelope_path"].(string)
	if path == "" {
		t.Fatalf("%s: envelope_path is empty; envelope: %v", label, printed)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read recorded envelope: %v", label, err)
	}
	return envelope.Decode(t, string(data)).Raw
}

// parityLoomStepRunLockBusy drives "lyx loom step" and "lyx shed step" over a pair seeded at "self" for loom whose run lock is already held, which refuses at the early run-lock probe with kind: busy -- above seedAndCommitBootstrap and above reed Up.
func parityLoomStepRunLockBusy(t *testing.T, h *hubforge.Hub) {
	hubforge.AddPair(t, h, "parity-step")
	cwd := h.PairCodeWorktree("parity-step")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeLoom)

	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	lockPath := shedrun.RunLock(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	fl, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%s): %v", lockPath, err)
	}
	t.Cleanup(func() { _ = fl.Release() })

	traceDir := t.TempDir()
	logger.SetDurableSinkDir(traceDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	var moduleOut, shedOut string
	runBoth(t, "loom step (run lock busy)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"step"})
			moduleOut = out.String()
			return code, moduleOut
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"step"})
			shedOut = out.String()
			return code, shedOut
		},
	)

	for label, out := range map[string]string{"module path": moduleOut, "shed path": shedOut} {
		env := envelope.Decode(t, out)
		if env.Raw["kind"] != "busy" {
			t.Errorf("%s: kind = %v; want busy", label, env.Raw["kind"])
		}
		recorded := readRecordedEnvelope(t, label, env.Raw)
		if want := shedrun.ScratchDir(location, shedrun.SelfRunID); recorded["scratch_dir"] != want {
			t.Errorf("%s: recorded scratch_dir = %v; want %q", label, recorded["scratch_dir"], want)
		}
		traceFile, _ := env.Raw["trace_file"].(string)
		if traceFile == "" {
			t.Errorf("%s: trace_file is empty; envelope: %v", label, env)
			continue
		}
		if filepath.Dir(traceFile) != filepath.Clean(traceDir) {
			t.Errorf("%s: trace_file %q is not inside %q", label, traceFile, traceDir)
		}
		content, err := os.ReadFile(traceFile)
		if err != nil {
			t.Errorf("%s: read trace_file: %v", label, err)
			continue
		}
		if !strings.Contains(string(content), `msg="shed: step"`) {
			t.Errorf("%s: trace lacks msg=\"shed: step\":\n%s", label, content)
		}
		refused := false
		for _, line := range strings.Split(string(content), "\n") {
			if strings.Contains(line, `msg="shed: step refused"`) && strings.Contains(line, "kind=busy") {
				refused = true
			}
		}
		if !refused {
			t.Errorf("%s: trace lacks a \"shed: step refused\" line with kind=busy:\n%s", label, content)
		}
	}
}

// parityBattenStepRunLockBusy drives "lyx batten step <slug>" and "lyx shed step <slug>" over a batten-seeded slug whose run lock is already held, which refuses with kind: busy, and asserts the envelope names the slug's scratch_dir and no friction_dir.
func parityBattenStepRunLockBusy(t *testing.T, h *hubforge.Hub) {
	cwd := h.PrimeWorktree()
	const slug = "parity-batten-step-busy"
	seedRunForTest(t, cwd, slug, shedrun.RecipeBatten)

	lockPath := battencli.RunLock(h.Location, slug)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	fl, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%s): %v", lockPath, err)
	}
	t.Cleanup(func() { _ = fl.Release() })

	var moduleOut, shedOut string
	runBoth(t, "batten step (run lock busy)",
		func() (int, string) {
			var out bytes.Buffer
			code := battencli.RunCLIIn(cwd, &out, []string{"step", slug})
			moduleOut = out.String()
			return code, moduleOut
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"step", slug})
			shedOut = out.String()
			return code, shedOut
		},
	)

	for label, out := range map[string]string{"module path": moduleOut, "shed path": shedOut} {
		env := envelope.Decode(t, out)
		if env.Raw["kind"] != "busy" {
			t.Errorf("%s: kind = %v; want busy", label, env.Raw["kind"])
		}
		recorded := readRecordedEnvelope(t, label, env.Raw)
		if want := shedrun.ScratchDir(h.Location, slug); recorded["scratch_dir"] != want {
			t.Errorf("%s: recorded scratch_dir = %v; want %q", label, recorded["scratch_dir"], want)
		}
		if got, _ := recorded["friction_dir"].(string); got != "" {
			t.Errorf("%s: recorded friction_dir = %q; want empty", label, got)
		}
	}
}

// parityLoomStatusSeeded drives "lyx loom status" and "lyx shed status" over a pair seeded at "self" for loom with a seeded status file, exercising the success envelope: status is read-only and lightweight-wired, and never reaches the substrate on any path.
func parityLoomStatusSeeded(t *testing.T, h *hubforge.Hub) {
	hubforge.AddPair(t, h, "parity-status")
	cwd := h.PairCodeWorktree("parity-status")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeLoom)

	seedLoomStatus(t, cwd, shedengine.Status{
		CurrentProducer: "Discussion-Write",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	})

	runBoth(t, "loom status (seeded)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"status"})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"status"})
			return code, out.String()
		},
	)
}

// parityLoomPauseSeeded drives "lyx loom pause" and "lyx shed pause" over a pair seeded at "self" for loom with a seeded status file, exercising the success envelope. pause mutates PauseRequested, but the mutation is idempotent and the envelope carries only status_file, so running both invocations sequentially over the same fixture does not disturb the comparison.
func parityLoomPauseSeeded(t *testing.T, h *hubforge.Hub) {
	hubforge.AddPair(t, h, "parity-pause")
	cwd := h.PairCodeWorktree("parity-pause")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeLoom)

	seedLoomStatus(t, cwd, shedengine.Status{
		CurrentProducer: "Discussion-Write",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	})

	runBoth(t, "loom pause (seeded)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"pause"})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"pause"})
			return code, out.String()
		},
	)
}

// parityBattenRunStateDone drives "lyx batten run <slug>" and "lyx shed run <slug>" over a slug seeded for batten whose persisted status is StateDone, which refuses inside batten's own PreRun before BuildShed is ever called.
func parityBattenRunStateDone(t *testing.T, h *hubforge.Hub) {
	cwd := h.PrimeWorktree()
	const slug = "parity-batten-done"
	seedRunForTest(t, cwd, slug, shedrun.RecipeBatten)

	// battencli.StatusLock lives under the ephemeral .lyx tree, a different parent directory than
	// the durable _lyx one seedRunForTest's own shedrun.WriteSeed just created; state.WriteJSON
	// only MkdirAlls its own status-file parent, never the lock's.
	statusLockPath := battencli.StatusLock(h.Location, slug)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(statusLockPath), err)
	}
	if err := state.WriteJSON(battencli.StatusFile(h.Location, slug), statusLockPath, shedengine.Status{
		CurrentProducer: "WorktreeTeardown",
		State:           shedengine.StateDone,
	}); err != nil {
		t.Fatalf("seed batten status: %v", err)
	}

	runBoth(t, "batten run (state done)",
		func() (int, string) {
			var out bytes.Buffer
			code := battencli.RunCLIIn(cwd, &out, []string{"run", slug})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"run", slug})
			return code, out.String()
		},
	)
}

// parityLightweightWiringStatusSucceedsWhenRunRefuses is the proof "lyx shed status" reaches loom's lightweight wiring rather than the full wire(): over a pair seeded at "self" for loom whose loom module config is deliberately broken in a way that refuses "lyx shed run", "lyx shed status" must still succeed.
// A verb-blind arming would silently reintroduce the exact hazard wireLightweight exists to avoid, on this path only.
func parityLightweightWiringStatusSucceedsWhenRunRefuses(t *testing.T, h *hubforge.Hub) {
	hubforge.AddPair(t, h, "parity-lightweight")
	cwd := h.PairCodeWorktree("parity-lightweight")
	seedRunForTest(t, cwd, shedrun.SelfRunID, shedrun.RecipeLoom)

	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	seedLoomStatus(t, cwd, shedengine.Status{
		CurrentProducer: "Discussion-Write",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	})

	// Break loom's own config with an unparseable model-spec value: LoadConfig's own
	// modelspec.Parse(cfg.Discussion) call fails on it, which is enough to fail the full wire()
	// without touching YAML syntax. wireLightweight never loads this file at all, so status must
	// still succeed.
	cfgPath := filepath.Join(location.AnchorPath(), "_lyx", "config", "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte("discussion: \"::not-a-valid-modelspec::\"\n"), 0o644); err != nil {
		t.Fatalf("write broken loom.yaml: %v", err)
	}

	var runOut bytes.Buffer
	runCode := RunCLIIn(cwd, &runOut, []string{"run"})
	if runCode == 0 {
		t.Fatalf("RunCLIIn(shed run) over a broken loom.yaml exit code = 0; want non-zero (the config must refuse). output: %s", runOut.String())
	}

	var statusOut bytes.Buffer
	statusCode := RunCLIIn(cwd, &statusOut, []string{"status"})
	if statusCode != 0 {
		t.Fatalf("RunCLIIn(shed status) over a broken loom.yaml exit code = %d; want 0 (status must reach the lightweight wiring, not the full wire()). output: %s", statusCode, statusOut.String())
	}
}

// TestParity_Scenario drives every module-versus-shed parity case over one hub; each step addresses its own pair or slug, so a step relies on no earlier step's state and the fixed order only lets a failing step stop the run.
// The scenario calls t.Parallel as a whole: its steps share the hub fixture and the process-global durable sink dir the busy step sets, so no step does.
func TestParity_Scenario(t *testing.T) {
	t.Parallel()
	h := hubforge.NewHub(t, ".")
	steps := []struct {
		name string
		run  func(t *testing.T, h *hubforge.Hub)
	}{
		{"loom run without a status file", parityLoomRunNoStatusFile},
		{"loom step with the run lock busy", parityLoomStepRunLockBusy},
		{"batten step with the run lock busy", parityBattenStepRunLockBusy},
		{"loom status seeded", parityLoomStatusSeeded},
		{"loom pause seeded", parityLoomPauseSeeded},
		{"batten run in state done", parityBattenRunStateDone},
		{"lightweight wiring lets status succeed when run refuses", parityLightweightWiringStatusSucceedsWhenRunRefuses},
		{"batten seed is refused outside prime", seedBattenRefusedOutsidePrime},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) { step.run(t, h) }) {
			return
		}
	}
}
