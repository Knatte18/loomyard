//go:build smoke

// smoke_test.go is the tagged smoke suite for the session bootstrap: the one thing nothing else in
// this package can catch, because it needs a real tmux server, a real detached driver process, and
// real advisory-lock interaction, not a hermetic fixture. It follows internal/reedcli's own smoke
// suite conventions (see its smoke_test.go/smoke_lifecycle_test.go) -- a real wired hub via
// hubforge.NewHub, a real tmux binary resolved from PATH or an override env var, skipping outright
// when the multiplexer is absent -- adapted for loom's own shape: every verb here goes through the
// REAL BUILT cmd/lyx binary as a genuine subprocess, never RunCLI in-process, because
// "lyx loom run" spawns its detached driver via os.Executable(), which resolves to whatever binary is
// currently running -- an in-process RunCLI call would resolve that to the test binary itself, not a
// binary "loom drive" knows how to dispatch.
//
// This suite is the regression home for the two bugs this task's own design rounds found before any
// code existed: the cleanliness-ordering blocker (loom's own seed dirtying the weft and failing
// loom's own first precondition row -- TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit) and the
// double-spawn window (the run lock being taken by the child long after the spawn call returns --
// TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver).
//
// A note on driver-liveness timing: loom's own producer table (contracts/recipes/loom-recipe.yaml)
// now carries seventeen rows, every one backed by a real producer -- no row reports Done
// unconditionally. A freshly-bootstrapped driver against a pair with no discussion or plan
// artifacts yet still bounces through Discussion-Write/Discussion-Validate a bounded number of
// times (shedengine's own default bounce budget) and then blocks, well before reaching any later
// row -- a lifecycle that can still complete in well under a second. Tests here that
// assert "a driver process exists" treat that as a best-effort observation (logged, not failed, when
// the driver has already run to completion by check time) and lean on the STATUS FILE's own history
// -- durable regardless of whether the driver process itself is still alive -- for the assertions
// that must hold unconditionally.
package loomcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// tmuxBinaryPath returns the tmux binary path from the environment or resolved via PATH, skipping
// the calling test when it is absent so a -tags=smoke run never hard-fails on a machine without the
// tool -- mirroring internal/reedcli's own tmuxBinaryPath.
func tmuxBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYX_LOOM_TMUX"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not found on PATH; set LYX_LOOM_TMUX to override")
	}
	return path
}

// smokeLyxBuild caches the one cmd/lyx binary this whole test binary needs, built exactly once
// regardless of how many tests call buildLyxBinary -- mirroring hubforge's own bareTemplateOnce
// pattern for an equally expensive one-time build.
var (
	smokeLyxBuildOnce sync.Once
	smokeLyxBuildPath string
	smokeLyxBuildErr  error
)

var _, smokeTestFile, _, _ = runtime.Caller(0)

// buildLyxBinary compiles cmd/lyx into a scratch temp dir and returns its path, building it at most
// once per test binary.
func buildLyxBinary(t *testing.T) string {
	t.Helper()
	smokeLyxBuildOnce.Do(func() {
		repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(smokeTestFile), "..", ".."))
		if err != nil {
			smokeLyxBuildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "loomcli-smoke-lyx-*")
		if err != nil {
			smokeLyxBuildErr = err
			return
		}
		exe := filepath.Join(dir, "lyx")
		cmd := exec.Command("go", "build", "-o", exe, "./cmd/lyx")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			smokeLyxBuildErr = fmt.Errorf("go build ./cmd/lyx: %w\n%s", err, out)
			return
		}
		smokeLyxBuildPath = exe
	})
	if smokeLyxBuildErr != nil {
		t.Fatalf("build lyx binary: %v", smokeLyxBuildErr)
	}
	return smokeLyxBuildPath
}

// runLoomCLINoFatal runs exe with args in dir as a real subprocess, bounded by timeout, and returns
// its combined stdout+stderr and exit code without ever calling a *testing.T method -- the pure seam
// case (h)'s concurrent invocations need, since t.Fatalf from a non-test goroutine is unsafe. Callers
// on the test's own goroutine that want fail-fast behaviour check the returned err themselves.
func runLoomCLINoFatal(exe, dir string, timeout time.Duration, args ...string) (stdout string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	out, runErr := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), -1, fmt.Errorf("lyx %v timed out after %s in %s; output so far:\n%s", args, timeout, dir, out)
	}
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, fmt.Errorf("lyx %v: %w; output:\n%s", args, runErr, out)
}

// newWiredPairFixture builds a real fabric hub, seeds every module's config template into it, and
// adds one worktree pair -- the shape every test in this suite needs before it can bootstrap loom
// against it. AddPair already writes and commits the pair's origin record (see the plan's
// origin-record-records-both-its-write-and-its-commit decision), so a caller never needs a --parent
// flag on its own first "loom run" call.
func newWiredPairFixture(t *testing.T) (h *hubforge.Hub, loc *lyxcwd.Location, worktree, slug string) {
	t.Helper()

	h = hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})

	slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree = h.PairWarpWorktree(slug)

	var err error
	loc, err = lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	return h, loc, worktree, slug
}

// providerlessShuttleConfig returns shuttle's shipped config template with two values overridden so
// no fixture in this suite can ever start a real provider session: the provider binary is a path
// that cannot exist, and the startup window is short enough that the resulting launch failure is
// classified within a test's own patience rather than ninety seconds later.
//
// This is a correction, not a convenience. The suite's own header used to assert that every fixture
// here "bounces before a real spawn", and the campaign's cost model was written on that claim. It
// stopped being true: a crucible round measured one real `claude` subprocess alive for the full
// thirty seconds of TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed, which is why that test
// timed out rather than merely being slow. Every test in this file is about bootstrap and driver
// mechanics -- locks, handshakes, strands, the status file -- and none of them is about what a
// provider does, so removing the provider makes the suite test what it claims to test AND makes the
// zero-subprocess claim true by construction instead of by hope.
func providerlessShuttleConfig() string {
	cfg := shuttleengine.ConfigTemplate()
	cfg = strings.Replace(cfg, "claude: ${env:LYX_SHUTTLE_CLAUDE:-}", "claude: /nonexistent/lyx-smoke-has-no-provider", 1)
	cfg = strings.Replace(cfg, "startup_timeout_s: 90", "startup_timeout_s: 2", 1)
	return cfg
}

// fastDeadlineLoomConfig returns loom's shipped config template with the discussion producer's
// deadline cut from eight hours to one minute.
//
// The two overrides work as a pair with providerlessShuttleConfig above, and neither is sufficient
// alone. Removing the provider stops a real session from starting; it does not stop the wait. A
// launch that fails still leaves reed holding the pane, so shuttle's wait loop polls until the
// SPEC's own deadline -- and Discussion-Write's deadline comes from loom.yaml's
// discussion_timeout_min, which ships at 480 so an autonomous agent exploring a codebase is never
// cut off. In a fixture with no agent at all that is an eight-hour block on a thirty-second test.
func fastDeadlineLoomConfig() string {
	return strings.Replace(loomengine.ConfigTemplate(), "discussion_timeout_min: 480", "discussion_timeout_min: 1", 1)
}

// registerBootstrapTeardown registers a cleanup that kills any surviving driver process for worktree
// and tears the reed substrate down, so a failed assertion never leaves a live tmux server or a
// detached driver running past the test.
func registerBootstrapTeardown(t *testing.T, loc *lyxcwd.Location, worktree string) {
	t.Helper()
	t.Cleanup(func() {
		for _, pid := range findDriverPIDs(worktree) {
			_ = proc.KillPID(pid)
		}
		eng := probeReedEngine(t, loc)
		_, _ = eng.Down()
	})
}

// probeReedEngine builds a standalone *reedengine.Engine against loc, independent of any loomCLI
// receiver, so a test can query reed's own Status()/Down() without going through the loom CLI seam.
func probeReedEngine(t *testing.T, loc *lyxcwd.Location) *reedengine.Engine {
	t.Helper()
	reedCfg, err := reedengine.LoadConfig(loc.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	return reedengine.New(reedCfg, hubgeom.ReedGeometry(loc))
}

// findDriverPIDs returns the pids of every live process whose current working directory is worktree
// and whose argv contains "drive" -- the /proc-native way to find the detached "lyx loom drive"
// process a bootstrap spawned, since it inherits its parent's cwd and its own argv is fixed. Linux
// only, mirroring internal/reedcli's own /proc-native probes; returns nil on any other GOOS.
func findDriverPIDs(worktree string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || cwd != worktree {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		for _, a := range strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00") {
			if a == "drive" {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// waitRunLockFree polls until loc's run lock is observably free -- released by a driver that has run
// to completion on its own, or by one this test already killed -- or fails the test after timeout.
func waitRunLockFree(t *testing.T, loc *lyxcwd.Location, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		fl, free, err := lock.TryAcquireWriteLock(loomengine.LoomRunLock(loc))
		if err != nil {
			t.Fatalf("probe run lock: %v", err)
		}
		if free {
			_ = fl.Release()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run lock still held after %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// weftCommitCount returns the number of commits reachable from HEAD in the git repository at dir.
func weftCommitCount(t *testing.T, dir string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatalf("git -C %s rev-list --count HEAD: %v", dir, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse rev-list count %q: %v", out, err)
	}
	return n
}

// weftHeadChangedFiles returns the paths HEAD's own commit changed, relative to dir's repository
// root.
func weftHeadChangedFiles(t *testing.T, dir string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").Output()
	if err != nil {
		t.Fatalf("git -C %s diff-tree HEAD: %v", dir, err)
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
}

// seedAndCommitStatus seeds loc's status file directly through the production Seed function and
// commits it weft-side -- the same seed-then-commit shape "lyx loom run" itself performs, used here
// so a standalone-driver test does not need a live tmux server just to get a valid seeded pair.
func seedAndCommitStatus(t *testing.T, loc *lyxcwd.Location, slug string) {
	t.Helper()
	if err := loomshed.Seed(loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc), slug, "main"); err != nil {
		t.Fatalf("loomshed.Seed: %v", err)
	}
	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitWeftPaths(rec, fabricengine.WeftWorktree(loc), loc.AnchorRel, []string{loomengine.LoomStatusRel()}, "smoke: seed status", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit seed: %v", err)
	}
}

// poisonStatusFile adds an unrecognised top-level field to loc's already-seeded status file and
// commits the change weft-side. This is the whole rig behind the driver-failure regression cases: the
// lenient json.Unmarshal shedengine.persist and loomshed.Seed both read through
// (internal/state.UpdateJSON) tolerates an unknown field, so ordinary re-entrant seeding stays
// unaffected, while the strict decoder Shed.Run's own read gate uses (state.ReadJSONStrict,
// DisallowUnknownFields) rejects it outright, and does so BEFORE the loop ever calls a producer or
// performs a persist -- an environment condition the production code already honours, not a change to
// it.
func poisonStatusFile(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	statusPath := loomengine.LoomStatusFile(loc)

	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal status file: %v", err)
	}
	fields["__smoke_unknown_field__"] = true
	poisoned, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatalf("marshal poisoned status: %v", err)
	}
	if err := os.WriteFile(statusPath, poisoned, 0o644); err != nil {
		t.Fatalf("write poisoned status: %v", err)
	}

	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitWeftPaths(rec, fabricengine.WeftWorktree(loc), loc.AnchorRel, []string{loomengine.LoomStatusRel()}, "smoke: poison status file for driver-failure rig", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit poisoned status: %v", err)
	}
}

// statusStrandCount returns how many of eng's tracked strands carry the given name.
func statusStrandCount(t *testing.T, eng *reedengine.Engine, name string) int {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	count := 0
	for _, s := range status.Strands {
		if s.Name == name {
			count++
		}
	}
	return count
}

// (a) the bootstrap verb in a real wired hub brings the tmux session up, leaves exactly one status
// strand present under its fixed name, leaves a detached driver process alive, and leaves the status
// file seeded.
func TestSmokeBootstrap_BringsUpSessionStrandAndDriver(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	// The overall envelope is not asserted here: a fast-finishing driver (loom's own bounded bounce
	// loop can finish inside a single poll gap) can make the handshake itself report a refusal even
	// though every one of this case's own assertions -- session up, strand present, status seeded --
	// already succeeded before the handshake ever ran. See the file-level doc comment.
	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("loom run: %v; output: %s", err, stdout)
	}

	eng := probeReedEngine(t, loc)
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Errorf("status strands named %q = %d; want exactly 1", statusStrandDisplayName, count)
	}

	if pids := findDriverPIDs(worktree); len(pids) == 0 {
		// See the file-level doc comment's note on driver-liveness timing: loom's own bounded
		// bounce loop can finish before this check runs. Logged rather than failed, since the
		// durable status-file assertion below is what actually pins "the driver ran".
		t.Logf("no detached driver process observed for %s; loom's bounded bounce loop may already have finished", worktree)
	} else if !proc.IsAlive(pids[0]) {
		t.Errorf("driver pid %d found but not alive", pids[0])
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc))
	if err != nil || !found {
		t.Fatalf("read status file after bootstrap: found=%v err=%v", found, err)
	}
	if st.CurrentProducer == "" {
		t.Errorf("seeded status file has an empty current_producer")
	}
}

// (b) a second bootstrap invocation while the first driver holds the run lock leaves still exactly
// one strand and still exactly one driver process, spawns nothing new, and does not surface the
// seed-exists refusal as a failure.
func TestSmokeBootstrap_SecondInvocationDoesNotSpawnASecondDriver(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	// Neither invocation's overall envelope is asserted ok:true here, for the same fast-driver-race
	// reason the file-level doc comment states -- what this case actually guards is that the
	// SEED-EXISTS sentinel specifically is never what surfaces as a failure, which is asserted
	// directly against the error text below rather than against ok:false's mere presence.
	stdout1, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("first loom run: %v; output: %s", err, stdout1)
	}
	firstPIDs := findDriverPIDs(worktree)

	stdout2, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("second loom run: %v; output: %s", err, stdout2)
	}
	if strings.Contains(stdout2, `"ok":false`) && strings.Contains(strings.ToLower(stdout2), "already exists") {
		t.Errorf("second bootstrap surfaced the seed-exists sentinel as a failure (must be tolerated, not surfaced): %s", stdout2)
	}

	secondPIDs := findDriverPIDs(worktree)
	if len(firstPIDs) == 1 && len(secondPIDs) == 1 {
		if secondPIDs[0] != firstPIDs[0] {
			t.Errorf("driver pid after the second bootstrap = %d; want the same pid %d the first bootstrap spawned (no second driver)", secondPIDs[0], firstPIDs[0])
		}
	} else if len(secondPIDs) > 1 {
		t.Errorf("driver pids after two bootstraps = %v; want at most 1 alive at once", secondPIDs)
	} else {
		t.Logf("driver pid observation was inconclusive (first=%v second=%v); loom's bounded bounce loop may already have finished -- see the file-level doc comment", firstPIDs, secondPIDs)
	}

	eng := probeReedEngine(t, loc)
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Errorf("status strands named %q after two bootstraps = %d; want exactly 1", statusStrandDisplayName, count)
	}
}

// (c) the driver verb standalone with no tmux at all, on a pair the fixture seeded by running the
// bootstrap once and then killing the driver, advances the machine and records that advance in the
// status file.
func TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("bootstrap: %v; output: %s", err, stdout)
	}

	for _, pid := range findDriverPIDs(worktree) {
		_ = proc.KillPID(pid)
	}
	waitRunLockFree(t, loc, 20*time.Second)

	before, foundBefore, err := state.ReadJSONStrict[shedengine.Status](loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc))
	if err != nil || !foundBefore {
		t.Fatalf("read status file before standalone drive: found=%v err=%v", foundBefore, err)
	}

	// The timeout is generous and the exit code is deliberately not asserted. What this case is
	// about is the VERB advancing the machine standalone, and in a fixture with no provider the row
	// it advances into ends in a launch failure -- a legitimate non-zero exit that says the driver
	// did its job and the (absent) agent did not. Asserting exit 0 instead made this test depend on
	// a real provider session completing, which is how it came to spawn one, block on it, and time
	// out. The durable assertion is the status file's own history, which is what "advances the
	// machine" means and is true regardless of how the row ended.
	driveOut, driveCode, err := runLoomCLINoFatal(exe, worktree, 3*time.Minute, "loom", "drive")
	if err != nil {
		t.Fatalf("loom drive: %v; output: %s", err, driveOut)
	}
	t.Logf("loom drive exited %d: %s", driveCode, strings.TrimSpace(driveOut))

	after, foundAfter, err := state.ReadJSONStrict[shedengine.Status](loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc))
	if err != nil || !foundAfter {
		t.Fatalf("read status file after standalone drive: found=%v err=%v", foundAfter, err)
	}
	if len(after.History) <= len(before.History) {
		t.Errorf("history length after standalone drive = %d; want more than before (%d) -- the machine must advance", len(after.History), len(before.History))
	}
}

// (d) the driver verb on a never-seeded pair refuses on the envelope with a message naming the
// bootstrap verb, writes no driver log, and leaves the weft clean.
func TestSmokeDriveStandalone_RefusesOnNeverSeededPair(t *testing.T) {
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)

	weftDir := fabricengine.WeftWorktree(loc)
	beforeCount := weftCommitCount(t, weftDir)

	stdout, code, err := runLoomCLINoFatal(exe, worktree, 15*time.Second, "loom", "drive")
	if err != nil {
		t.Fatalf("loom drive: %v; output: %s", err, stdout)
	}
	if code != 1 {
		t.Fatalf("loom drive on a never-seeded pair = %d; want 1", code)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
		t.Fatalf("decode refusal envelope %q: %v", stdout, err)
	}
	if envelope.OK {
		t.Fatalf("refusal envelope ok = true; want false: %s", stdout)
	}
	if !strings.Contains(envelope.Error, "loom run") {
		t.Errorf("refusal error = %q; want it to name the bootstrap verb", envelope.Error)
	}

	if _, err := os.Stat(loomengine.LoomDriverLog(loc)); !os.IsNotExist(err) {
		t.Errorf("driver log exists after a never-seeded refusal (stat err=%v); want none written", err)
	}

	clean, reason, err := fabricengine.Clean(loc)
	if err != nil {
		t.Fatalf("fabricengine.Clean: %v", err)
	}
	if !clean {
		t.Errorf("fabricengine.Clean() = (false, %q); want the weft left clean after the refusal", reason)
	}
	if afterCount := weftCommitCount(t, weftDir); afterCount != beforeCount {
		t.Errorf("weft commit count changed from %d to %d; want unchanged on a pre-flight refusal", beforeCount, afterCount)
	}
}

// (e) a driver rigged to fail before its first persist leaves a non-empty driver log naming the
// failure. See poisonStatusFile's doc comment for the rig itself: an unknown field state.UpdateJSON
// tolerates but state.ReadJSONStrict rejects, so Shed.Run's own read gate fails before the loop ever
// calls a producer -- before any persist can happen.
func TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog(t *testing.T) {
	exe := buildLyxBinary(t)
	_, loc, worktree, slug := newWiredPairFixture(t)
	// "loom drive" calls reed.Up() before the phase machine runs, so this case brings a real tmux
	// server up even though it never reaches a producer. Without this teardown it leaked that server
	// past the test -- measured, not theorised: two of them were left running by a crucible round's
	// own smoke sweep.
	registerBootstrapTeardown(t, loc, worktree)

	seedAndCommitStatus(t, loc, slug)
	poisonStatusFile(t, loc)
	poisonedBytes, err := os.ReadFile(loomengine.LoomStatusFile(loc))
	if err != nil {
		t.Fatalf("read poisoned status file: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "driver.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create driver log: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "loom", "drive")
	cmd.Dir = worktree
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	runErr := cmd.Run()
	_ = logFile.Close()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("loom drive against a poisoned status file timed out")
	}
	if runErr == nil {
		t.Fatalf("loom drive against a poisoned status file succeeded; want a failure before any persist")
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read driver log: %v", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		t.Fatalf("driver log is empty; want it to name the failure")
	}
	if !strings.Contains(strings.ToLower(string(content)), "decode") {
		t.Errorf("driver log = %q; want it to name the decode failure", content)
	}

	after, err := os.ReadFile(loomengine.LoomStatusFile(loc))
	if err != nil {
		t.Fatalf("read status file after the failed drive: %v", err)
	}
	if string(after) != string(poisonedBytes) {
		t.Errorf("status file changed after the failed drive; want it untouched -- a persist must never have happened")
	}
}

// (f) the run launcher exists in the per-slug hub launcher directory after a pair is added and is
// gone after the matching remove.
func TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "loom-smoke-launcher"
	hubforge.AddPair(t, h, slug)

	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}
	runLauncherPath := filepath.Join(h.PairLauncherDir(slug), "run"+ext)
	if _, err := os.Stat(runLauncherPath); err != nil {
		t.Fatalf("run launcher missing after add: %v", err)
	}

	if _, err := h.Topology.Remove(h.Location, slug, false); err != nil {
		t.Fatalf("Remove(%s): %v", slug, err)
	}
	if _, err := os.Stat(runLauncherPath); !os.IsNotExist(err) {
		t.Fatalf("run launcher still present after remove (stat err=%v); want it gone", err)
	}
}

// (g) the cleanliness ordering -- in a freshly added, never-run pair the bootstrap reaches past the
// first precondition row rather than blocking on it, the fabric cleanliness check reports clean
// immediately after the seed commit, and the weft carries exactly one new commit touching only the
// status file. This is one of the two named regression guards this suite exists for and asserts the
// mechanism directly, not merely a happy outcome.
// This case deliberately drives just the seed-then-commit mechanism directly (seedAndCommitStatus,
// the same Seed+CommitWeftPaths pair "lyx loom run" itself performs at its own steps 2-3) rather than
// the full "loom run" bootstrap: once a driver actually runs, its own persists rewrite the status
// file's WORKING TREE content on every phase transition without ever committing those rewrites (only
// the CLI's own explicit seed commit is ever committed), which would dirty the weft again and make a
// post-driver cleanliness check fail for a reason that has nothing to do with the ordering this case
// exists to pin -- see the file-level doc comment's note on driver-liveness timing for why the driver
// portion of a full bootstrap cannot be relied on to have not yet run by the time a caller checks.
func TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit(t *testing.T) {
	_, loc, worktree, slug := newWiredPairFixture(t)

	weftDir := fabricengine.WeftWorktree(loc)
	beforeCount := weftCommitCount(t, weftDir)

	seedAndCommitStatus(t, loc, slug)

	clean, reason, err := fabricengine.Clean(loc)
	if err != nil {
		t.Fatalf("fabricengine.Clean: %v", err)
	}
	if !clean {
		t.Errorf("fabricengine.Clean() = (false, %q); want clean immediately after the seed commit", reason)
	}

	afterCount := weftCommitCount(t, weftDir)
	if afterCount != beforeCount+1 {
		t.Errorf("weft commit count = %d; want exactly %d (the single seed commit)", afterCount, beforeCount+1)
	}
	wantFiles := []string{loomengine.LoomStatusRel()}
	if changed := weftHeadChangedFiles(t, weftDir); !slices.Equal(changed, wantFiles) {
		t.Errorf("weft HEAD changed files = %v; want exactly %v", changed, wantFiles)
	}

	report, _, err := preflight.Check(worktree)
	if err != nil {
		t.Fatalf("preflight.Check: %v", err)
	}
	if report.Has(preflight.CheckWorktreeClean) {
		t.Errorf("Check report still carries CheckWorktreeClean after the seed commit; the bootstrap must reach past the first precondition row rather than block on it")
	}
}

// mustGitSmoke runs a git subcommand against dir, failing the test on any non-zero exit -- the plain
// direct-exec shape smoke tests already use for weftCommitCount/weftHeadChangedFiles, extended here to
// a general run-and-fail helper for the legacy-pair rig below.
func mustGitSmoke(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git -C %s %v: %v; output: %s", dir, args, err, out)
	}
}

// (g2) the regression guard for the origin-record self-healing gap the holistic review found: a
// legacy pair whose provenance record was written to disk (step 1 of "loom run") but never committed
// weft-side (step 3), because the process died in between. The very next "loom run" must find the
// record already present on disk with a matching value -- resolveParentBranch's ordinary re-run row,
// requesting no write of its own -- and still commit the still-untracked record, exactly as the status
// file already self-heals, rather than leaving it stranded as an untracked file that permanently fails
// fabricengine.Clean's own first Preflight precondition row.
func TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, slug := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	weftDir := fabricengine.WeftWorktree(loc)
	originRel := filepath.Join(loc.AnchorRel, fabricengine.OriginRecordRel())

	// Roll the pair back to a legacy shape: no origin record tracked at all, as if the pair had been
	// created before the record existed.
	mustGitSmoke(t, weftDir, "rm", "-q", "--", originRel)
	mustGitSmoke(t, weftDir, "commit", "-m", "smoke: simulate legacy pair with no origin record")

	// Simulate the crash: write the record straight to disk through the same production primitive
	// step 1 itself uses, but never commit it -- the exact state a process death between steps 1 and
	// 3 would leave behind.
	rec := fabricengine.NewMutations("")
	if err := fabricengine.WriteOrigin(rec, loc, slug, fabricengine.Origin{ParentBranch: "main"}); err != nil {
		t.Fatalf("WriteOrigin (simulated crash write): %v", err)
	}

	if status := weftPathspecStatus(t, weftDir, originRel); status == "" {
		t.Fatalf("origin record status before the healing run = clean; want the simulated crash to leave it uncommitted")
	}

	// The next "loom run" needs no --parent: ReadOrigin finds the just-written record on disk with a
	// matching value, so resolveParentBranch reports write == false -- the exact row the finding says
	// used to strand the record forever.
	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("healing loom run: %v; output: %s", err, stdout)
	}

	// The origin record specifically must now be committed and clean. The overall weft is not
	// asserted clean here: once a driver actually runs, its own persists rewrite the status file's
	// working-tree content on every phase transition without ever committing those rewrites (see
	// TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit's doc comment), which legitimately dirties
	// the weft again for a reason that has nothing to do with the origin-record healing this case
	// exists to pin.
	if status := weftPathspecStatus(t, weftDir, originRel); status != "" {
		t.Errorf("origin record status after the healing run = %q; want it committed and clean", status)
	}
}

// weftPathspecStatus returns dir's `git status --porcelain` output scoped to relPath, empty when
// relPath is fully committed and unchanged.
func weftPathspecStatus(t *testing.T, dir, relPath string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--", relPath).Output()
	if err != nil {
		t.Fatalf("git -C %s status --porcelain -- %s: %v", dir, relPath, err)
	}
	return strings.TrimSpace(string(out))
}

// (h) the spawn handshake -- two bootstrap invocations started concurrently produce exactly one
// driver process and no already-running refusal in the driver log. This is the other named regression
// guard: the pre-fix defect was the run lock being taken by the child long after the spawn call
// returned, which raced two concurrent "lyx loom run" invocations into each believing it must spawn
// its own driver.
func TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	// A background sampler polls the live driver-pid count for the whole duration both "loom run"
	// invocations are in flight, tracking the maximum ever observed -- a post-hoc single check after
	// both return cannot prove a double-spawn never happened transiently, since either or both
	// drivers may already have finished their own bounded bounce loop by the time a caller checks
	// only at the end. This is the mechanism assertion the card requires, not merely a happy outcome.
	stopSampling := make(chan struct{})
	var sampleWG sync.WaitGroup
	var maxConcurrentDrivers int
	sampleWG.Add(1)
	go func() {
		defer sampleWG.Done()
		for {
			select {
			case <-stopSampling:
				return
			default:
			}
			if n := len(findDriverPIDs(worktree)); n > maxConcurrentDrivers {
				maxConcurrentDrivers = n
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	const runners = 2
	outputs := make([]string, runners)
	runErrs := make([]error, runners)
	var wg sync.WaitGroup
	for i := 0; i < runners; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outputs[i], _, runErrs[i] = runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
		}(i)
	}
	wg.Wait()
	close(stopSampling)
	sampleWG.Wait()

	for i, err := range runErrs {
		if err != nil {
			t.Fatalf("concurrent run #%d: %v; output: %s", i, err, outputs[i])
		}
	}

	if maxConcurrentDrivers > 1 {
		t.Errorf("observed %d driver processes alive at once during the concurrent bootstrap; want at most 1 -- the bootstrap lock must serialize the spawn decision", maxConcurrentDrivers)
	}

	logContent, _ := os.ReadFile(loomengine.LoomDriverLog(loc))
	if strings.Contains(strings.ToLower(string(logContent)), "already running") {
		t.Errorf("driver log names an already-running refusal: %s", logContent)
	}
}

// (i) the handshake's died-child disposition -- a driver rigged to die immediately does NOT make
// the bootstrap refuse. dispositionForHandshake maps awaitRunLockChildDied onto proceed, and
// deliberately so: a child already gone before the handshake's first poll is a driver that RAN AND
// FINISHED, and refusing there would tell an operator the bootstrap broke when in fact their task
// halted, while skipping the tmux handover that is the one place the halt is legible. Only
// awaitRunLockDeadline -- a child still alive after the whole attempt budget, never having taken the
// lock -- is a genuine refusal, and a dying driver cannot produce that.
//
// This case used to assert the opposite, and failed against the shipped code because a poisoned
// status file kills the child rather than wedging it. What it pins now is the disposition that
// actually ships, plus the two properties that make it safe: the driver log carries the real reason
// the run halted, and the bootstrap lock is released rather than stranded.
//
// The rig is the poisoned-status-file mechanism poisonStatusFile documents, applied after a first,
// genuine bootstrap so the pair already has a live reed substrate and the run lock has been observed
// free again. The second bootstrap's own steps 1-4 use only the lenient read paths (ReadOrigin,
// Seed, CommitWeftPaths) that tolerate the poisoned field, so only the spawned CHILD's strict read
// gate ever sees the failure.
func TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	firstOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("first bootstrap: %v; output: %s", err, firstOut)
	}
	for _, pid := range findDriverPIDs(worktree) {
		_ = proc.KillPID(pid)
	}
	waitRunLockFree(t, loc, 20*time.Second)

	poisonStatusFile(t, loc)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "run")
	if err != nil {
		t.Fatalf("second (rigged) bootstrap: %v; output: %s", err, stdout)
	}

	// Proceeded to step 7 rather than refusing. In this test process there is no controlling
	// terminal, so the handover itself cannot succeed and tmux says so on stderr -- which is exactly
	// the evidence that the handover was REACHED. A refusal would have written a JSON envelope
	// instead and never got here.
	if !strings.Contains(stdout, "not a terminal") {
		t.Errorf("second bootstrap output = %q; want it to show the tmux handover being reached, not a refusal before it", stdout)
	}
	if strings.Contains(stdout, `"ok":false`) {
		t.Errorf("second bootstrap emitted a refusal envelope: %s -- a driver that died is a run that finished, not a broken bootstrap", stdout)
	}

	// The rig is confirmed by the driver log's own content: the poisoned read gate's decode failure,
	// not an ordinary completed run. Without this the case would pass against any fast-exiting
	// driver and prove nothing about the rig.
	wantLog := loomengine.LoomDriverLog(loc)
	driverLog, err := os.ReadFile(wantLog)
	if err != nil {
		t.Fatalf("read driver log %s: %v", wantLog, err)
	}
	if !strings.Contains(strings.ToLower(string(driverLog)), "decode") {
		t.Errorf("driver log = %q; want it to name the poisoned status file's decode failure -- the reason the run halted must survive in the log even though the bootstrap proceeded", driverLog)
	}

	fl, free, err := lock.TryAcquireWriteLock(loomengine.LoomBootstrapLock(loc))
	if err != nil {
		t.Fatalf("probe bootstrap lock: %v", err)
	}
	if !free {
		t.Errorf("bootstrap lock still held after the handover; want it released")
	} else {
		_ = fl.Release()
	}
}
