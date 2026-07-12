//go:build integration

// run_integration_test.go holds the run verb's weft-sync tests: each seeds a
// real paired git-repo fixture (lyxtest.CopyPairedLocal) and asserts on the
// actual weft git log/tracked-files via exec.Command, so this file is
// integration-tagged per the Test Tier Purity Invariant.

package perchcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestRunCLI_Run_WeftSyncRunsOnEngineError verifies that Engine.Run
// returning a hard error still gets the SAME weft commit+push treatment a
// successful terminal outcome does, per the Weft Git Invariant: perchcli is
// the loop owner regardless of how the block ended. A profile whose
// round-caps ladder fails Profile.validate (non-increasing entries) makes
// Engine.Run return an error deterministically, with no live mux/claude
// substrate needed — validate runs before the first round would ever spawn,
// so this test pre-seeds the run dir with a placeholder artifact (standing
// in for what a real partially-completed block, e.g. a completed round
// before a later could-not-start gate error, would have left behind) to
// prove the sync call actually runs and actually commits it on this path.
func TestRunCLI_Run_WeftSyncRunsOnEngineError(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real partially-completed block's leftover artifact:
	// Profile.validate fails before Engine.Run ever creates the run dir
	// itself, so a placeholder file is planted directly inside the
	// weft-prime worktree at the path the host's "_lyx" junction would
	// otherwise transparently resolve to (this fixture predates "lyx init",
	// so no junction exists yet — writing straight into WeftPrime is the
	// established pattern other cli test suites use, e.g. weftcli's
	// TestRunCLI_EnvMapToOption).
	placeholderDir := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "weft-on-error")
	if err := os.MkdirAll(placeholderDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(placeholderDir, "round-1-review.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write placeholder artifact: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "weft-on-error"})

	if exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate)`, exitCode)
	}
	if !strings.Contains(out.String(), "strictly increasing") {
		t.Fatalf(`RunCLI([run]) output missing the round-caps validation error; got: %q`, out.String())
	}

	weftLog := gitLogOneline(t, fixture.WeftPrime)
	if !strings.Contains(weftLog, "weft-on-error ERROR") {
		t.Errorf("weft log = %q; want a %q commit even though Engine.Run returned an error", weftLog, "perch: weft-on-error ERROR")
	}
}

// TestRunCLI_Run_WeftCommitExcludesLockFiles verifies the block-exit weft
// commit stages a run dir's real block state (state.json, round artifacts)
// but never its machine-local advisory-lock files (run.lock,
// state.json.lock): committing those would leak runtime noise into durable
// weft history and materialize stale lock files on every other machine's
// weft pull. Uses the same deterministic engine-error skeleton as
// TestRunCLI_Run_WeftSyncRunsOnEngineError — the sync path is identical for
// every outcome, so the cheapest deterministic exit exercises the pathspec.
func TestRunCLI_Run_WeftCommitExcludesLockFiles(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real block's run dir: state alongside the two lock
	// files a real Engine.Run leaves behind (see the WeftSyncRunsOnEngineError
	// test above for why this is planted straight into WeftPrime).
	runDir := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "lock-exclusion")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":        "{}",
		"round-1-review.md": "placeholder",
		"run.lock":          "",
		"state.json.lock":   "",
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	var out bytes.Buffer
	if exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "lock-exclusion"}); exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate), output: %s`, exitCode, out.String())
	}

	// The commit must carry the block state and nothing lock-shaped.
	tracked := gitLsFiles(t, fixture.WeftPrime)
	if !strings.Contains(tracked, "lock-exclusion/state.json\n") || !strings.Contains(tracked, "lock-exclusion/round-1-review.md") {
		t.Errorf("weft tracked files = %q; want state.json and round-1-review.md committed", tracked)
	}
	if strings.Contains(tracked, ".lock") {
		t.Errorf("weft tracked files = %q; want no *.lock file ever committed", tracked)
	}
}

// TestRunCLI_Run_BusyBlockSkipsWeftSync verifies that a run refused because
// another invocation holds the block's run.lock does NOT run the block-exit
// weft sync: the loser changed nothing on disk, and syncing would commit
// (and push) the WINNER's in-flight partial state under a misleading
// "perch: <id> ERROR" message. A dirty file is planted in the weft-side
// _lyx (standing in for the winner's mid-round state) to prove the sync
// would have had something to commit and still did not run.
func TestRunCLI_Run_BusyBlockSkipsWeftSync(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// The winner's in-flight state, planted straight into WeftPrime (this
	// fixture predates "lyx init", so no junction exists — same pattern as
	// the weft tests above). runDirBase resolves against the HOST cwd, so
	// hold the run.lock there; the dirty weft file proves the skipped sync
	// had real material.
	hostRunDir := filepath.Join(hubgeometry.PerchRunsDir(fixture.Hub), "busyblock")
	if err := os.MkdirAll(hostRunDir, 0o755); err != nil {
		t.Fatalf("mkdir host run dir: %v", err)
	}
	weftDirty := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "busyblock")
	if err := os.MkdirAll(weftDirty, 0o755); err != nil {
		t.Fatalf("mkdir weft dirty dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(weftDirty, "round-1-review.md"), []byte("winner's in-flight partial"), 0o644); err != nil {
		t.Fatalf("write weft dirty file: %v", err)
	}

	// Stand in for the winning invocation: hold the run.lock for the whole
	// losing call, exactly as a mid-round Engine.Run does.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(hostRunDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() = (%v, %v); want a held lock", locked, err)
	}
	defer runLock.Release()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "busyblock"})
	if exitCode != 1 {
		t.Fatalf(`RunCLI([run --run-id busyblock]) = %d; want 1, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already running") {
		t.Errorf(`RunCLI([run --run-id busyblock]) output missing "already running"; got: %q`, out.String())
	}

	weftLog := gitLogOneline(t, fixture.WeftPrime)
	if strings.Contains(weftLog, "busyblock") {
		t.Errorf("weft log = %q; want NO commit from the losing invocation (the winner syncs at its own exit)", weftLog)
	}
}

// gitLsFiles runs `git ls-files` inside dir and returns its output, failing
// the test loudly if git cannot be invoked.
func gitLsFiles(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "ls-files")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files in %q: %v (output: %s)", dir, err, output)
	}
	return string(output)
}

// gitLogOneline runs `git log --oneline` inside dir and returns its output,
// failing the test loudly if git cannot be invoked.
func gitLogOneline(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "--oneline")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log --oneline in %q: %v (output: %s)", dir, err, output)
	}
	return string(output)
}
