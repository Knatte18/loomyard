// dobbynotice_test.go pins what the run notices read of a child over a task worktree on disk: the mapping of its status file and respawn memory record into dobby's reading, the held-lock reading of a child's driver, and the end time of its newest ended shuttle run.

package battencli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// TestDobbyRunNotices_NewestAgentEndReadsOnlyTheTaskWorktreesEndedRuns asserts the end-time seam returns the latest end of the ended runs under the slug's task worktree.
// It ignores a run still running there and an ended run of a sibling worktree, and returns the zero time when the task worktree has none.
func TestDobbyRunNotices_NewestAgentEndReadsOnlyTheTaskWorktreesEndedRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const slug = "run-a"
	prime := noticeTaskWorktree(t, slug)
	notices := DobbyRunNotices(prime, filepath.Join(prime.HubPath, "pause.json"), time.Minute)

	if ended, err := notices.NewestAgentEnd(context.Background(), slug); err != nil || !ended.IsZero() {
		t.Fatalf("NewestAgentEnd() = %v, %v with no run record; want the zero time, nil", ended, err)
	}

	writeRun := func(runsRoot, name, outcome string, modified time.Time) {
		t.Helper()
		dir := filepath.Join(runsRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		record := fmt.Sprintf(`{"runId":%q,"strandName":"hub:task:impl","outcome":%q,"pid":1,"createdAt":"2026-01-01T09:00:00Z"}`, name, outcome)
		recordPath := filepath.Join(dir, "run.json")
		if err := os.WriteFile(recordPath, []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(recordPath, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	taskLocation, err := taskWorktreeLocator(prime, slug)()
	if err != nil {
		t.Fatalf("locate the task worktree: %v", err)
	}
	taskRuns := filepath.Join(taskLocation.AnchorPath(), ".lym", "shuttle")
	siblingRuns := filepath.Join(prime.HubPath, "sibling", ".lym", "shuttle")
	taskEnded := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	writeRun(taskRuns, "older", "stopped", taskEnded.Add(-time.Hour))
	writeRun(taskRuns, "newest", "stopped", taskEnded)
	writeRun(taskRuns, "live", "running", taskEnded.Add(time.Hour))
	writeRun(siblingRuns, "elsewhere", "stopped", taskEnded.Add(2*time.Hour))

	if ended, err := notices.NewestAgentEnd(context.Background(), slug); err != nil || !ended.Equal(taskEnded) {
		t.Errorf("NewestAgentEnd() = %v, %v; want %v, nil, the newest ended run of the task worktree alone", ended, err, taskEnded)
	}
}

func TestReadNoticeChild(t *testing.T) {
	tiergate.Require(t, 0)
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
			if child.PairDir != taskLocation.WorktreePath() {
				t.Errorf("readNoticeChild() PairDir = %q; want the task worktree root %q", child.PairDir, taskLocation.WorktreePath())
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
	tiergate.Require(t, 0)
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
			name: "AHeldLoomBootstrapLockReadsAlive",
			arrange: func(t *testing.T, taskLocation *lymcwd.Location) {
				t.Helper()
				holdLock(t, loomengine.LoomBootstrapLock(taskLocation))
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
