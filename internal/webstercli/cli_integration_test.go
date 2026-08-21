//go:build integration

// cli_integration_test.go drives RunCLIIn against a temporary directory outside any git repository,
// proving the standalone pre-run reaches a verb's own RunE rather than failing with a
// cwd-resolution error -- the one property no untagged test in this package can observe, since it
// requires the real standalonestate.Derive, the real standalone stencil seed, and an end-to-end
// pre-run. It follows the shape internal/reedcli/cli_integration_test.go already establishes for a
// tagged CLI-level test; this package already carries a hermetic TestMain (testmain_test.go) and an
// existing tagged file (verbs_test.go), so no new test-main wiring is needed here.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate drives "run" from a temporary directory
// that is not a git repository at all -- lyxcwd.Resolve fails there, so preflight.HubPresent folds it
// into standalone mode rather than refusing outright. It redirects the standalone state directory to
// a temporary one via XDG_STATE_HOME (the per-OS variable standalonestate.Derive reads on Linux, the
// platform this test runs on) before calling RunCLIIn, since without that redirect the real Derive
// would resolve into the operator's actual home directory. The redirect is why this test is not
// marked t.Parallel(): t.Setenv panics under a parallel test.
//
// The seeded plan carries a card missing a required field, so once the pre-run itself succeeds "run"
// reaches its own automatic plan-validation gate (run.go's own Long documents this as "the automatic
// plan-validation gate") and refuses there -- proving the pre-run got all the way through wiring
// rather than dying earlier with a cwd-resolution error, which would produce a completely different
// message.
func TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())

	stateDir, _, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	seedMissingFieldPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

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

// TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged proves the two-roots split's whole point:
// the target directory itself -- the operator's git repository -- gains no hidden state tree, no lock
// file, and no rendered prompt from a standalone invocation. Every durable and scratch artifact lives
// under the derived state directory instead. This is the one property no untagged unit test in this
// batch observes, since it requires a real Derive call and a real filesystem to assert an absence
// against.
func TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())

	before, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("ReadDir(%q) before = %v", target, err)
	}
	if len(before) != 0 {
		t.Fatalf("target directory %q is not empty before the invocation; fixture is not clean: %v", target, before)
	}

	var out strings.Builder
	// "status" is enough to drive the pre-run's full standalone wiring (including the stencil seed)
	// without requiring a seeded plan directory -- status never reads the plan at all.
	_ = RunCLIIn(target, &out, []string{"status"})

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

// TestRunCLIIn_WronglyEnteredHub_Refuses is deviation four on the plan's hub byte-identity list:
// internal/webstercli/cli.go discarded ResolveMode's error class before this task, so entering a
// wired hub worktree from an ordinary subdirectory started a silent standalone session there. This
// invocation now refuses instead. It is a deliberate correction of shipped behaviour, which is why
// it is pinned rather than merely allowed.
//
// It builds a real wired hub via hubforge.NewHub, creates an ordinary subdirectory inside the prime
// worktree, and drives RunCLIIn from that subdirectory. "status" is chosen because it drives the
// full pre-run without requiring a seeded plan directory, matching the reasoning
// TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged already records for the same choice.
//
// It redirects XDG_STATE_HOME and LOCALAPPDATA to t.TempDir() values before the call and asserts
// both remain empty afterwards: this invocation must refuse before wireStandalone ever calls
// standalonestate.Derive, and without the redirect the emptiness assertion would be made against the
// operator's real state directory. The redirect means this test must not be marked t.Parallel() --
// t.Setenv panics under a parallel test.
func TestRunCLIIn_WronglyEnteredHub_Refuses(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	sub := filepath.Join(h.PrimeWorktree(), "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}

	xdgStateHome := t.TempDir()
	localAppData := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdgStateHome)
	t.Setenv("LOCALAPPDATA", localAppData)

	var out strings.Builder
	exitCode := RunCLIIn(sub, &out, []string{"status"})

	if exitCode == 0 {
		t.Fatalf("RunCLIIn(%q, [status]) = %d; want non-zero", sub, exitCode)
	}
	got := out.String()
	if !strings.Contains(got, lyxcwd.AnchorFileName) {
		t.Errorf("RunCLIIn(%q, [status]) output missing the gated cwd-anchor error naming %s; got: %q", sub, lyxcwd.AnchorFileName, got)
	}
	if strings.Contains(got, "\"stateDir\"") || strings.Contains(got, "\"mode\"") {
		t.Errorf("RunCLIIn(%q, [status]) output looks like a standalone session's own output, not a refusal; got: %q", sub, got)
	}

	assertEmptyDir(t, xdgStateHome)
	assertEmptyDir(t, localAppData)
}

// assertEmptyDir fails the test if dir contains any entry, proving no standalone state tree was
// created under it.
func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", dir, err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("%q gained entries: %v; want it to stay empty -- wireStandalone must never be reached on a refusal", dir, names)
	}
}
