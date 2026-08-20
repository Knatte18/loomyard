//go:build integration

// run_integration_test.go holds the run verb's fabric-sync tests: each builds a real hub
// (hubforge.NewHub) and asserts on the actual fabric git log/tracked-files via exec.Command, so this
// file is integration-tagged per the Test Tier Purity Invariant.

package perchcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/perchengine"
)

// TestRunCLI_Run_FabricSyncRunsOnEngineError verifies that Engine.Run returning a hard error still
// gets the SAME fabric commit+push treatment a successful terminal outcome does, per the Fabric Git
// Invariant: perchcli is the loop owner regardless of how the block ended.
// A profile whose round-caps ladder fails Profile.validate (non-increasing entries) makes
// Engine.Run return an error deterministically, with no live reed/claude substrate needed —
// validate runs before the first round would ever spawn, so this test pre-seeds the run dir with a
// placeholder artifact (standing in for what a real partially-completed block, e.g.
// a completed round before a later could-not-start gate error, would have left behind) to prove the
// sync call actually runs and actually commits it on this path.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_Run_FabricSyncRunsOnEngineError(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

	profilePath := filepath.Join(h.PrimeWorktree(), "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real partially-completed block's leftover artifact:
	// Profile.validate fails before Engine.Run ever creates the run dir
	// itself, so a placeholder file is planted directly inside the
	// fabric-prime worktree at the path the warp's "_lyx" junction would
	// otherwise transparently resolve to (this fixture predates "lyx init",
	// so no junction exists yet — writing straight into PrimeWeft is the
	// established pattern other cli test suites use, e.g. fabriccli's
	// TestRunCLI_EnvMapToOption).
	placeholderDir := filepath.Join(h.PrimeWeft(), lyxdirs.LyxDirName, "perch", "fabric-on-error")
	if err := os.MkdirAll(placeholderDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(placeholderDir, "round-1-review.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write placeholder artifact: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"run", "--profile", profilePath, "--run-id", "fabric-on-error"})

	if exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate)`, exitCode)
	}
	if !strings.Contains(out.String(), "strictly increasing") {
		t.Fatalf(`RunCLI([run]) output missing the round-caps validation error; got: %q`, out.String())
	}

	fabricLog := gitLogOneline(t, h.PrimeWeft())
	if !strings.Contains(fabricLog, "fabric-on-error ERROR") {
		t.Errorf("fabric log = %q; want a %q commit even though Engine.Run returned an error", fabricLog, "perch: fabric-on-error ERROR")
	}
}

// TestRunCLI_Run_FabricCommitExcludesLockFiles verifies the block-exit fabric commit stages a run dir's
// real block state (state.json, round artifacts) but never its machine-local advisory-lock files
// (run.lock, state.json.lock): committing those would leak runtime noise into durable fabric history
// and materialize stale lock files on every other machine's fabric pull.
// Per the mirrored-subpath decision, run.lock/state.json.lock live under the block's ".lyx" scratch
// dir rather than its "_lyx" run dir, so the two lock files are planted there — outside the commit
// pathspec entirely, with no exclude-pattern machinery needed to keep them out.
// Uses the same deterministic engine-error skeleton as TestRunCLI_Run_FabricSyncRunsOnEngineError —
// the sync path is identical for every outcome, so the cheapest deterministic exit exercises the
// pathspec.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_Run_FabricCommitExcludesLockFiles(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

	profilePath := filepath.Join(h.PrimeWorktree(), "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real block's run dir (see the FabricSyncRunsOnEngineError
	// test above for why this is planted straight into PrimeWeft).
	runDir := filepath.Join(h.PrimeWeft(), lyxdirs.LyxDirName, "perch", "lock-exclusion")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":        "{}",
		"round-1-review.md": "placeholder",
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	// Stand in for the same block's ".lyx" scratch dir, where the two lock
	// files a real Engine.Run leaves behind actually live.
	scratchDir := filepath.Join(h.PrimeWeft(), lyxdirs.DotLyxDirName, "perch", "lock-exclusion")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder scratch dir: %v", err)
	}
	for name, content := range map[string]string{
		"run.lock":        "",
		"state.json.lock": "",
	} {
		if err := os.WriteFile(filepath.Join(scratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	var out bytes.Buffer
	if exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"run", "--profile", profilePath, "--run-id", "lock-exclusion"}); exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate), output: %s`, exitCode, out.String())
	}

	// The commit must carry the block state and nothing lock-shaped.
	tracked := gitLsFiles(t, h.PrimeWeft())
	if !strings.Contains(tracked, "lock-exclusion/state.json\n") || !strings.Contains(tracked, "lock-exclusion/round-1-review.md") {
		t.Errorf("fabric tracked files = %q; want state.json and round-1-review.md committed", tracked)
	}
	if strings.Contains(tracked, ".lock") {
		t.Errorf("fabric tracked files = %q; want no *.lock file ever committed", tracked)
	}
}

// TestRunCLI_Run_FabricCommitExcludesLockFiles_NestedRelPath is the regression guard perch lacked:
// TestRunCLI_Run_FabricCommitExcludesLockFiles above only exercises RelPath ".".
// This test proves the same "run.lock/state.json.lock live under .lyx, never under the committed
// _lyx pathspec" split still keeps the two lock files out of the commit while still landing the
// rest of the block state, with the worktree geometry itself anchored two segments deep.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_Run_FabricCommitExcludesLockFiles_NestedRelPath(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const relPath = "wts/some-task"
	h := hubforge.NewHub(t, relPath)
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

	// The anchored warp directory already exists in hubforge's own warp template (see hub.go's
	// buildBareTemplate), so nothing further needs arranging here beyond passing it as RunCLIIn's cwd.
	anchorCwd := h.Location.AnchorPath()

	profilePath := filepath.Join(h.PrimeWorktree(), "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real block's run dir, nested under the recorded anchor's subpath exactly as the
	// real fabric junction would mirror it. h.WeftBase is that anchor-joined weft directory, fabric's
	// own join rather than one built here from PrimeWeft()+relPath.
	runDir := filepath.Join(h.WeftBase, lyxdirs.LyxDirName, "perch", "nested-lock-exclusion")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":        "{}",
		"round-1-review.md": "placeholder",
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	// Stand in for the same block's ".lyx" scratch dir, nested the same way,
	// where the two lock files a real Engine.Run leaves behind actually live.
	scratchDir := filepath.Join(h.WeftBase, lyxdirs.DotLyxDirName, "perch", "nested-lock-exclusion")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder scratch dir: %v", err)
	}
	for name, content := range map[string]string{
		"run.lock":        "",
		"state.json.lock": "",
	} {
		if err := os.WriteFile(filepath.Join(scratchDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	var out bytes.Buffer
	if exitCode := RunCLIIn(anchorCwd, &out, []string{"run", "--profile", profilePath, "--run-id", "nested-lock-exclusion"}); exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate), output: %s`, exitCode, out.String())
	}

	// The commit must carry the block state and nothing lock-shaped, even
	// though the whole worktree is anchored two RelPath segments deep.
	tracked := gitLsFiles(t, h.PrimeWeft())
	wantPresent := relPath + "/_lyx/perch/nested-lock-exclusion/state.json"
	wantPresent2 := relPath + "/_lyx/perch/nested-lock-exclusion/round-1-review.md"
	if !strings.Contains(tracked, wantPresent) || !strings.Contains(tracked, wantPresent2) {
		t.Errorf("fabric tracked files = %q; want %q and %q committed", tracked, wantPresent, wantPresent2)
	}
	if strings.Contains(tracked, ".lock") {
		t.Errorf("fabric tracked files = %q; want no *.lock file ever committed, even nested under %q", tracked, relPath)
	}
}

// TestRunCLI_Run_BusyBlockSkipsFabricSync verifies that a run refused because another invocation
// holds the block's run.lock does NOT run the block-exit fabric sync: the loser changed nothing on
// disk,
// and syncing would commit (and push) the WINNER's in-flight partial state under a misleading
// "perch: <id> ERROR" message.
// A dirty file is planted in the fabric-side _lyx (standing in for the winner's mid-round state) to
// prove the sync would have had something to commit and still did not run.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_Run_BusyBlockSkipsFabricSync(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

	profilePath := filepath.Join(h.PrimeWorktree(), "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// The winner's in-flight state, planted straight into PrimeWeft (this
	// fixture predates "lyx init", so no junction exists — same pattern as
	// the fabric tests above). runDirBase resolves against the WARP cwd, so
	// hold the run.lock there; the dirty fabric file proves the skipped sync
	// had real material.
	warpRunDir := filepath.Join(perchengine.RunsDir(h.Location.AnchorPath()), "busyblock")
	if err := os.MkdirAll(warpRunDir, 0o755); err != nil {
		t.Fatalf("mkdir warp run dir: %v", err)
	}
	// run.lock itself now lives in the block's .lyx scratch dir, not its
	// _lyx run dir — see perchengine.ScratchDir and
	// treadleengine.Options.ScratchDir.
	warpScratchDir := filepath.Join(perchengine.ScratchDir(h.Location.AnchorPath()), "busyblock")
	if err := os.MkdirAll(warpScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir warp scratch dir: %v", err)
	}
	fabricDirty := filepath.Join(h.PrimeWeft(), lyxdirs.LyxDirName, "perch", "busyblock")
	if err := os.MkdirAll(fabricDirty, 0o755); err != nil {
		t.Fatalf("mkdir fabric dirty dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fabricDirty, "round-1-review.md"), []byte("winner's in-flight partial"), 0o644); err != nil {
		t.Fatalf("write fabric dirty file: %v", err)
	}

	// Stand in for the winning invocation: hold the run.lock for the whole
	// losing call, exactly as a mid-round Engine.Run does.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(warpScratchDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() = (%v, %v); want a held lock", locked, err)
	}
	defer runLock.Release()

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"run", "--profile", profilePath, "--run-id", "busyblock"})
	if exitCode != 1 {
		t.Fatalf(`RunCLI([run --run-id busyblock]) = %d; want 1, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already running") {
		t.Errorf(`RunCLI([run --run-id busyblock]) output missing "already running"; got: %q`, out.String())
	}

	fabricLog := gitLogOneline(t, h.PrimeWeft())
	if strings.Contains(fabricLog, "busyblock") {
		t.Errorf("fabric log = %q; want NO commit from the losing invocation (the winner syncs at its own exit)", fabricLog)
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
