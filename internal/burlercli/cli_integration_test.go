//go:build integration

// cli_integration_test.go drives RunCLIIn against a temporary directory outside any git repository,
// proving the standalone pre-run reaches a verb's own RunE rather than failing with a
// cwd-resolution error -- the one property no untagged test in this package can observe, since it
// requires the real standalonestate.Derive, the real standalone stencil seed, and an end-to-end
// pre-run. It follows the shape internal/webstercli/cli_integration_test.go already establishes for
// a tagged CLI-level test; card 17's testmain_test.go supplies the hermetic git environment the
// `git rev-parse` inside preflight.ResolveMode needs, since this package spawns git from no test
// otherwise.

package burlercli

import (
	"os"
	"strings"
	"testing"
)

// TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate drives "run" from a temporary directory
// that is not a git repository at all -- lyxcwd.Resolve fails there, so preflight.ResolveMode folds
// it into standalone mode rather than refusing outright. It redirects the standalone state directory
// to a temporary one via XDG_STATE_HOME/LOCALAPPDATA before calling RunCLIIn, since without that
// redirect the real Derive would resolve into the operator's actual home directory. The redirect is
// why this test is not marked t.Parallel(): t.Setenv panics under a parallel test.
//
// No --profile is passed, so once the pre-run itself succeeds "run" reaches its own manual flag-shape
// gate and refuses there with "burler: --profile is required" -- proving the pre-run got all the way
// through wiring rather than dying earlier with a cwd-resolution error, which would produce a
// completely different message.
func TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate(t *testing.T) {
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
}

// TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged proves the two-roots split's whole point:
// the target directory itself -- the operator's git repository -- gains no hidden state tree, no lock
// file, and no rendered prompt from a standalone invocation. Every durable and scratch artifact lives
// under the derived state directory instead. This is the one property no untagged test in this batch
// can observe, since it requires a real Derive call and a real filesystem to assert an absence
// against.
func TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged(t *testing.T) {
	target := t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	before, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("ReadDir(%q) before = %v", target, err)
	}
	if len(before) != 0 {
		t.Fatalf("target directory %q is not empty before the invocation; fixture is not clean: %v", target, before)
	}

	var out strings.Builder
	// "run" without --profile is enough to drive the pre-run's full standalone wiring (including
	// the stencil seed) even though the verb itself refuses right after.
	_ = RunCLIIn(target, &out, []string{"run"})

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
