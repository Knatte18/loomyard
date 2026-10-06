//go:build integration

// verifygate_integration_test.go drives newVerifyGate's real verifytree functions against a scratch repo and real shell commands,
// and pins the record-keyed behaviour the unit tests fake: the log holds the command's output on a pass and a fail,
// a verify interrupted before the record write runs again, and a no-op merge onto a verified tree skips it.

package landingshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// newGateScratch returns a Deps over a fresh one-commit repo whose verify directory and counter file lie outside the worktree,
// with a verify command that appends one line to the counter file, so a test counts how often the command ran.
func newGateScratch(t *testing.T) (deps Deps, counter string) {
	t.Helper()
	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")

	counter = filepath.Join(t.TempDir(), "runs")
	deps = newTestDeps(t)
	deps.WorktreeRoot = worktree
	deps.VerifyDir = t.TempDir()
	deps.VerifyCommand = func() (string, error) { return "echo run >> " + counter, nil }
	return deps, counter
}

// runs returns how many times the counting command has run.
func runs(t *testing.T, counter string) int {
	t.Helper()
	data, err := os.ReadFile(counter)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "run")
}

// TestVerifyGate_RealVerifytreeScenario drives newVerifyGate's real verifytree functions through one scratch repo, a step at a time,
// and pins the record-keyed behaviour the unit tests fake: a verify cut off before the record write runs again,
// the log holds the command's output on a pass and a fail, a check over the tree already verified runs nothing,
// a tree the record does not name is verified, and the clean-tree seam sees an untracked file.
//
// The steps run in order on one repo and one verified-tree record, so no step runs in parallel and each relies on the state the one before it left;
// the top-level test calls t.Parallel because the repo is its own.
func TestVerifyGate_RealVerifytreeScenario(t *testing.T) {
	t.Parallel()

	deps, counter := newGateScratch(t)
	command := "echo verify-ok; echo run >> " + counter
	deps.VerifyCommand = func() (string, error) { return command, nil }
	gate := newVerifyGate(deps)
	check := func(ctx context.Context) (string, error) { return gate.check(ctx, "Publish", "main") }
	requireRuns := func(t *testing.T, want int) {
		t.Helper()
		if got := runs(t, counter); got != want {
			t.Fatalf("command ran %d time(s); want %d", got, want)
		}
	}
	requireLogHas := func(t *testing.T, want string) {
		t.Helper()
		got, err := os.ReadFile(gate.paths.Log)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), want) {
			t.Fatalf("log = %q, want it to contain %q", got, want)
		}
	}

	// A verify cut off before the record write leaves a mismatch, so the next check runs it.
	if !t.Run("interrupted verify runs nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := check(ctx); err == nil {
			t.Fatal("check on a cancelled context: want an error")
		}
		requireRuns(t, 0)
	}) {
		return
	}

	// Relies on the cancelled step having left no record for this tree.
	if !t.Run("resumed check runs the command once and logs its output", func(t *testing.T) {
		if reason, err := check(context.Background()); reason != "" || err != nil {
			t.Fatalf("resumed check = (%q, %v); want (\"\", nil)", reason, err)
		}
		requireRuns(t, 1)
		requireLogHas(t, "verify-ok")
	}) {
		return
	}

	// Relies on the resumed step having recorded this tree as verified.
	if !t.Run("a tree already verified runs nothing", func(t *testing.T) {
		if reason, err := check(context.Background()); reason != "" || err != nil {
			t.Fatalf("check over a verified tree = (%q, %v); want (\"\", nil)", reason, err)
		}
		requireRuns(t, 1)
	}) {
		return
	}

	// A new commit gives a tree the record does not name, as after a merge that brought in parent commits.
	if !t.Run("a new commit verifies again and a failing command is logged", func(t *testing.T) {
		gitkit.CommitFile(t, deps.WorktreeRoot, "b.txt", "b\n", "parent progress")
		command = "echo verify-broken; echo run >> " + counter + "; exit 3"
		reason, err := check(context.Background())
		if err != nil || reason == "" {
			t.Fatalf("check over a new tree with a failing command = (%q, %v); want a failure reason", reason, err)
		}
		requireRuns(t, 2)
		requireLogHas(t, "verify-broken")
	}) {
		return
	}

	// Relies on the previous step's commit leaving the worktree clean.
	t.Run("the clean-tree seam sees an untracked file", func(t *testing.T) {
		if reason, err := gate.clean("Publish", "before the merge-in"); reason != "" || err != nil {
			t.Fatalf("clean tree: (%q, %v); want (\"\", nil)", reason, err)
		}
		if err := os.WriteFile(filepath.Join(deps.WorktreeRoot, "stray.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		reason, err := gate.clean("Publish", "before the merge-in")
		if err != nil || !strings.Contains(reason, "stray.txt") {
			t.Fatalf("dirty tree: (%q, %v); want a reason naming stray.txt", reason, err)
		}
	})
}
