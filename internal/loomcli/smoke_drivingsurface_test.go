//go:build smoke

// smoke_drivingsurface_test.go pins the live-substrate properties behind a run's driving surface.
// On the start side, an llm-seeded start removes every status strand,
// and a failed reed Up refuses before any spawn.
// On the step side, "lyx loom step" and "lyx shed step" bring reed up and never add, replace or remove a status strand, whatever driver the run was seeded with.
// ensureStatusStrand's branches are pinned at Tier 1 through resolveStatusStrandAction;
// what only a real tmux server can show is that a step leaves reed's strand table without a status strand, and leaves a pre-existing one alone.
//
// Zero real LLM subprocesses: the fixtures are reused from smoke_test.go and smoke_driverstrand_test.go,
// and the shuttle config is the providerless one,
// so the producer a step reaches bounces at launch rather than starting a provider.
package loomcli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verbSmokeTimeout bounds one step or `start --no-attach` invocation.
// The providerless shuttle config makes a step's producer launch and a start's llm driver launch fail within seconds,
// and a failed reed Up refuses before either.
const verbSmokeTimeout = 60 * time.Second

func TestSmokeStep_AddsNoStatusStrandOnEitherDriver(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)

	seeders := map[string]func(*testing.T, *lyxcwd.Location){
		"go":  seedGoDriverRun,
		"llm": seedLLMDriver,
	}
	verbs := map[string][]string{
		"loom step": {"loom", "step"},
		"shed step": {"shed", "step"},
	}
	for driver, seed := range seeders {
		for verb, args := range verbs {
			t.Run(driver+"/"+verb, func(t *testing.T) {
				_, loc, worktree, _ := newWiredPairFixture(t)
				registerBootstrapTeardown(t, loc, worktree)
				seed(t, loc)

				out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, args...)
				if err != nil {
					t.Fatalf("lyx %s: %v; output: %s", verb, err, out)
				}

				eng := probeReedEngine(t, loc)
				if _, err := eng.Status(); err != nil {
					t.Fatalf("reed status after %s: %v; want reed up; output: %s", verb, err, out)
				}
				if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 0 {
					t.Errorf("status strands after %s on a %s-seeded run = %d; want 0", verb, driver, count)
				}
			})
		}
	}
}

func TestSmokeStep_LeavesAPreexistingStatusStrandUntouched(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	eng := probeReedEngine(t, loc)
	if _, err := eng.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}
	added, err := eng.AddStrand(statusStrandAddSpec("sleep 3600"))
	if err != nil {
		t.Fatalf("add status strand: %v", err)
	}

	out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "step")
	if err != nil {
		t.Fatalf("lyx loom step: %v; output: %s", err, out)
	}

	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	var guids []string
	for _, s := range status.Strands {
		if s.Name == statusStrandDisplayName {
			guids = append(guids, s.GUID)
		}
	}
	if len(guids) != 1 || guids[0] != added.GUID {
		t.Errorf("status strand guids after step = %v; want exactly [%s], the one added before the step", guids, added.GUID)
	}
}

// newBadReedUpFixture builds a hub whose reed config carries an invalid mouse value, so reed Up fails.
// It seeds the pair with seed and returns the resolved location and the worktree path.
func newBadReedUpFixture(t *testing.T, seed func(*testing.T, *lyxcwd.Location)) (*lyxcwd.Location, string) {
	t.Helper()
	reedCfg := reedengine.ConfigTemplate()
	var lines []string
	replaced := false
	for _, line := range strings.Split(reedCfg, "\n") {
		if strings.HasPrefix(line, "mouse:") {
			line = "mouse: not-a-mouse-value"
			replaced = true
		}
		lines = append(lines, line)
	}
	if !replaced {
		t.Fatalf("reed config template drift: no mouse: line found")
	}

	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    strings.Join(lines, "\n"),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairWarpWorktree(slug)
	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	seed(t, loc)
	return loc, worktree
}

func TestSmokeStep_FailedReedUpRefusesWithBootstrapKind(t *testing.T) {
	exe := sharedLyxBinary(t)
	_, worktree := newBadReedUpFixture(t, seedGoDriverRun)

	out, exit, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "step")
	if err != nil {
		t.Fatalf("lyx loom step: %v; output: %s", err, out)
	}
	if exit != 1 {
		t.Fatalf("lyx loom step exit = %d; want 1; output: %s", exit, out)
	}
	var env map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var candidate map[string]any
		if json.Unmarshal([]byte(line), &candidate) == nil {
			env = candidate
		}
	}
	if env == nil || env["kind"] != "bootstrap" {
		t.Errorf("envelope kind = %v; want \"bootstrap\"; output: %s", env["kind"], out)
	}
}

// TestSmokeStart_LLMRunRemovesEveryStatusStrand pins that an llm-seeded start removes every status strand the session holds, duplicates included.
// The providerless shuttle config makes the driver launch fail, which the test ignores: the removal runs before the driver spawn.
func TestSmokeStart_LLMRunRemovesEveryStatusStrand(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedLLMDriver(t, loc)

	eng := probeReedEngine(t, loc)
	if _, err := eng.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := eng.AddStrand(statusStrandAddSpec("sleep 3600")); err != nil {
			t.Fatalf("add status strand %d: %v", i, err)
		}
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 2 {
		t.Fatalf("status strands before start = %d; want 2", count)
	}

	out, _, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("lyx loom start --no-attach: %v; output: %s", err, out)
	}

	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 0 {
		t.Errorf("status strands after an llm-seeded start = %d; want 0; output: %s", count, out)
	}
}

// TestSmokeStart_FailedReedUpRefusesOnEitherDriver pins that a failed reed Up refuses start before the watchdog and driver spawn on both arms.
// It needs no tmux.
func TestSmokeStart_FailedReedUpRefusesOnEitherDriver(t *testing.T) {
	exe := sharedLyxBinary(t)

	seeders := map[string]func(*testing.T, *lyxcwd.Location){
		"go":  seedGoDriverRun,
		"llm": seedLLMDriver,
	}
	for driver, seed := range seeders {
		t.Run(driver, func(t *testing.T) {
			loc, worktree := newBadReedUpFixture(t, seed)

			out, exit, err := runLoomCLINoFatal(exe, worktree, verbSmokeTimeout, "loom", "start", "--no-attach")
			if err != nil {
				t.Fatalf("lyx loom start --no-attach: %v; output: %s", err, out)
			}
			if exit != 1 {
				t.Fatalf("lyx loom start exit = %d; want 1; output: %s", exit, out)
			}
			if !strings.Contains(out, `"ok":false`) {
				t.Errorf("start envelope is not ok:false; output: %s", out)
			}
			if pids := findWatchdogPIDs(loc.HubPath); len(pids) != 0 {
				t.Errorf("watchdog pids after a failed reed Up = %v; want none", pids)
			}
			if pids := findDriverPIDs(worktree); len(pids) != 0 {
				t.Errorf("driver pids after a failed reed Up = %v; want none", pids)
			}
		})
	}
}
