// sink_callsite_integration_test.go exercises the durable sink's cwd-anchored fallback through its real call site.
// It arms the sink through logger.Info with no directory override, against a real git repository on disk.
// sink_test.go pins the isLymWorktree helper directly; this file covers the call site that uses it.
// The file carries the integration tag because the fallback path runs lymcwd.Resolve, which spawns git.
// Untagged test files may not spawn git.
// It is an external test package: gitkit, which provides the hermetic git environment, imports logger transitively.

package logger_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this binary spawns git, per PATTERN-test-isolation.
// It then runs the tests under tmuxkit.Main.
// It compiles only under the integration tag; the untagged logger test binary gets its TestMain from testmain_test.go.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}

// initPlainGitRepo turns dir into a plain git repository root and returns its symlink-resolved spelling.
// That spelling is the one lymcwd's own resolution produces, so path assertions compare like with like.
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

// armFallbackSinkFrom points the process at repo and clears the directory override.
// The next Info-or-above record then takes the cwd-anchored fallback.
// It sets LYM_TRACE=1 under `go test`, since armDurableSinkLocked skips the fallback entirely without it.
// It is not parallel-safe: t.Chdir and t.Setenv both panic under t.Parallel.
func armFallbackSinkFrom(t *testing.T, repo string) {
	t.Helper()
	t.Chdir(repo)
	t.Setenv("LYM_TRACE", "1")
	logger.SetDurableSinkDir("")
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
}

// TestDurableSink_CwdFallbackFollowsLymOwnership drives the cwd-anchored fallback through its call site.
// Its steps run in order on one git repository, and the second relies on the first leaving it behind.
// The test does not call t.Parallel.
// Its steps chdir and set environment variables, which t.Chdir and t.Setenv refuse under t.Parallel.
func TestDurableSink_CwdFallbackFollowsLymOwnership(t *testing.T) {
	tiergate.Require(t, 1)
	repo := initPlainGitRepo(t, t.TempDir())

	// A plain git repository lym does not own (no _lym at its root) gets no .lym tree from an Info record.
	// Otherwise an Info record writes <repo>/.lym/logs/trace-*.log into the operator's own checkout.
	if !t.Run("a plain checkout is never armed", func(t *testing.T) {
		armFallbackSinkFrom(t, repo)

		logger.Info("sink_callsite_integration_test: plain-checkout probe")

		if _, err := os.Stat(filepath.Join(repo, lymdirs.DotLymDirName)); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s/.lym) error = %v; want IsNotExist — the cwd-anchored fallback armed inside a repository lym does not own", repo, err)
		}
	}) {
		return
	}

	// Once _lym is at the root, the same record must arm the fallback and land a trace file under <repo>/.lym/logs.
	// That is the worktree case the fallback exists for.
	t.Run("a lym-owned worktree is armed", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(repo, lymdirs.LymDirName), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		armFallbackSinkFrom(t, repo)

		logger.Info("sink_callsite_integration_test: lym-owned-worktree probe")

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
