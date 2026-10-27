//go:build tier1

// sink_callsite_integration_test.go exercises the durable sink's cwd-anchored fallback THROUGH its real call site — logger.Info arming the sink via ensureDurableSink/armDurableSinkLocked with no directory override — against a real git repository on disk.
// sink_test.go's TestIsLymWorktree_GatesTheCwdAnchoredFallback pins the isLymWorktree helper directly, but nothing drove the actual wiring point R6-6's fix lives in: crucible round fable-high-r7 recorded that as a real coverage gap (D1) and this file closes it.
// It lives behind the integration tag because the fallback path runs lymcwd.Resolve, which spawns git — banned from untagged files by the Test Tier Purity Invariant — and it is an EXTERNAL test package because gitkit (needed for the hermetic git environment) transitively imports logger.

package logger_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this binary spawns git, per
// PATTERN-test-isolation, then runs the tests under tmuxkit.Main.
// It compiles only under the integration tag, so the untagged logger test binary gets its TestMain from testmain_test.go.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}

// initPlainGitRepo turns dir into a plain git repository root and returns its symlink-resolved
// spelling — the spelling lymcwd's own resolution produces, so path assertions below compare like
// with like.
func initPlainGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v (%s)", dir, err, out)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", dir, err)
	}
	return resolved
}

// armFallbackSinkFrom points the process at repo, clears any directory override so the NEXT
// Info-or-above record takes the cwd-anchored fallback, and enables the fallback under `go test`
// (armDurableSinkLocked skips it entirely without LYM_TRACE=1). Not parallel-safe by construction:
// t.Chdir and t.Setenv both panic under t.Parallel.
func armFallbackSinkFrom(t *testing.T, repo string) {
	t.Helper()
	t.Chdir(repo)
	t.Setenv("LYM_TRACE", "1")
	logger.SetDurableSinkDir("")
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
}

// TestDurableSink_CwdFallbackFollowsLymOwnership drives R6-6's fix at its call site in one git repository, added to between its steps.
// The scenario's top-level test does not call t.Parallel, because its steps chdir and set environment variables, which t.Chdir and t.Setenv refuse under t.Parallel; the step after the first relies on the repository the first step left behind.
func TestDurableSink_CwdFallbackFollowsLymOwnership(t *testing.T) {
	repo := initPlainGitRepo(t, t.TempDir())

	// A plain git repository lym does not own (no _lym at its root) gets NO .lym tree from an Info record — before the fix, every non-zero-exit refusal wrote <repo>/.lym/logs/trace-*.log into the operator's own checkout.
	if !t.Run("a plain checkout is never armed", func(t *testing.T) {
		armFallbackSinkFrom(t, repo)

		logger.Info("sink_callsite_integration_test: plain-checkout probe")

		if _, err := os.Stat(filepath.Join(repo, lymdirs.DotLymDirName)); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s/.lym) error = %v; want IsNotExist — the cwd-anchored fallback armed inside a repository lym does not own", repo, err)
		}
	}) {
		return
	}

	// The mirror case, relying on the step above's repository: with _lym now present at the root, the same record must arm the fallback and land a trace file under <repo>/.lym/logs — the worktree the fallback exists for.
	t.Run("a lym-owned worktree is armed", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(repo, lymdirs.LymDirName), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		armFallbackSinkFrom(t, repo)

		logger.Info("sink_callsite_integration_test: lym-owned-worktree probe")

		// The cwd-anchored arm reports not redirected, the logs directory, and the repo as its anchor.
		want := logger.SinkArmState{
			Armed:      true,
			Redirected: false,
			Dir:        filepath.Join(repo, lymdirs.DotLymDirName, "logs"),
			AnchorPath: repo,
		}
		if got := logger.CurrentSinkArmState(); got != want {
			t.Errorf("CurrentSinkArmState() = %+v; want %+v", got, want)
		}

		matches, err := filepath.Glob(filepath.Join(repo, lymdirs.DotLymDirName, "logs", "trace-*.log"))
		if err != nil {
			t.Fatalf("Glob() error = %v", err)
		}
		if len(matches) == 0 {
			t.Errorf("no trace-*.log under %s/.lym/logs; want the cwd-anchored fallback armed — this is exactly the worktree it exists for", repo)
		}
	})
}
