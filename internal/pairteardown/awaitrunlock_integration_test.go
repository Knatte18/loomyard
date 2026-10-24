//go:build tier1

// awaitrunlock_integration_test.go pins awaitRunLockOrPass's holder, waiting-marker and parent-lock reads over a real task worktree and a hub's pause and parent lock files.

package pairteardown

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestAwaitRunLockOrPass holds a pair's run lock and reads whether the pre-end wait passes its holder, the unfinished step's pid, or refuses with the resume form the pause readings pick.
func TestAwaitRunLockOrPass(t *testing.T) {
	const retry = "the removal"
	tests := []struct {
		name string
		// holdLock holds the run lock for the whole row.
		holdLock bool
		// paused and hubEntry are the readings passed in.
		paused, hubEntry bool
		// parentLock records a live parent lock owned by the slug; waiting writes a waiting marker carrying the holder's pid.
		parentLock, waiting bool
		wantPass            bool
		wantContains        []string
		wantNotContains     []string
	}{
		{
			name: "HubEntryUnderAParentLockRefusesWithTheNoSlugResume", holdLock: true, paused: true, hubEntry: true, parentLock: true,
			wantContains:    []string{"lyx tender resume` with no slug", "lifts every live entry of the hub"},
			wantNotContains: []string{"lyx tender resume slug"},
		},
		{
			name: "SlugEntryUnderAParentLockRefusesWithTheSlugResume", holdLock: true, paused: true, parentLock: true,
			wantContains: []string{"lyx tender resume slug"},
		},
		{name: "WaitingMarkerOfTheHoldersPidPassesUnderAParentLock", holdLock: true, parentLock: true, waiting: true, wantPass: true},
		{name: "PausedWithNoParentLockPasses", holdLock: true, paused: true, wantPass: true},
		{name: "FreeLockPassesAtOnce", wantPass: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			if err := os.MkdirAll(filepath.Join(hub, ".git"), 0o755); err != nil {
				t.Fatalf("mark the hub's prime: %v", err)
			}
			taskRoot := filepath.Join(hub, "slug")
			if err := os.MkdirAll(taskRoot, 0o755); err != nil {
				t.Fatalf("create the task root: %v", err)
			}
			gitkit.MustRun(t, taskRoot, "git", "init")
			task, err := lyxcwd.ResolveWorktree(taskRoot)
			if err != nil {
				t.Fatalf("ResolveWorktree: %v", err)
			}
			stepsDir := shedrun.StepsDir(task, shedrun.SelfRunID)
			if err := os.MkdirAll(stepsDir, 0o755); err != nil {
				t.Fatalf("create the steps directory: %v", err)
			}
			pid := os.Getpid()
			writeInflight(t, stepsDir, "step", pid, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), false)
			if tt.holdLock {
				held := mustHoldLock(t, shedrun.RunLock(task, shedrun.SelfRunID))
				defer func() { _ = held.Release() }()
			}

			prime := &lyxcwd.Location{HubPath: hub, AnchorRel: "."}
			startTime, _ := proc.StartTime(pid)
			if tt.parentLock {
				store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(hub))}
				owner := parentlock.Owner{Slug: "slug", RunID: "run", PID: pid, StartTime: startTime}
				if attempt, err := store.TryAcquire("main", owner); err != nil || !attempt.Granted {
					t.Fatalf("TryAcquire = %+v, %v; want the lock", attempt, err)
				}
			}
			if tt.waiting {
				geometry, err := hubgeom.PauseGeometry(prime)
				if err != nil {
					t.Fatalf("PauseGeometry: %v", err)
				}
				dir := pauserecord.WaitingDir(geometry.RecordPath)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create the waiting directory: %v", err)
				}
				data, err := json.Marshal(pauserecord.WaitingMarker{PID: pid, StartTime: startTime, Slug: "slug"})
				if err != nil {
					t.Fatalf("encode the waiting marker: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "marker.json"), data, 0o644); err != nil {
					t.Fatalf("write the waiting marker: %v", err)
				}
			}

			sleeps := 0
			td := &Teardown{
				prime:     prime,
				goos:      "linux",
				sleep:     func(context.Context, time.Duration) error { sleeps++; return nil },
				driverPID: func(string) (int, bool) { return 0, false },
			}
			err = td.awaitRunLockOrPass(context.Background(), "slug", tt.paused, tt.hubEntry, retry)

			if tt.wantPass {
				if err != nil {
					t.Fatalf("awaitRunLockOrPass() = %v; want nil", err)
				}
				if !tt.holdLock && sleeps != 0 {
					t.Errorf("sleeps = %d; want none for a lock free at once", sleeps)
				}
				return
			}
			if !errors.Is(err, ErrDriverBusy) {
				t.Fatalf("awaitRunLockOrPass() = %v; want one wrapping ErrDriverBusy", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q lacks %q", err.Error(), want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("refusal %q names %q", err.Error(), notWant)
				}
			}
		})
	}
}
