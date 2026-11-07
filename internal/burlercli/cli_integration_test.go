// cli_integration_test.go drives RunCLIIn against a temporary directory outside any git repository,
// proving the standalone pre-run reaches a verb's own RunE rather than failing with a
// cwd-resolution error.
// Only this test observes that property.
// It needs the real standalonestate.Derive, the real standalone stencil seed, and an end-to-end pre-run.
// The hermetic git environment for `git rev-parse` in preflight.ResolveMode comes from testmain_test.go.
// No other test in this package spawns git.

package burlercli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGateAndLeavesTargetUntouched drives "run".
// It runs from a temporary directory that is not a git repository.
// lymcwd.Resolve fails there, so preflight.ResolveMode folds the invocation into standalone mode instead of refusing it.
// The test redirects the standalone state directory to a temporary one via XDG_STATE_HOME/LOCALAPPDATA.
// Without the redirect, the real Derive would resolve into the operator's home directory.
// That redirect is why the test is not marked t.Parallel(), since t.Setenv panics under a parallel test.
//
// No --profile is passed, so once the pre-run succeeds, "run" reaches its own flag-shape gate.
// That gate refuses with "burler: --profile is required", which shows the pre-run wired through;
// a cwd-resolution failure would produce a different message.
//
// The target directory, the operator's git repository, gains no state tree, lock file or rendered prompt.
// Every durable and scratch artifact lives under the derived state directory instead;
// the test needs a real Derive call and a real filesystem to assert that absence.
func TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGateAndLeavesTargetUntouched(t *testing.T) {
	tiergate.Require(t, 1)
	target := t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	var out strings.Builder
	exitCode := RunCLIIn(target, &out, []string{"run"})

	if exitCode != 1 {
		t.Fatalf(`RunCLIIn(%q, [run]) = %d; want 1, output: %s`, target, exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "burler: --profile is required") {
		t.Errorf(`RunCLIIn(%q, [run]) output missing the run verb's own flag-validation error; got: %q`, target, got)
	}
	if strings.Contains(got, "not a git repository") {
		t.Errorf(`RunCLIIn(%q, [run]) output looks like a cwd-resolution failure, not the run verb's own validation gate; got: %q`, target, got)
	}

	after, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("ReadDir(%q) after = %v", target, err)
	}
	if len(after) != 0 {
		names := make([]string, len(after))
		for i, e := range after {
			names[i] = e.Name()
		}
		t.Errorf("target directory %q gained entries from a standalone invocation: %v; want it byte-for-byte unchanged -- no hidden state tree, no lock file, no rendered prompt", target, names)
	}
}
