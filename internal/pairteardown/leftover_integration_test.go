//go:build !windows

// leftover_integration_test.go drives RemoveOrphanPair over hub pairs.
// Its rows pin the archive-then-remove order and the re-check that runs first.

package pairteardown_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requirePairStanding fails the test unless the code worktree and the records sibling of slug are both on disk.
func requirePairStanding(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for _, path := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("stat %s error = %v; want the pair standing", path, err)
		}
	}
}

// requirePairGone fails the test unless neither the code worktree nor the records sibling of slug is on disk.
func requirePairGone(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for _, path := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stat %s error = %v; want the pair gone", path, err)
		}
	}
}

// ownerArchiveTags returns the owner archive tags of slug on the bare repository bare.
func ownerArchiveTags(t *testing.T, bare, slug string) []string {
	t.Helper()

	return strings.Fields(gitkit.Git(t, bare, "tag", "--list", fabricengine.OwnerArchiveTagPrefix+slug+"/*"))
}

// registerLiveProcess starts a sleeping process and registers it under slug in the hub's process registry.
// The process group is killed when the test ends.
func registerLiveProcess(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	sleeper := exec.Command("sleep", "300")
	sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start the sleeping process: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-sleeper.Process.Pid, syscall.SIGKILL)
		_ = sleeper.Wait()
	})
	record := runproc.ChildRecord(sleeper.Process.Pid, runproc.RoleBatten, []string{"sleep", "300"})
	if err := runproc.Register(fabricengine.RunProcDir(h.Path), slug, record); err != nil {
		t.Fatalf("register the sleeping process: %v", err)
	}
}

// TestRemoveOrphanPair drives RemoveOrphanPair over an orphan pair.
// The pair holds a local commit origin lacks and an uncommitted file.
func TestRemoveOrphanPair(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "orphan-run"
	tests := []struct {
		name string
		// setup shapes the hub further, after the pair holds its commit and its uncommitted file.
		setup func(t *testing.T, h *hubforge.Hub)
		// wantErr is a substring of the expected error, empty when the removal succeeds.
		wantErr string
	}{
		{name: "AnOrphanPairIsArchivedAndRemoved"},
		{
			name: "ARefusedArchiveTagPushLeavesThePairStanding",
			setup: func(t *testing.T, h *hubforge.Hub) {
				hook := filepath.Join(h.CodeBare, "hooks", "pre-receive")
				if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
					t.Fatalf("create the hooks directory: %v", err)
				}
				if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
					t.Fatalf("write the pre-receive hook: %v", err)
				}
			},
			wantErr: "archive the tips",
		},
		{
			name: "ABoardEntryNamingThePairRefusesTheRemoval",
			setup: func(t *testing.T, h *hubforge.Hub) {
				holdBoardEntry(t, h, slug)
			},
			wantErr: "a board entry names it",
		},
		{
			name: "ABattenRunDirectoryNamingThePairRefusesTheRemoval",
			setup: func(t *testing.T, h *hubforge.Hub) {
				if err := os.MkdirAll(shedrun.RunDir(h.Location, slug), 0o755); err != nil {
					t.Fatalf("create the batten run directory: %v", err)
				}
			},
			wantErr: "a batten run directory names it",
		},
		{
			name: "ALiveRegisteredProcessRefusesTheRemoval",
			setup: func(t *testing.T, h *hubforge.Hub) {
				registerLiveProcess(t, h, slug)
			},
			wantErr: "a process registered under it lives",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, slug)
			code := h.PairCodeWorktree(slug)
			gitkit.CommitFile(t, code, "local.txt", "local\n", "a commit origin lacks")
			if err := os.WriteFile(filepath.Join(code, "uncommitted.txt"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatalf("write the uncommitted file: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t, h)
			}

			_, err := pairteardown.RemoveOrphanPair(context.Background(), h.Location, slug)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("RemoveOrphanPair() error = %v; want one containing %q", err, tt.wantErr)
				}
				requirePairStanding(t, h, slug)
				if tags := ownerArchiveTags(t, h.CodeBare, slug); len(tags) != 0 {
					t.Errorf("owner archive tags on the code origin = %q; want none", tags)
				}
				return
			}
			if err != nil {
				t.Fatalf("RemoveOrphanPair() error = %v; want nil", err)
			}
			requirePairGone(t, h, slug)
			tags := ownerArchiveTags(t, h.CodeBare, slug)
			if len(tags) != 1 {
				t.Fatalf("owner archive tags on the code origin = %q; want exactly one", tags)
			}
			for file, want := range map[string]string{"uncommitted.txt": "uncommitted", "local.txt": "local"} {
				if got := gitkit.Git(t, h.CodeBare, "show", tags[0]+":"+file); got != want {
					t.Errorf("%s at the archive tag = %q; want %q", file, got, want)
				}
			}
		})
	}
}
