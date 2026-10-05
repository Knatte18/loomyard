//go:build tmux

// cli_tmux_test.go holds the one cli_integration_test.go test that starts a tmux server:
// the standalone `run` pre-run boots a real reed session before the plan-validation gate, so the test sits in the `tmux` tier.
// It follows the shape internal/reedcli/cli_integration_test.go already establishes for a tagged CLI-level test;
// this package's hermetic TestMain (testmain_test.go) is untagged, so no new test-main wiring is needed here.

package webstercli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate drives "run" from a temporary directory that is not a git repository at all -- lyxcwd.Resolve fails there, so preflight.HubPresent folds it into standalone mode rather than refusing outright.
// It redirects the standalone state directory to a temporary one via XDG_STATE_HOME (the per-OS variable standalonestate.Derive reads on Linux, the platform this test runs on) before calling RunCLIIn, since without that redirect the real Derive would resolve into the operator's actual home directory.
// The redirect is why this test is not marked t.Parallel(): t.Setenv panics under a parallel test.
//
// The seeded plan carries a card missing a required field, so once the pre-run itself succeeds "run" reaches its own automatic plan-validation gate (run.go's own Long documents this as "the automatic plan-validation gate") and refuses there -- proving the pre-run got all the way through wiring rather than dying earlier with a cwd-resolution error, which would produce a completely different message.
func TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	seedMissingIntentPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))
	tearDownStandaloneReed(t, target, stateDir, hash8)

	var out strings.Builder
	exitCode := RunCLIIn(target, &out, []string{"run"})

	if exitCode != 1 {
		t.Fatalf(`RunCLIIn(%q, [run]) = %d; want 1, output: %s`, target, exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "plan validation refused this run") {
		t.Errorf(`RunCLIIn(%q, [run]) output missing the plan-validation refusal; got: %q`, target, got)
	}
	if strings.Contains(got, "not a git repository") || strings.Contains(got, "ErrNotAGitRepo") || strings.Contains(got, "ErrCwdOutsideAnchor") {
		t.Errorf(`RunCLIIn(%q, [run]) output looks like a cwd-resolution failure, not the run verb's own validation gate; got: %q`, target, got)
	}
}

// tearDownStandaloneReed registers a t.Cleanup that brings down the standalone reed session this test's own invocation boots, addressed through the same standalonegeom.ReedGeometry the CLI's wiring derives.
//
// It exists because the `run` verb boots that session (wiring.go's reedUp seam) BEFORE websterengine.Run reaches the plan-validation gate this test asserts on, so the assertion cannot be made without a real tmux server coming up.
// Nothing else ever takes it down: the target it was derived from is a t.TempDir() that is deleted out from under it, and each execution derives a fresh hash8, so the leak does not even coalesce — it was one orphaned `tmux -L lyx-<hash8>` server per run of this suite, on every machine that ran it, forever (crucible round opus-medium-r5, R5-1).
//
// Registered BEFORE the invocation rather than after it, so a t.Fatal inside the assertions still tears the server down.
// Down's own error is reported rather than ignored: a cleanup that silently fails is how this leak would come back.
func tearDownStandaloneReed(t *testing.T, target, stateDir, hash8 string) {
	t.Helper()

	t.Cleanup(func() {
		// The config is whatever wiring seeded under the state dir;
		// a load failure means wiring never got that far, and the zero Config still addresses the right socket and session, which is all Down needs.
		cfg, err := reedengine.LoadConfig(stateDir, "reed")
		if err != nil {
			cfg = reedengine.Config{}
		}
		if _, err := reedengine.New(cfg, standalonegeom.ReedGeometry(target, stateDir, hash8)).Down(); err != nil {
			t.Errorf("tearing down the standalone reed session for %q: %v; a leaked tmux server survives this test", target, err)
		}
	})
}
