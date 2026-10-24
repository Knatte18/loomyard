//go:build tier1

// originseams_integration_test.go pins HubSeamOptions' reads of the pause record and the parent lock store on a real git repository, which the untagged tests cannot create.

package pairteardown

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
)

// TestHubSeamOptions pins the options a call site holding only the prime builds: the pause record read at each call, the prime branch's parent lock read from the store, and the beam an unspawned member of a beam project reads as its parent.
func TestHubSeamOptions(t *testing.T) {
	hub := t.TempDir()
	primeRoot := filepath.Join(hub, "prime")
	if err := os.MkdirAll(primeRoot, 0o755); err != nil {
		t.Fatalf("create the prime root: %v", err)
	}
	gitkit.MustRun(t, primeRoot, "git", "init")
	gitkit.CommitFile(t, primeRoot, "README.md", "prime\n", "first commit")
	prime, err := lyxcwd.ResolveWorktree(primeRoot)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	options := HubSeamOptions(prime)

	if paused, err := options.HubPaused("task"); err != nil || paused {
		t.Errorf("HubPaused(task) with no pause entry = %v, %v; want not paused", paused, err)
	}
	if free, err := options.PrimeParentLockFree(); err != nil || !free {
		t.Errorf("PrimeParentLockFree() with no lock record = %v, %v; want free", free, err)
	}
	if options.WakeHeld != nil || options.LiftWakeHold != nil {
		t.Errorf("WakeHeld and LiftWakeHold = set; want both left nil for the call site that knows a wake hold")
	}

	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: now, Caller: "EXTERNAL"})
		return nil
	}); err != nil {
		t.Fatalf("write the pause entry: %v", err)
	}
	if paused, err := options.HubPaused("task"); err != nil || !paused {
		t.Errorf("HubPaused(task) under a live hub entry = %v, %v; want paused", paused, err)
	}

	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		for _, entry := range record.Live {
			entry.ResumedAt = now.Add(time.Minute)
			record.Resumed = append(record.Resumed, entry)
		}
		record.Live = nil
		return nil
	}); err != nil {
		t.Fatalf("lift the pause entry: %v", err)
	}
	if paused, err := options.HubPaused("task"); err != nil || paused {
		t.Errorf("HubPaused(task) after the entry was lifted = %v, %v; want not paused, read from the record at the call", paused, err)
	}

	branch := gitkit.CurrentBranch(t, primeRoot)
	startTime, _ := proc.StartTime(os.Getpid())
	store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(prime.HubPath))}
	attempt, err := store.TryAcquire(branch, parentlock.Owner{Slug: "task", Pair: "task", RunID: "run", PID: os.Getpid(), StartTime: startTime})
	if err != nil || !attempt.Granted {
		t.Fatalf("TryAcquire(%s) = %+v, %v; want the lock granted", branch, attempt, err)
	}
	if free, err := options.PrimeParentLockFree(); err != nil || free {
		t.Errorf("PrimeParentLockFree() with a live owner's record = %v, %v; want held", free, err)
	}

	boardDir := fabricengine.BoardDir(prime.HubPath)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("create the board dir: %v", err)
	}
	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"beam","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":1,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"beam"}]}`
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	if parent, err := options.ParentOf("member"); err != nil || parent != "beam" {
		t.Errorf("ParentOf(member) for an unspawned member of a beam project = %q, %v; want the beam", parent, err)
	}
}
