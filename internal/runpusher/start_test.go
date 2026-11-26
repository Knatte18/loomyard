// start_test.go drives the pusher's start conditions over a starter built from fake reads and a fake spawn.

package runpusher

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// startFake holds what the fake reads answer and records what the starter read and spawned.
type startFake struct {
	skipped  bool
	lockFree bool
	stands   bool
	paused   bool
	marked   bool
	ahead    bool
	// failing names the read that fails: lock, stands, paused, marked, ahead or spawn.
	failing string
	reads   []string
	spawned []spawnRequest
}

var errFakeRead = errors.New("fake read failed")

// starter builds the starter over the fake.
func (f *startFake) starter() starter {
	read := func(name string, answer bool) (bool, error) {
		f.reads = append(f.reads, name)
		if f.failing == name {
			return false, errFakeRead
		}
		return answer, nil
	}
	return starter{
		skipped:           func() bool { return f.skipped },
		lockFree:          func(string) (bool, error) { return read("lock", f.lockFree) },
		refusedPushStands: func(Target) (bool, error) { return read("stands", f.stands) },
		pauseCovers:       func(Target) (bool, error) { return read("paused", f.paused) },
		handedOver:        func(Target) (bool, error) { return read("marked", f.marked) },
		ahead:             func(Target) (bool, error) { return read("ahead", f.ahead) },
		spawn: func(req spawnRequest) error {
			if f.failing == "spawn" {
				return errFakeRead
			}
			f.spawned = append(f.spawned, req)
			return nil
		},
	}
}

// TestStart covers when a pusher is spawned and what the spawn carries.
func TestStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := filepath.Join(string(filepath.Separator), "hub")
	pair := Target{Location: &lymcwd.Location{HubPath: hub, WorktreeName: "pair-slug"}, Slug: "pair-slug", Pair: true}
	prime := Target{Location: &lymcwd.Location{HubPath: hub, WorktreeName: "prime"}}
	runProcDir := filepath.Join(hub, lymcwd.BoltDirName, ".lym", "runproc")
	pairRequest := spawnRequest{
		Args:       []string{"loom", "pusher", "--worktree", filepath.Join(hub, "pair-slug"), "--slug", "pair-slug"},
		Dir:        filepath.Join(hub, lymcwd.BoltDirName),
		RunProcDir: runProcDir,
		Owner:      lymcwd.BoltDirName,
		Role:       "push",
	}
	primeRequest := spawnRequest{
		Args:       []string{"loom", "pusher", "--worktree", filepath.Join(hub, "prime")},
		Dir:        filepath.Join(hub, lymcwd.BoltDirName),
		RunProcDir: runProcDir,
		Owner:      lymcwd.BoltDirName,
		Role:       "push",
	}

	tests := []struct {
		name        string
		target      Target
		ifAhead     bool
		setup       func(f *startFake)
		wantRequest *spawnRequest
		wantErr     bool
		// wantNoReads asserts the starter read nothing.
		wantNoReads bool
	}{
		{name: "EveryConditionHoldsForAPair", target: pair, wantRequest: &pairRequest},
		{name: "APrimeSpawnsWithoutASlug", target: prime, wantRequest: &primeRequest},
		{name: "LockHeld", target: pair, setup: func(f *startFake) { f.lockFree = false }},
		{name: "RefusedPushStands", target: pair, setup: func(f *startFake) { f.stands = true }},
		{name: "PauseCoversAPair", target: pair, setup: func(f *startFake) { f.paused = true }},
		{name: "HandOverMarkNamesAPair", target: pair, setup: func(f *startFake) { f.marked = true }},
		{
			name: "PauseAndMarkDoNotApplyToThePrime", target: prime, wantRequest: &primeRequest,
			setup: func(f *startFake) { f.paused, f.marked = true, true },
		},
		{name: "GitOrPushesOffReadsNothing", target: pair, setup: func(f *startFake) { f.skipped = true }, wantNoReads: true},
		{name: "IfAheadWithTheWorktreeLevel", target: pair, ifAhead: true, setup: func(f *startFake) { f.ahead = false }},
		{name: "IfAheadWithTheWorktreeAhead", target: pair, ifAhead: true, wantRequest: &pairRequest},
		{name: "LevelStartsWithoutTheAheadRead", target: pair, setup: func(f *startFake) { f.ahead = false }, wantRequest: &pairRequest},
		{name: "LockReadFails", target: pair, setup: func(f *startFake) { f.failing = "lock" }, wantErr: true},
		{name: "RefusedPushReadFails", target: pair, setup: func(f *startFake) { f.failing = "stands" }, wantErr: true},
		{name: "PauseReadFails", target: pair, setup: func(f *startFake) { f.failing = "paused" }, wantErr: true},
		{name: "HandOverMarkReadFails", target: pair, setup: func(f *startFake) { f.failing = "marked" }, wantErr: true},
		{name: "AheadReadFails", target: pair, ifAhead: true, setup: func(f *startFake) { f.failing = "ahead" }, wantErr: true},
		{name: "SpawnFails", target: pair, setup: func(f *startFake) { f.failing = "spawn" }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &startFake{lockFree: true, ahead: true}
			if tt.setup != nil {
				tt.setup(f)
			}

			spawned, err := f.starter().start(tt.target, tt.ifAhead)

			if (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, errFakeRead)) {
				t.Fatalf("start() error = %v; want error %v", err, tt.wantErr)
			}
			if wantSpawn := tt.wantRequest != nil; spawned != wantSpawn {
				t.Errorf("start() = %v; want %v", spawned, wantSpawn)
			}
			if tt.wantRequest == nil && len(f.spawned) != 0 {
				t.Errorf("spawns = %+v; want none", f.spawned)
			}
			if tt.wantRequest != nil && !reflect.DeepEqual(f.spawned, []spawnRequest{*tt.wantRequest}) {
				t.Errorf("spawns = %+v; want one %+v", f.spawned, *tt.wantRequest)
			}
			if tt.wantNoReads && len(f.reads) != 0 {
				t.Errorf("reads = %v; want none", f.reads)
			}
		})
	}
}
