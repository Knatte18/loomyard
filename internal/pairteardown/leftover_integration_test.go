//go:build !windows

// leftover_integration_test.go drives RemoveOrphanPair and RemoveBenchPairs over hub pairs.
// Its rows pin the archive-then-remove order and the checks that run first.

package pairteardown_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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

	return strings.Fields(gitkit.Git(t, bare, "tag", "--list", fabriclayout.OwnerArchiveTagPrefix+slug+"/*"))
}

// registerLiveProcess starts a sleeping process and registers it under slug in the hub's process registry.
// The process inherits the open files in inherited.
// The process group is killed when the test ends.
func registerLiveProcess(t *testing.T, h *hubforge.Hub, slug string, inherited ...*os.File) {
	t.Helper()

	sleeper := exec.Command("sleep", "300")
	sleeper.ExtraFiles = inherited
	sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start the sleeping process: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-sleeper.Process.Pid, syscall.SIGKILL)
		_ = sleeper.Wait()
	})
	record := runproc.ChildRecord(sleeper.Process.Pid, runproc.RoleBatten, []string{"sleep", "300"})
	if err := runproc.Register(fabriclayout.RunProcDir(h.Path), slug, record); err != nil {
		t.Fatalf("register the sleeping process: %v", err)
	}
}

// holdRunLockFromLiveProcess takes the run lock of the pair slug.
// It leaves the lock held by a registered sleeping process alone.
func holdRunLockFromLiveProcess(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	pair, err := lymcwd.NamedWorktree(h.Path, slug)
	if err != nil {
		t.Fatalf("locate the pair %s: %v", slug, err)
	}
	lockPath := shedrun.RunLock(pair, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create the run lock's directory: %v", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open the run lock: %v", err)
	}
	// The flock belongs to the open file, so it stays with the child that inherits it once this handle is closed.
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("take the run lock: %v", err)
	}
	registerLiveProcess(t, h, slug, file)
}

// refuseCodePushesNaming installs a pre-receive hook on the code origin that refuses every ref whose name holds part.
func refuseCodePushesNaming(t *testing.T, h *hubforge.Hub, part string) {
	t.Helper()

	hook := filepath.Join(h.CodeBare, "hooks", "pre-receive")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatalf("create the hooks directory: %v", err)
	}
	script := "#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in *" + part + "*) exit 1 ;; esac\ndone\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write the pre-receive hook: %v", err)
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

// TestRemoveBenchPairs drives RemoveBenchPairs for the task "task".
// The hub holds two of its bench pairs and a pair whose slug only shares the task's prefix.
// The first bench pair holds a local commit origin lacks and an uncommitted file.
func TestRemoveBenchPairs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		task         = "task"
		bench        = "task-b1"
		secondBench  = "task-b2"
		sharedPrefix = "task-notes"
	)
	tests := []struct {
		name string
		// setup shapes the hub further, after the first bench pair holds its commit and its uncommitted file.
		setup func(t *testing.T, h *hubforge.Hub)
		// wantRemoved lists the slugs the removal reports, on a failure those removed before the failing pair.
		wantRemoved []string
		// wantErr is a substring of the expected error, empty when the removal succeeds.
		wantErr string
	}{
		{name: "EveryBenchPairIsArchivedAndRemoved", wantRemoved: []string{bench, secondBench}},
		{
			name: "ALiveProcessOfABenchPairIsEndedNotRefused",
			setup: func(t *testing.T, h *hubforge.Hub) {
				registerLiveProcess(t, h, bench)
			},
			wantRemoved: []string{bench, secondBench},
		},
		{
			name: "ARunLockHeldByALiveProcessOfABenchPairIsEndedNotRefused",
			setup: func(t *testing.T, h *hubforge.Hub) {
				holdRunLockFromLiveProcess(t, h, bench)
			},
			wantRemoved: []string{bench, secondBench},
		},
		{
			name: "ABoardEntryNamingABenchSlugKeepsThatPair",
			setup: func(t *testing.T, h *hubforge.Hub) {
				holdBoardEntry(t, h, bench)
			},
			wantRemoved: []string{secondBench},
		},
		{
			name: "ARefusedArchiveTagPushKeepsTheBenchPair",
			setup: func(t *testing.T, h *hubforge.Hub) {
				gitkit.CommitFile(t, h.PairCodeWorktree(secondBench), "second.txt", "second\n", "a commit origin lacks")
				refuseCodePushesNaming(t, h, secondBench)
			},
			wantRemoved: []string{bench},
			wantErr:     "archive the tips of the bench pair",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			for _, slug := range []string{bench, secondBench, sharedPrefix} {
				hubforge.AddPair(t, h, slug)
			}
			code := h.PairCodeWorktree(bench)
			gitkit.CommitFile(t, code, "local.txt", "local\n", "a commit origin lacks")
			if err := os.WriteFile(filepath.Join(code, "uncommitted.txt"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatalf("write the uncommitted file: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t, h)
			}

			removed, err := pairteardown.RemoveBenchPairs(context.Background(), h.Location, task)

			requirePairStanding(t, h, sharedPrefix)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("RemoveBenchPairs() error = %v; want one containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("RemoveBenchPairs() error = %v; want nil", err)
			}
			slices.Sort(removed)
			if !slices.Equal(removed, tt.wantRemoved) {
				t.Errorf("RemoveBenchPairs() = %q; want %q", removed, tt.wantRemoved)
			}
			for _, slug := range []string{bench, secondBench} {
				if slices.Contains(tt.wantRemoved, slug) {
					requirePairGone(t, h, slug)
				} else {
					requirePairStanding(t, h, slug)
				}
			}
			if !slices.Contains(tt.wantRemoved, bench) {
				return
			}
			tags := ownerArchiveTags(t, h.CodeBare, bench)
			if len(tags) != 1 {
				t.Fatalf("owner archive tags of %s on the code origin = %q; want exactly one", bench, tags)
			}
			if got := gitkit.Git(t, h.CodeBare, "show", tags[0]+":uncommitted.txt"); got != "uncommitted" {
				t.Errorf("uncommitted.txt at the archive tag = %q; want %q", got, "uncommitted")
			}
		})
	}
}
