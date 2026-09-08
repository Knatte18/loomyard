//go:build integration

// sink_callsite_integration_test.go exercises the durable sink's cwd-anchored fallback THROUGH its
// real call site — logger.Info arming the sink via ensureDurableSink/armDurableSinkLocked with no
// directory override — against a real git repository on disk. sink_test.go's
// TestIsLyxWorktree_GatesTheCwdAnchoredFallback pins the isLyxWorktree helper directly, but nothing
// drove the actual wiring point R6-6's fix lives in: crucible round fable-high-r7 recorded that as
// a real coverage gap (D1) and this file closes it.
// It lives behind the integration tag because the fallback path runs lyxcwd.Resolve, which spawns
// git — banned from untagged files by the Test Tier Purity Invariant — and it is an EXTERNAL test
// package because gitkit (needed for the hermetic git environment) transitively imports logger.

package logger_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this binary spawns git, per
// CONSTRAINTS.md's Hermetic Git Test Environment Invariant. It compiles only under the integration
// tag, so the untagged logger test binary keeps its default TestMain.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}

// initPlainGitRepo turns dir into a plain git repository root and returns its symlink-resolved
// spelling — the spelling lyxcwd's own resolution produces, so path assertions below compare like
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
// (armDurableSinkLocked skips it entirely without LYX_TRACE=1). Not parallel-safe by construction:
// t.Chdir and t.Setenv both panic under t.Parallel.
func armFallbackSinkFrom(t *testing.T, repo string) {
	t.Helper()
	t.Chdir(repo)
	t.Setenv("LYX_TRACE", "1")
	logger.SetDurableSinkDir("")
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
}

// TestDurableSink_CwdFallbackNeverArmsInAPlainCheckout drives R6-6's fix at its call site: a plain
// git repository lyx does not own (no _lyx at its root) gets NO .lyx tree from an Info record —
// before the fix, every non-zero-exit refusal wrote <repo>/.lyx/logs/trace-*.log into the
// operator's own checkout.
func TestDurableSink_CwdFallbackNeverArmsInAPlainCheckout(t *testing.T) {
	repo := initPlainGitRepo(t, t.TempDir())
	armFallbackSinkFrom(t, repo)

	logger.Info("sink_callsite_integration_test: plain-checkout probe")

	if _, err := os.Stat(filepath.Join(repo, lyxdirs.DotLyxDirName)); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s/.lyx) error = %v; want IsNotExist — the cwd-anchored fallback armed inside a repository lyx does not own", repo, err)
	}
}

// TestDurableSink_CwdFallbackArmsInALyxOwnedWorktree is the mirror case: with _lyx present at the
// root, the same record must arm the fallback and land a trace file under <repo>/.lyx/logs — the
// worktree the fallback exists for.
func TestDurableSink_CwdFallbackArmsInALyxOwnedWorktree(t *testing.T) {
	repo := initPlainGitRepo(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(repo, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	armFallbackSinkFrom(t, repo)

	logger.Info("sink_callsite_integration_test: lyx-owned-worktree probe")

	matches, err := filepath.Glob(filepath.Join(repo, lyxdirs.DotLyxDirName, "logs", "trace-*.log"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) == 0 {
		t.Errorf("no trace-*.log under %s/.lyx/logs; want the cwd-anchored fallback armed — this is exactly the worktree it exists for", repo)
	}
}
