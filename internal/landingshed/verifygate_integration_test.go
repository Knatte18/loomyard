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

func TestVerifyGate_RealVerifytreeLog(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		wantReason bool
		wantOutput string
	}{
		{"pass", "echo verify-ok", false, "verify-ok"},
		{"fail", "echo verify-broken; exit 3", true, "verify-broken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, _ := newGateScratch(t)
			deps.VerifyCommand = func() (string, error) { return tc.command, nil }
			gate := newVerifyGate(deps)
			reason, err := gate.check(context.Background(), "Publish", "main")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (reason != "") != tc.wantReason {
				t.Fatalf("reason = %q, wantReason = %v", reason, tc.wantReason)
			}
			got, err := os.ReadFile(gate.paths.Log)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), tc.wantOutput) {
				t.Fatalf("log = %q, want it to contain %q", got, tc.wantOutput)
			}
		})
	}
}

// TestVerifyGate_InterruptedVerifyRunsAgain pins the crash case: a verify cut off before the record write leaves a mismatch, so the next check runs it.
func TestVerifyGate_InterruptedVerifyRunsAgain(t *testing.T) {
	deps, counter := newGateScratch(t)
	gate := newVerifyGate(deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gate.check(ctx, "Publish", "main"); err == nil {
		t.Fatal("check on a cancelled context: want an error")
	}
	if got := runs(t, counter); got != 0 {
		t.Fatalf("interrupted verify ran the command %d time(s); want 0", got)
	}

	reason, err := gate.check(context.Background(), "Publish", "main")
	if reason != "" || err != nil {
		t.Fatalf("resumed check = (%q, %v); want (\"\", nil)", reason, err)
	}
	if got := runs(t, counter); got != 1 {
		t.Fatalf("resumed check ran the command %d time(s); want 1", got)
	}
}

// TestVerifyGate_NoOpMergeSkipsVerify pins that a second check over the tree the first one verified runs nothing.
func TestVerifyGate_NoOpMergeSkipsVerify(t *testing.T) {
	deps, counter := newGateScratch(t)
	gate := newVerifyGate(deps)
	for i := 0; i < 2; i++ {
		if reason, err := gate.check(context.Background(), "Finalize", "main"); reason != "" || err != nil {
			t.Fatalf("check %d = (%q, %v); want (\"\", nil)", i, reason, err)
		}
	}
	if got := runs(t, counter); got != 1 {
		t.Fatalf("command ran %d time(s) over a verified tree; want 1", got)
	}
}

// TestVerifyGate_NewCommitVerifiesAgain pins that a tree the record does not name is verified, as after a merge that brought in parent commits.
func TestVerifyGate_NewCommitVerifiesAgain(t *testing.T) {
	deps, counter := newGateScratch(t)
	gate := newVerifyGate(deps)
	if _, err := gate.check(context.Background(), "Publish", "main"); err != nil {
		t.Fatal(err)
	}
	gitkit.CommitFile(t, deps.WorktreeRoot, "b.txt", "b\n", "parent progress")
	if _, err := gate.check(context.Background(), "Publish", "main"); err != nil {
		t.Fatal(err)
	}
	if got := runs(t, counter); got != 2 {
		t.Fatalf("command ran %d time(s) over two trees; want 2", got)
	}
}

// TestVerifyGate_CleanSeesUntrackedFile pins that the real clean-tree seam reports an untracked file.
func TestVerifyGate_CleanSeesUntrackedFile(t *testing.T) {
	deps, _ := newGateScratch(t)
	gate := newVerifyGate(deps)
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
}
