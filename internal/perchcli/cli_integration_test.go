//go:build integration

// cli_integration_test.go holds the perchcli pause tests that build a real hub (hubforge.NewHub) and
// write run-dir state on disk, so they are integration-tagged per the Test Tier Purity Invariant.

package perchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/perchengine"
)

// TestRunCLI_Pause_InvalidRunID verifies that a --run-id carrying a path separator (the class of
// value that would escape the perch runs directory via filepath.Join, e.g. "../elsewhere") is
// rejected loud before pause ever stats or writes anything, rather than resolving outside the perch
// runs area.
func TestRunCLI_Pause_InvalidRunID(t *testing.T) {
	t.Parallel()

	h := seedPerchFixture(t)

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"pause", "--run-id", "../../escaped"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause --run-id ../../escaped]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "lowercase alphanumerics and dashes only") {
		t.Errorf(`RunCLI([pause --run-id ../../escaped]) output missing the run-id shape error; got: %q`, out.String())
	}
	// Rewritten against an absolute path derived from the hub, since this test no longer chdirs into
	// h.PrimeWorktree(): proving "../../escaped" did not escape the perch runs area no longer relies
	// on a process-relative Stat, which would be meaningless once cwd is passed per call instead of
	// held by the process.
	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "..", "..", "escaped")); err == nil {
		t.Error("a directory was created outside the perch runs area; --run-id validation did not prevent the escape")
	}
}

// seedPerchFixture returns a real hub. shuttle/reed/perch config is no longer seeded here --
// fabriccli.CloneAndWire already materializes every registered module's plain template.
func seedPerchFixture(t *testing.T) *hubforge.Hub {
	t.Helper()

	return hubforge.NewHub(t, ".")
}

// TestRunCLI_Pause_FinishedBlockRefused verifies that pausing a block whose state.json already
// records a terminal outcome fails loud naming that outcome, instead of reporting ok for a pause
// flag no run loop will ever observe (proven misleading live: a finished-STUCK block accepted a
// pause and the operator had no signal it could never be honored).
func TestRunCLI_Pause_FinishedBlockRefused(t *testing.T) {
	t.Parallel()

	h := seedPerchFixture(t)

	runDir := filepath.Join(perchengine.RunsDir(h.Location.AnchorPath()), "finishedrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	// A terminal state.json exactly as a STUCK block exit persists it.
	stateContent := `{"profileHash":"h","roundCaps":[1],"rounds":[],"outcome":"STUCK","stuckReason":"hard-cap"}`
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write terminal state.json: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"pause", "--run-id", "finishedrun"})
	if exitCode != 1 {
		t.Fatalf(`RunCLI([pause --run-id finishedrun]) = %d; want 1, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already finished (STUCK)") {
		t.Errorf(`RunCLI([pause --run-id finishedrun]) output missing "already finished (STUCK)"; got: %q`, out.String())
	}
	scratchDir := filepath.Join(perchengine.ScratchDir(h.Location.AnchorPath()), "finishedrun")
	if _, err := os.Stat(perchengine.PauseFlagPath(scratchDir)); err == nil {
		t.Error("pause flag was written for a finished block; want no flag")
	}
}

// TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd verifies the run-dir base is anchored at the
// INITIALIZED directory (layout.Cwd — where _lyx and the config dir live), not the git worktree
// root.
// lyx init is user-driven from any directory, so a repo may be initialized in a subdirectory of its
// git worktree (RelPath != "."); anchoring at WorktreeRoot there would resolve run dirs into an
// un-junctioned <gitroot>/_lyx that the fabric commit's RelPath-scoped pathspec never includes,
// silently stranding every block artifact outside fabric.
// The pause verb's run-dir lookup exposes the resolved base: a run dir created under
// <cwd>/_lyx/perch must be found.
func TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd(t *testing.T) {
	t.Parallel()

	// Build the hub at the real "nested" anchor: fabriccli.CloneAndWire records that anchor at
	// BoardDir for real, which is the entire point of this migration -- a hand-rolled anchor marker
	// on top of a real hub would be asserting against an invented shape again.
	h := hubforge.NewHub(t, "nested")

	runDir := filepath.Join(perchengine.RunsDir(h.Location.AnchorPath()), "nestedrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLIIn(h.Location.AnchorPath(), &out, []string{"pause", "--run-id", "nestedrun"})
	if exitCode != 0 {
		t.Fatalf(`RunCLI([pause --run-id nestedrun]) = %d; want 0 — the run dir under <cwd>/_lyx/perch must be found, output: %s`, exitCode, out.String())
	}
	scratchDir := filepath.Join(perchengine.ScratchDir(h.Location.AnchorPath()), "nestedrun")
	if _, err := os.Stat(perchengine.PauseFlagPath(scratchDir)); err != nil {
		t.Errorf("pause flag not written under the nested .lyx run dir %q: %v", scratchDir, err)
	}
}

// TestRunCLI_Pause_NoSuchRun verifies that pausing a run-id whose run dir does not exist fails loud
// with a "no such run" error, rather than silently fabricating an empty run dir for a pause flag
// with nothing to pause.
func TestRunCLI_Pause_NoSuchRun(t *testing.T) {
	t.Parallel()

	h := seedPerchFixture(t)

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"pause", "--run-id", "does-not-exist"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause --run-id does-not-exist]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "no such run") {
		t.Errorf(`RunCLI([pause --run-id does-not-exist]) output missing "no such run"; got: %q`, out.String())
	}
}

// TestRunCLI_Pause_WritesFlagAndIsIdempotent verifies that pausing an existing run dir writes the
// pause flag file at perchengine.PauseFlagPath(scratchDir) — the block's .lyx scratch dir, never
// its _lyx run dir — succeeds, and that a second pause call against the same run-id is a no-op
// success (idempotent re-pause).
func TestRunCLI_Pause_WritesFlagAndIsIdempotent(t *testing.T) {
	t.Parallel()

	h := seedPerchFixture(t)

	runDir := filepath.Join(perchengine.RunsDir(h.Location.AnchorPath()), "myrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"pause", "--run-id", "myrun"})
	if exitCode != 0 {
		t.Fatalf(`RunCLI([pause --run-id myrun]) = %d; want 0, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf(`RunCLI([pause --run-id myrun]) output missing ok:true envelope; got: %q`, out.String())
	}

	scratchDir := filepath.Join(perchengine.ScratchDir(h.Location.AnchorPath()), "myrun")
	pauseFile := perchengine.PauseFlagPath(scratchDir)
	if _, err := os.Stat(pauseFile); err != nil {
		t.Fatalf("pause flag file %q not written: %v", pauseFile, err)
	}
	// The load-bearing assertion: the flag lives under the .lyx perch tree,
	// never under the _lyx perch tree it used to share with state.json.
	if _, err := os.Stat(filepath.Join(runDir, perchengine.PauseFlagName)); err == nil {
		t.Errorf("pause flag also exists under the _lyx run dir %q; want it exclusively under the .lyx scratch dir", runDir)
	}

	// Idempotent re-pause: calling pause again while the flag already
	// exists is a no-op success, not an error.
	var out2 bytes.Buffer
	exitCode2 := RunCLIIn(h.PrimeWorktree(), &out2, []string{"pause", "--run-id", "myrun"})
	if exitCode2 != 0 {
		t.Fatalf(`second RunCLI([pause --run-id myrun]) = %d; want 0, output: %s`, exitCode2, out2.String())
	}
	if !strings.Contains(out2.String(), `"ok":true`) {
		t.Errorf(`second RunCLI([pause --run-id myrun]) output missing ok:true envelope; got: %q`, out2.String())
	}
}
