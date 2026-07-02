//go:build integration

// sync_test.go — tests for weft git operations (commit, push, pull).

package weftengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestCommit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		opts           SyncOptions
		modify         bool
		wantCommitted  bool
		hasStagedInLyx bool
	}{
		{"StagedChanges", SyncOptions{}, true, true, true},
		{"CleanTree", SyncOptions{}, false, false, false},
		{"SkipGit", SyncOptions{SkipGit: true}, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath

			if tt.modify {
				// Modify a file in the pathspec
				lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
				if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}

				// Create a stray file at the repo root (not in pathspec)
				strayFile := filepath.Join(weftRepo, "stray.txt")
				if err := os.WriteFile(strayFile, []byte("stray"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			// Commit only the _lyx pathspec
			committed, err := Commit(weftRepo, []string{"_lyx"}, DefaultCommitMessage, tt.opts)
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}

			if committed != tt.wantCommitted {
				t.Errorf("Commit() = %v; want %v", committed, tt.wantCommitted)
			}

			if tt.hasStagedInLyx {
				// Verify _lyx changes are committed
				cmd := exec.Command("git", "show", "HEAD:_lyx/config.yaml")
				cmd.Dir = weftRepo
				output, err := cmd.Output()
				if err != nil {
					t.Fatalf("git show HEAD:_lyx/config.yaml: %v", err)
				}
				if string(output) != "modified" {
					t.Errorf("committed content = %q; want %q", string(output), "modified")
				}
			}

			if tt.modify {
				// Verify stray.txt is still untracked (only in StagedChanges case)
				if tt.wantCommitted {
					cmd := exec.Command("git", "status", "--porcelain")
					cmd.Dir = weftRepo
					output, err := cmd.Output()
					if err != nil {
						t.Fatalf("git status: %v", err)
					}
					status := string(output)
					if !strings.Contains(status, "stray.txt") {
						t.Errorf("stray.txt should be untracked after commit; git status: %q", status)
					}
				}
			}
		})
	}
}

func TestCommit_ScopedPathspec(t *testing.T) {
	t.Parallel()

	// Pure-function assertion: at ".", ["_lyx"] → ["_lyx"].
	pathspec := ScopedPathspec(".", []string{"_lyx"})
	if len(pathspec) != 1 || pathspec[0] != "_lyx" {
		t.Errorf("ScopedPathspec(\".\", [\"_lyx\"]) = %v; want [_lyx]", pathspec)
	}

	// Behavioural: committing via the scoped pathspec stages the _lyx change.
	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, ScopedPathspec(".", []string{"_lyx"}), DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Errorf("Commit() with ScopedPathspec = false; want true")
	}
}

// TestCommit_CustomMessage asserts that a custom message passed to Commit is what
// lands in the weft repo's history, not DefaultCommitMessage — this is the behavior
// the --undo path (a different package) relies on to record a distinct commit
// message for its weft-side deletion.
func TestCommit_CustomMessage(t *testing.T) {
	t.Parallel()
	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	const customMessage = "custom test message"
	committed, err := Commit(weftRepo, []string{"_lyx"}, customMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should have succeeded")
	}

	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = weftRepo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%s: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != customMessage {
		t.Errorf("commit message = %q; want %q", got, customMessage)
	}
}

// TestCommit_PathspecAlreadyRemoved verifies that Commit tolerates a
// pathspec that has already been fully removed from both the working tree
// and the git index by a prior commit -- the case a caller hits when it
// unconditionally re-invokes Commit against a since-cleared pathspec (e.g.
// `lyx init --undo` run a second time after the first run already committed
// the _lyx deletion). It must report (false, nil), the same "nothing to
// stage" contract as the CleanTree case in TestCommit, rather than
// surfacing git's "pathspec ... did not match any files" as a hard error.
func TestCommit_PathspecAlreadyRemoved(t *testing.T) {
	t.Parallel()
	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	// Remove the pathspec's whole directory and commit that removal, so the
	// second Commit call below sees a pathspec matching nothing at all.
	lyxDir := filepath.Join(weftRepo, "_lyx")
	if err := os.RemoveAll(lyxDir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	committed, err := Commit(weftRepo, []string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("first Commit (removal): %v", err)
	}
	if !committed {
		t.Fatalf("first Commit (removal) should have succeeded")
	}

	// Second call: the pathspec no longer matches anything on disk or in the
	// index. This must be a clean no-op, not an error.
	committed, err = Commit(weftRepo, []string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("second Commit (already-removed pathspec): %v", err)
	}
	if committed {
		t.Errorf("second Commit (already-removed pathspec) = true; want false (nothing to stage)")
	}
}

func TestPush(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts SyncOptions
	}{
		{"Success", SyncOptions{}},
		{"SkipGit", SyncOptions{SkipGit: true}},
		{"SkipPush", SyncOptions{SkipPush: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath

			// Modify and commit a change
			lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
			if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			committed, err := Commit(weftRepo, []string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if !committed {
				t.Fatalf("Commit should have succeeded")
			}

			// Push
			err = Push(weftRepo, tt.opts)
			if err != nil {
				t.Fatalf("Push: %v", err)
			}

			// Verify that nothing failed
		})
	}
}

// TestPush_BrokenRemoteFailsWithoutStderrLeak asserts that when a push fails for a
// reason the rebase-retry loop cannot address (here, a remote URL that does not
// point at any git repository), the returned error is composed from local context
// (the weft path and git's exit code) rather than git's own stderr text.
func TestPush_BrokenRemoteFailsWithoutStderrLeak(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	// Modify and commit so there is something unpushed.
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	committed, err := Commit(weftRepo, []string{"_lyx"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should have succeeded")
	}

	// Point origin at a path that is not a git repository at all. The local
	// remote-tracking ref (used by hasUnpushed's @{u} lookup) is unaffected, but
	// the push itself fails for a reason that does not match any of the
	// retry-triggering substrings ("non-fast-forward", "rejected", "fetch first"),
	// so it survives the rebase-retry loop and reaches the final error path.
	badRemote := filepath.Join(t.TempDir(), "does-not-exist")
	lyxtest.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", badRemote)

	err = Push(weftRepo, SyncOptions{})
	if err == nil {
		t.Fatalf("Push() error = nil; want error (broken remote)")
	}
	wantSubstr := fmt.Sprintf("%q", weftRepo)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("Push() error = %q; want substring %q (weft path)", err.Error(), wantSubstr)
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Push() error = %q; want no %q substring (raw git stderr leak)", err.Error(), "fatal:")
	}
}

func TestPull_FastForward(t *testing.T) {
	t.Parallel()
	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	// Push an initial commit
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	committed, err := Commit(weftRepo, []string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should have succeeded")
	}
	err = Push(weftRepo, SyncOptions{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Simulate a remote-ahead state by resetting locally
	lyxtest.MustRun(t, weftRepo, "git", "reset", "--hard", "HEAD~1")

	// Now pull should fast-forward
	err = Pull(weftRepo, SyncOptions{})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Verify we're back at the pushed commit
	content, err := os.ReadFile(lyxFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "v1" {
		t.Errorf("after pull, config.yaml = %q; want %q", string(content), "v1")
	}
}
