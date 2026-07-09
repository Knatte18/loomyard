// cli_test.go covers the perchcli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, and the help-tree Short completeness check. run and
// pause verb behavior (missing --profile, missing --run-id, strict profile
// decode, pause-flag mechanics) is covered by run_test.go (card 15) and the
// pause-verb tests appended to this file (card 16). Engine.Run itself is NOT
// exercised here — it needs a live mux/claude session; that coverage lives
// in the smoke test and the sandbox suite.

package perchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx perch" with no subcommand lists the
// run and pause verbs and exits 0 — no git repo is needed, since the
// PersistentPreRunE guard skips layout/config/engine resolution for the
// group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	for _, want := range []string{"run", "pause"} {
		if !strings.Contains(got, want) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", want, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "perch" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_GroupGuard_OutsideGitRepo asserts the PersistentPreRunE guard:
// bare "lyx perch" works outside a git repository, mirroring burlercli's
// guard rationale (neither the bare listing nor the unknown-subcommand path
// should require layout/config resolution).
func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

// TestCommand_EveryCommandHasShort walks the full perch command tree and
// asserts that every command — the parent group and every subcommand —
// carries a non-empty Short, per the CLI/Cobra Invariant.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Short == "" {
			t.Errorf("command %q has empty Short", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestRunCLI_Pause_MissingRunID verifies that "lyx perch pause" without
// --run-id fails with pause's own manual flag-shape error (not cobra's
// MarkFlagRequired) before ever touching c.runDirBase. This case runs
// against an uninitialized (non-git) directory, so PersistentPreRunE's own
// abort error is also present in the captured output alongside the
// flag-specific error line — the same documented double-failure shape as
// run_test.go's TestRunCLI_Run_MissingProfile.
func TestRunCLI_Pause_MissingRunID(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "--run-id is required") {
		t.Errorf(`RunCLI([pause]) output missing "--run-id is required"; got: %q`, out.String())
	}
}

// TestRunCLI_Pause_InvalidRunID verifies that a --run-id carrying a path
// separator (the class of value that would escape the perch runs directory
// via filepath.Join, e.g. "../elsewhere") is rejected loud before pause
// ever stats or writes anything, rather than resolving outside the perch
// runs area.
func TestRunCLI_Pause_InvalidRunID(t *testing.T) {
	seedPerchFixture(t)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause", "--run-id", "../../escaped"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause --run-id ../../escaped]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "lowercase alphanumerics and dashes only") {
		t.Errorf(`RunCLI([pause --run-id ../../escaped]) output missing the run-id shape error; got: %q`, out.String())
	}
	if _, err := os.Stat(filepath.Join("..", "..", "escaped")); err == nil {
		t.Error("a directory was created outside the perch runs area; --run-id validation did not prevent the escape")
	}
}

// seedPerchFixture returns a paired git-repo fixture with real shuttle/mux/
// perch config seeded, chdir'd into the host hub, ready for a "lyx perch
// pause" invocation. It never boots psmux or spawns a burler round — pause
// only stats/writes a flag file and never touches the resolved engine
// ingredients PersistentPreRunE stores.
func seedPerchFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	return fixture
}

// TestRunCLI_Pause_FinishedBlockRefused verifies that pausing a block whose
// state.json already records a terminal outcome fails loud naming that
// outcome, instead of reporting ok for a pause flag no run loop will ever
// observe (proven misleading live: a finished-STUCK block accepted a pause
// and the operator had no signal it could never be honored).
func TestRunCLI_Pause_FinishedBlockRefused(t *testing.T) {
	fixture := seedPerchFixture(t)

	runDir := filepath.Join(hubgeometry.PerchRunsDir(fixture.Hub), "finishedrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	// A terminal state.json exactly as a STUCK block exit persists it.
	stateContent := `{"profileHash":"h","roundCaps":[1],"rounds":[],"outcome":"STUCK","stuckReason":"hard-cap"}`
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write terminal state.json: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause", "--run-id", "finishedrun"})
	if exitCode != 1 {
		t.Fatalf(`RunCLI([pause --run-id finishedrun]) = %d; want 1, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already finished (STUCK)") {
		t.Errorf(`RunCLI([pause --run-id finishedrun]) output missing "already finished (STUCK)"; got: %q`, out.String())
	}
	if _, err := os.Stat(perchengine.PauseFlagPath(runDir)); err == nil {
		t.Error("pause flag was written for a finished block; want no flag")
	}
}

// TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd verifies the run-dir base
// is anchored at the INITIALIZED directory (layout.Cwd — where _lyx and the
// config dir live), not the git worktree root. lyx init is user-driven from
// any directory, so a repo may be initialized in a subdirectory of its git
// worktree (RelPath != "."); anchoring at WorktreeRoot there would resolve
// run dirs into an un-junctioned <gitroot>/_lyx that the weft commit's
// RelPath-scoped pathspec never includes, silently stranding every block
// artifact outside the weft. The pause verb's run-dir lookup exposes the
// resolved base: a run dir created under <cwd>/_lyx/perch must be found.
func TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)

	// Initialize a NESTED directory of the host repo, exactly as lyx init
	// run from <hub>/nested would: configs and _lyx live under nested/.
	nested := filepath.Join(fixture.Hub, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}
	lyxtest.SeedConfig(t, nested, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(nested)

	runDir := filepath.Join(hubgeometry.PerchRunsDir(nested), "nestedrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause", "--run-id", "nestedrun"})
	if exitCode != 0 {
		t.Fatalf(`RunCLI([pause --run-id nestedrun]) = %d; want 0 — the run dir under <cwd>/_lyx/perch must be found, output: %s`, exitCode, out.String())
	}
	if _, err := os.Stat(perchengine.PauseFlagPath(runDir)); err != nil {
		t.Errorf("pause flag not written under the nested _lyx run dir %q: %v", runDir, err)
	}
}

// TestRunCLI_Pause_NoSuchRun verifies that pausing a run-id whose run dir
// does not exist fails loud with a "no such run" error, rather than
// silently fabricating an empty run dir for a pause flag with nothing to
// pause.
func TestRunCLI_Pause_NoSuchRun(t *testing.T) {
	seedPerchFixture(t)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause", "--run-id", "does-not-exist"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause --run-id does-not-exist]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "no such run") {
		t.Errorf(`RunCLI([pause --run-id does-not-exist]) output missing "no such run"; got: %q`, out.String())
	}
}

// TestRunCLI_Pause_WritesFlagAndIsIdempotent verifies that pausing an
// existing run dir writes the pause flag file at
// perchengine.PauseFlagPath(runDir), succeeds, and that a second pause call
// against the same run-id is a no-op success (idempotent re-pause).
func TestRunCLI_Pause_WritesFlagAndIsIdempotent(t *testing.T) {
	fixture := seedPerchFixture(t)

	runDir := filepath.Join(hubgeometry.PerchRunsDir(fixture.Hub), "myrun")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause", "--run-id", "myrun"})
	if exitCode != 0 {
		t.Fatalf(`RunCLI([pause --run-id myrun]) = %d; want 0, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf(`RunCLI([pause --run-id myrun]) output missing ok:true envelope; got: %q`, out.String())
	}

	pauseFile := perchengine.PauseFlagPath(runDir)
	if _, err := os.Stat(pauseFile); err != nil {
		t.Fatalf("pause flag file %q not written: %v", pauseFile, err)
	}

	// Idempotent re-pause: calling pause again while the flag already
	// exists is a no-op success, not an error.
	var out2 bytes.Buffer
	exitCode2 := RunCLI(&out2, []string{"pause", "--run-id", "myrun"})
	if exitCode2 != 0 {
		t.Fatalf(`second RunCLI([pause --run-id myrun]) = %d; want 0, output: %s`, exitCode2, out2.String())
	}
	if !strings.Contains(out2.String(), `"ok":true`) {
		t.Errorf(`second RunCLI([pause --run-id myrun]) output missing ok:true envelope; got: %q`, out2.String())
	}
}
