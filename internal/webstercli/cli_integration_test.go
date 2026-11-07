// cli_integration_test.go drives RunCLIIn against a temporary directory outside any git repository.
// It proves the standalone pre-run leaves that directory untouched and that a wrongly entered hub refuses.
// Those properties need the real standalonestate.Derive, the real standalone stencil seed and an end-to-end pre-run;
// no untagged test in this package observes them.
// The test that boots a real reed session lives in cli_tmux_test.go.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged checks that a standalone run leaves the target untouched.
// The target is the operator's git repository, and it gains no state tree, lock file, or rendered prompt.
// Every durable and scratch artifact lives under the derived state directory instead.
// No untagged unit test observes this, since it needs a real Derive call and a real filesystem to assert an absence.
//
// It also checks that a trace-*.log file lands under the derived state directory's standalonegeom.LogsDir,
// and that its first line's worktree_root= field names target.
// This positive assertion fails against source that never redirects the sink, where nothing is written there at all.
// t.Setenv("LYM_TRACE", "1") lifts the durable sink's testing.Testing() suppression.
// Without that lift the sink stays closed for the whole process.
// The positive assertion would then pass for the wrong reason.
//
// The cleanliness assertion is scoped to "status", an invocation that reaches wiring.
// It does not extend to an invocation that fails before standalonestate.Derive.
// That covers a cobra flag-parsing failure, a root pre-run failure, or a Derive failure itself.
// Under the real lym binary, logger.NotifyExit force-arms the sink on a pre-redirect non-zero exit,
// and the cwd fallback then writes one trace file into the target over that bounded window.
// NotifyExit is called only from cmd/lym/main.go.
// This test drives RunCLIIn directly, so NotifyExit never runs in this process.
// The test therefore cannot claim the target always stays clean, since the shipped path can write into it.
func TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged(t *testing.T) {
	tiergate.Require(t, 1)
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("LYM_TRACE", "1")
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	stateDir, _, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}

	before, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("ReadDir(%q) before = %v", target, err)
	}
	if len(before) != 0 {
		t.Fatalf("target directory %q is not empty before the invocation; fixture is not clean: %v", target, before)
	}

	var out strings.Builder
	// "status" drives the pre-run's full standalone wiring, including the stencil seed.
	// It needs no seeded plan directory, since status never reads the plan.
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

	// "status" never logs at Info or above, so nothing arms the durable sink on its own.
	// The forced logger.Info call after the invocation shows where wireStandalone pointed the sink.
	// It uses the same mechanism as the sink-redirect tests in internal/webstercli/wiring_test.go.
	// Against source with no redirect, this call falls back to the process's real cwd.
	// That fallback goes through ensureDurableSink's dir=="" branch, which LYM_TRACE=1 unblocks.
	// The trace file then lands outside logsDir below, and the positive assertion fails.
	logger.Info("cli_integration_test: arm the sink")

	logsDir := standalonegeom.LogsDir(stateDir)
	matches, err := filepath.Glob(filepath.Join(logsDir, "trace-*.log"))
	if err != nil {
		t.Fatalf("glob %s: %v", logsDir, err)
	}
	if len(matches) == 0 {
		t.Fatalf("no trace-*.log file under %s; want the standalone invocation to have redirected the durable sink there", logsDir)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	if !strings.Contains(firstLine, "worktree_root="+target) {
		t.Errorf("trace file %s first line = %q; want it to carry worktree_root=%s", matches[0], firstLine, target)
	}
}

// TestRunCLIIn_WronglyEnteredHub_Refuses checks that entering a wired hub worktree from a subdirectory refuses.
// The invocation refuses rather than starting a standalone session in that subdirectory.
// The test pins that behaviour.
//
// It builds a real wired hub via hubforge.NewHub.
// It creates an ordinary subdirectory inside the prime worktree and drives RunCLIIn from that subdirectory.
// "status" drives the full pre-run and needs no seeded plan directory.
// It is the same choice as TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged.
//
// It redirects XDG_STATE_HOME and LOCALAPPDATA to t.TempDir() values and asserts that both stay empty afterwards.
// The invocation must refuse before wireStandalone calls standalonestate.Derive.
// Without the redirect, the emptiness assertion would run against the operator's real state directory.
// The redirect means this test must not be marked t.Parallel, since t.Setenv panics under a parallel test.
func TestRunCLIIn_WronglyEnteredHub_Refuses(t *testing.T) {
	tiergate.Require(t, 1)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
	if !strings.Contains(got, lymcwd.AnchorFileName) {
		t.Errorf("RunCLIIn(%q, [status]) output missing the gated cwd-anchor error naming %s; got: %q", sub, lymcwd.AnchorFileName, got)
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
