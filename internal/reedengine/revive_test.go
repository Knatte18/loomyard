// revive_test.go pins the revival trigger inside the boot and the Revive predicate against the fake tmux.

package reedengine

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// reviveTestEngine builds an engine whose boot reaches the point where it would create its session:
// the multiplexer probe passes and the session is not live.
func reviveTestEngine(t *testing.T) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	e.cfg.DebugLog = "0"
	e.cfg.Mouse = "off"
	// The boot refuses a shell that does not resolve before any tmux round trip.
	if runtime.GOOS != "windows" {
		e.cfg.Shell = "sh"
	}
	fake := installFakeTmux(t, e)
	fake.answer("-V", fakeVersionOutput, nil)
	fake.answer("list-commands", fakeFullCommandsOutput(), nil)
	fake.answer("has-session", "", exitCodeErr{code: 1})
	return e, fake
}

func recordSession(t *testing.T, e *Engine) {
	t.Helper()
	if err := SaveState(e.stateDir(), &ReedState{Session: e.SessionName()}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
}

func toldSpawnOrder() func() ([]ReviveEntry, error) {
	return func() ([]ReviveEntry, error) { return nil, nil }
}

// TestEnsureServerAndSession_RevivalTrigger pins when the boot returns the revival sentinel instead of creating the session:
// only for a recorded session that is not live, with a spawn order told and the skip unset.
// A first boot, a nil spawn order and the skip each go on to create the session, which the nonexistent multiplexer binary refuses as "start tmux".
func TestEnsureServerAndSession_RevivalTrigger(t *testing.T) {
	tests := []struct {
		name      string
		recorded  bool
		spawn     func() ([]ReviveEntry, error)
		skip      bool
		liveSess  bool
		wantFirst bool
		wantSpawn bool
	}{
		{name: "RecordedNotLiveRevivesFirst", recorded: true, spawn: toldSpawnOrder(), wantFirst: true},
		{name: "FirstBootCreates", spawn: toldSpawnOrder(), wantSpawn: true},
		{name: "NilSpawnOrderCreates", recorded: true, wantSpawn: true},
		{name: "SkipSetCreates", recorded: true, spawn: toldSpawnOrder(), skip: true, wantSpawn: true},
		{name: "LiveSessionIsUsedAsFound", recorded: true, spawn: toldSpawnOrder(), liveSess: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := reviveTestEngine(t)
			e.geom.SpawnOrder = tt.spawn
			e.skipRevival = tt.skip
			if tt.recorded {
				recordSession(t, e)
			}
			if tt.liveSess {
				fake.answer("has-session", "", nil)
				fake.answer("list-panes", "%1 0 0 10 5 4321\n", nil)
			}

			booted, _, err := e.ensureServerAndSessionLocked()

			switch {
			case tt.wantFirst:
				if !errors.Is(err, errReviveFirst) {
					t.Errorf("ensureServerAndSessionLocked() error = %v, want the revival sentinel", err)
				}
			case tt.wantSpawn:
				if err == nil || !strings.Contains(err.Error(), "start tmux") {
					t.Errorf("ensureServerAndSessionLocked() error = %v, want it to reach the session spawn", err)
				}
			default:
				if err != nil || booted {
					t.Errorf("ensureServerAndSessionLocked() = (booted %v, %v), want the live session used as found", booted, err)
				}
			}
		})
	}
}

// TestRevive_CreatesNothingWithoutThePredicate pins that Revive reports false and spawns nothing for a worktree that records no session or whose recorded session is live.
func TestRevive_CreatesNothingWithoutThePredicate(t *testing.T) {
	tests := []struct {
		name      string
		recorded  bool
		liveSess  bool
		wantCalls int
	}{
		{name: "NoRecordedSession", wantCalls: 0},
		{name: "SessionLive", recorded: true, liveSess: true, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := reviveTestEngine(t)
			if tt.recorded {
				recordSession(t, e)
			}
			if tt.liveSess {
				fake.answer("has-session", "", nil)
			}

			revived, err := e.Revive()

			if revived || err != nil {
				t.Errorf("Revive() = (%v, %v), want (false, nil)", revived, err)
			}
			if calls := fake.Calls(); len(calls) != tt.wantCalls {
				t.Errorf("Revive() issued tmux calls %v, want %d (only the liveness check)", calls, tt.wantCalls)
			}
		})
	}
}
