//go:build integration

// sync_test.go — tests for weft git operations (commit, push, pull).

package weftengine

import (
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
			committed, err := Commit(weftRepo, []string{"_lyx"}, tt.opts)
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

	committed, err := Commit(weftRepo, ScopedPathspec(".", []string{"_lyx"}), SyncOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Errorf("Commit() with ScopedPathspec = false; want true")
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

			committed, err := Commit(weftRepo, []string{"_lyx"}, SyncOptions{})
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

func TestPull_FastForward(t *testing.T) {
	t.Parallel()
	fixture := lyxtest.CopyWeft(t)
	weftRepo := fixture.WeftPath

	// Push an initial commit
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	committed, err := Commit(weftRepo, []string{"_lyx"}, SyncOptions{})
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
