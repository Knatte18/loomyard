//go:build integration

// parity_test.go asserts byte-identical envelopes from the same fixture across the two invocation
// paths -- "lyx <module> <verb>" and "lyx shed <verb> --recipe <module>" -- for every verb/recipe
// pair this table supports, plus positional-argument parity and the lightweight-wiring proof.
//
// Every case here is tier 2: RunCLIIn reaches each module's PersistentPreRunE and therefore
// lyxcwd.Resolve, which spawns git through internal/gitexec, and the Test Tier Purity Invariant bans
// gitexec.Run outside integration/smoke-tagged files -- loom's own run additionally calls
// c.reed.Up() and fabricengine.Open. internal/loomcli/parity_test.go is the precedent for the
// comparison shape but not for the tier -- its own header states no test there calls RunCLIIn, so it
// stays tier 1.
//
// The comparison itself is a plain byte-for-byte equality of the two captured stdout buffers, rather
// than loomcli's own three-way producer-vs-CLI verdict mapping: this suite compares two already-
// materialized CLI outputs against each other, not a producer's own outcome against a CLI's
// envelope, so there is no second vocabulary to map onto a shared verdict type -- byte equality is
// the direct and correct check, and it is what "byte-identical" means to a supervisor parsing the
// line.
//
// Every fixture below is pinned to an arm that refuses or completes strictly above the substrate:
// no parity case here may reach reed, tmux, an LLM producer, or shed.Run/shed.Step's own producer
// call. run's arm is a hub with no loom status file, refusing at the very first statement in loom's
// PreRun; step's arm is a hub whose run lock is already held, refusing at the early run-lock probe
// above seedAndCommitBootstrap and ensureStatusStrand; status and pause are read-only and never
// reach the substrate on any path, so they are driven against a seeded status file to exercise the
// success envelope; and batten's run is driven against a slug whose persisted status is
// StateDone, which refuses inside batten's own PreRun before BuildShed is ever called. That bound
// is what keeps this suite in the integration tier rather than pushing it to smoke, and it is also
// why it proves what it needs to: the two paths' divergence risk lives entirely in arming and
// pre-run resolution, which every one of these arms exercises in full.

package shedcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// lyxcwdResolveWorktreeForTest resolves cwd -- a pair's warp worktree root -- into a *lyxcwd.Location
// via lyxcwd.ResolveWorktree, which applies no cwd gate: the caller here holds a worktree root, not
// an acting cwd, exactly as internal/battencli's own taskWorktreeLocation does.
func lyxcwdResolveWorktreeForTest(t *testing.T, cwd string) (*lyxcwd.Location, error) {
	t.Helper()
	return lyxcwd.ResolveWorktree(cwd)
}

// seedLoomStatus writes st as loom's own status file for the pair rooted at cwd.
func seedLoomStatus(t *testing.T, cwd string, st shedengine.Status) {
	t.Helper()
	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	// The status file lives under _lyx (durable) while its advisory lock lives under .lyx
	// (ephemeral) -- two different trees state.WriteJSON's own MkdirAll does not bridge, since it
	// only creates the status file's own parent. Status's own RunE performs this same MkdirAll via
	// ensureStatusLockDir before ever touching internal/state; this fixture must do it too.
	lockPath := loomengine.LoomStatusLock(location)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	if err := state.WriteJSON(loomengine.LoomStatusFile(location), lockPath, st); err != nil {
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

// TestParity_LoomRun_NoStatusFile drives "lyx loom run" and "lyx shed run --recipe loom" over a
// fresh pair with no loom status file, which refuses at the very first statement in loomPreRun --
// well above reed.Up() and fabricengine.Open.
func TestParity_LoomRun_NoStatusFile(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "parity-run")
	cwd := h.PairWarpWorktree("parity-run")

	runBoth(t, "loom run (no status file)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"run"})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"run", "--recipe", "loom"})
			return code, out.String()
		},
	)
}

// TestParity_LoomStep_RunLockBusy drives "lyx loom step" and "lyx shed step --recipe loom" over a
// pair whose run lock is already held, which refuses at the early run-lock probe with kind: busy --
// above seedAndCommitBootstrap and above ensureStatusStrand.
func TestParity_LoomStep_RunLockBusy(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "parity-step")
	cwd := h.PairWarpWorktree("parity-step")

	location, err := lyxcwdResolveWorktreeForTest(t, cwd)
	if err != nil {
		t.Fatalf("resolve worktree %s: %v", cwd, err)
	}
	lockPath := loomengine.LoomRunLock(location)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(lockPath), err)
	}
	fl, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%s): %v", lockPath, err)
	}
	t.Cleanup(func() { _ = fl.Release() })

	runBoth(t, "loom step (run lock busy)",
		func() (int, string) {
			var out bytes.Buffer
			code := loomcli.RunCLIIn(cwd, &out, []string{"step"})
			return code, out.String()
		},
		func() (int, string) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"step", "--recipe", "loom"})
			return code, out.String()
		},
	)
}

// TestParity_LoomStatus_Seeded drives "lyx loom status" and "lyx shed status --recipe loom" over a
// pair with a seeded status file, exercising the success envelope: status is read-only and
// lightweight-wired, and never reaches the substrate on any path.
func TestParity_LoomStatus_Seeded(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "parity-status")
	cwd := h.PairWarpWorktree("parity-status")

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
			code := RunCLIIn(cwd, &out, []string{"status", "--recipe", "loom"})
			return code, out.String()
		},
	)
}

// TestParity_LoomPause_Seeded drives "lyx loom pause" and "lyx shed pause --recipe loom" over a pair
// with a seeded status file, exercising the success envelope. pause mutates PauseRequested, but the
// mutation is idempotent and the envelope carries only status_file, so running both invocations
// sequentially over the same fixture does not disturb the comparison.
func TestParity_LoomPause_Seeded(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "parity-pause")
	cwd := h.PairWarpWorktree("parity-pause")

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
			code := RunCLIIn(cwd, &out, []string{"pause", "--recipe", "loom"})
			return code, out.String()
		},
	)
}

// TestParity_BattenRun_StateDone drives "lyx batten run <slug>" and "lyx shed run --recipe
// batten <slug>" over a slug whose persisted status is StateDone, which refuses inside
// batten's own PreRun before BuildShed is ever called.
func TestParity_BattenRun_StateDone(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	cwd := h.PrimeWorktree()
	const slug = "parity-batten-done"

	if err := state.WriteJSON(battencli.StatusFile(h.Location, slug), battencli.StatusLock(h.Location, slug), shedengine.Status{
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
			code := RunCLIIn(cwd, &out, []string{"run", "--recipe", "batten", slug})
			return code, out.String()
		},
	)
}

// TestParity_PositionalArgs asserts positional-argument parity: "lyx shed run --recipe batten"
// with no slug and with two slugs must be refused byte-identically to "lyx batten run" with the
// same argument counts, since both sides validate through the identical cobra.ExactArgs(1) value.
func TestParity_PositionalArgs(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	cwd := h.PrimeWorktree()

	cases := []struct {
		name string
		args []string
	}{
		{"NoSlug", nil},
		{"TwoSlugs", []string{"slug-one", "slug-two"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runBoth(t, "batten run positional-arg parity: "+tc.name,
				func() (int, string) {
					var out bytes.Buffer
					code := battencli.RunCLIIn(cwd, &out, append([]string{"run"}, tc.args...))
					return code, out.String()
				},
				func() (int, string) {
					var out bytes.Buffer
					code := RunCLIIn(cwd, &out, append([]string{"run", "--recipe", "batten"}, tc.args...))
					return code, out.String()
				},
			)
		})
	}
}

// TestShed_LoomRunRefusesExtraArgByNoArgs asserts "lyx shed run --recipe loom <extra>" is refused by
// cobra.NoArgs. This is a single-side assertion, not a parity comparison against loom's own path: no
// registered loomcli command accepts an extra positional argument to compare against, so there is
// nothing on the other side of a parity check here.
func TestShed_LoomRunRefusesExtraArgByNoArgs(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	cwd := h.PrimeWorktree()

	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, []string{"run", "--recipe", "loom", "unexpected-extra-arg"})
	if code != 1 {
		t.Fatalf("RunCLIIn(shed run --recipe loom <extra>) exit code = %d; want 1; output: %s", code, out.String())
	}
}

// TestParity_LightweightWiring_StatusSucceedsWhenRunRefuses is the proof "lyx shed status" reaches
// loom's lightweight wiring rather than the full wire(): over a pair whose loom module config is
// deliberately broken in a way that refuses "lyx shed run --recipe loom", "lyx shed status --recipe
// loom" must still succeed. A verb-blind arming would silently reintroduce the exact hazard
// wireLightweight exists to avoid, on this path only.
func TestParity_LightweightWiring_StatusSucceedsWhenRunRefuses(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "parity-lightweight")
	cwd := h.PairWarpWorktree("parity-lightweight")

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
	runCode := RunCLIIn(cwd, &runOut, []string{"run", "--recipe", "loom"})
	if runCode == 0 {
		t.Fatalf("RunCLIIn(shed run --recipe loom) over a broken loom.yaml exit code = 0; want non-zero (the config must refuse). output: %s", runOut.String())
	}

	var statusOut bytes.Buffer
	statusCode := RunCLIIn(cwd, &statusOut, []string{"status", "--recipe", "loom"})
	if statusCode != 0 {
		t.Fatalf("RunCLIIn(shed status --recipe loom) over a broken loom.yaml exit code = %d; want 0 (status must reach the lightweight wiring, not the full wire()). output: %s", statusCode, statusOut.String())
	}
}
