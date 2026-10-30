//go:build tier0

// dobbynotice_test.go pins what the run notices read of a child over a task worktree on disk: the mapping of its status file and respawn memory record into dobby's reading, and the held-lock reading of a child's driver.

package battencli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// noticeTaskWorktree creates a directory the repository walk accepts as the root of the task worktree slug under a new hub, and returns the hub's prime location.
func noticeTaskWorktree(t *testing.T, slug string) *lymcwd.Location {
	t.Helper()
	hub := t.TempDir()
	worktree := filepath.Join(hub, slug)
	for _, dir := range []string{filepath.Join(worktree, ".git", "objects"), filepath.Join(worktree, ".git", "refs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return locationkit.Location(hub, "prime", ".")
}

func TestReadNoticeChild(t *testing.T) {
	t.Parallel()
	const slug = "run-a"
	statusWritten := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newestHistory := time.Date(2026, 1, 1, 11, 30, 0, 0, time.UTC)

	// writeMemory writes the respawn memory record of the Run-Shed row with its status time at respawnStatusTime.
	writeMemory := func(respawnStatusTime time.Time) func(t *testing.T, scratchDir string) {
		return func(t *testing.T, scratchDir string) {
			t.Helper()
			record := `{"respawns":3,"respawn_status_time":"` + respawnStatusTime.Format(time.RFC3339) + `"}`
			path := filepath.Join(scratchDir, battenrecipe.NameRunShed+"-memory.json")
			if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	tests := []struct {
		name string
		// memory writes the memory record into the scratch directory, none when nil.
		memory     func(t *testing.T, scratchDir string)
		wantCapped bool
	}{
		{name: "ACappedChildCarriesTheRespawnCap", memory: writeMemory(statusWritten), wantCapped: true},
		{name: "AStatusWriteSinceTheLastRespawnIsNotCapped", memory: writeMemory(statusWritten.Add(-time.Minute))},
		{name: "AMissingMemoryRecordIsNotCapped"},
		{
			name: "AnUnreadableMemoryRecordIsNotCappedAndStillReadsTheChild",
			memory: func(t *testing.T, scratchDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(scratchDir, battenrecipe.NameRunShed+"-memory.json"), []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := noticeTaskWorktree(t, slug)
			locateTask := taskWorktreeLocator(prime, slug)
			taskLocation, err := locateTask()
			if err != nil {
				t.Fatalf("locate the task worktree: %v", err)
			}
			statusPath, statusLockPath := shedrun.StatusFile(taskLocation, shedrun.SelfRunID), shedrun.StatusLock(taskLocation, shedrun.SelfRunID)
			for _, dir := range []string{filepath.Dir(statusPath), filepath.Dir(statusLockPath)} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			status := shedengine.Status{
				State:           shedengine.StateRunning,
				CurrentProducer: "Plan-Write",
				Error:           "waits on the plan",
				History: []shedengine.HistoryEntry{
					{Producer: "Worktree-Create", At: newestHistory.Add(-time.Hour).Format(time.RFC3339)},
					{Producer: "Seed-Child", At: newestHistory.Format(time.RFC3339)},
				},
			}
			if err := state.WriteJSON(statusPath, statusLockPath, status); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(statusPath, statusWritten, statusWritten); err != nil {
				t.Fatal(err)
			}
			if tt.memory != nil {
				scratchDir := shedrun.ScratchDir(prime, slug)
				if err := os.MkdirAll(scratchDir, 0o755); err != nil {
					t.Fatal(err)
				}
				tt.memory(t, scratchDir)
			}

			child, found, err := readNoticeChild(prime, slug, locateTask)
			if err != nil || !found {
				t.Fatalf("readNoticeChild() found=%v, error=%v; want the child found", found, err)
			}
			if child.State != "running" || child.CurrentProducer != "Plan-Write" || child.Error != "waits on the plan" || child.HistoryLen != 2 {
				t.Errorf("readNoticeChild() = %+v; want the status file's state, producer, error and history length", child)
			}
			if !child.NewestHistory.Equal(newestHistory) || !child.Modified.Equal(statusWritten) {
				t.Errorf("readNoticeChild() newest history %v, modified %v; want %v and %v", child.NewestHistory, child.Modified, newestHistory, statusWritten)
			}
			if child.PairDir != taskLocation.AnchorPath() {
				t.Errorf("readNoticeChild() PairDir = %q; want %q", child.PairDir, taskLocation.AnchorPath())
			}
			wantCap := 0
			if tt.wantCapped {
				wantCap = battenshed.RespawnCap
			}
			if child.RespawnCap != wantCap {
				t.Errorf("readNoticeChild() RespawnCap = %d; want %d", child.RespawnCap, wantCap)
			}
		})
	}
}

func TestChildDriverLockHeld(t *testing.T) {
	t.Parallel()
	const slug = "run-a"

	tests := []struct {
		name string
		// arrange holds or breaks the child's locks under taskLocation.
		arrange func(t *testing.T, taskLocation *lymcwd.Location)
		want    bool
	}{
		{name: "NoLockHeldReadsNotAlive", arrange: func(*testing.T, *lymcwd.Location) {}},
		{
			name: "AHeldDriverLockReadsAlive",
			arrange: func(t *testing.T, taskLocation *lymcwd.Location) {
				t.Helper()
				holdLock(t, shedrun.DriverLock(taskLocation, shedrun.SelfRunID))
			},
			want: true,
		},
		{
			name: "AHeldRunLockReadsAlive",
			arrange: func(t *testing.T, taskLocation *lymcwd.Location) {
				t.Helper()
				holdLock(t, shedrun.RunLock(taskLocation, shedrun.SelfRunID))
			},
			want: true,
		},
		{
			name: "AnUnreadableLockReadsAlive",
			arrange: func(t *testing.T, taskLocation *lymcwd.Location) {
				t.Helper()
				// A file where the scratch directory belongs makes the lock's directory impossible to create.
				scratchDir := shedrun.ScratchDir(taskLocation, shedrun.SelfRunID)
				if err := os.MkdirAll(filepath.Dir(scratchDir), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(scratchDir, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			taskLocation := locationkit.Location(t.TempDir(), slug, ".")
			tt.arrange(t, taskLocation)
			if got := childDriverLockHeld(slug, taskLocation); got != tt.want {
				t.Errorf("childDriverLockHeld() = %v; want %v", got, tt.want)
			}
		})
	}
}

// holdLock takes the write lock at lockPath until the test ends.
func holdLock(t *testing.T, lockPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, free, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil || !free {
		t.Fatalf("TryAcquireWriteLock(%q) free=%v, error=%v; want the lock taken", lockPath, free, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}
