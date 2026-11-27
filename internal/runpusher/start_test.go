// start_test.go drives the pusher's start conditions over a starter built from fake reads and a fake spawn.
// It also drives the ledger pruning.

package runpusher

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
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

// startTargets are the targets the rows of TestStart pick from, over one hub.
type startTargets struct {
	pair, prime, board, landing Target
}

func targetsOfHub(hub string) startTargets {
	prime := &lymcwd.Location{HubPath: hub, WorktreeName: "prime"}
	pair := &lymcwd.Location{HubPath: hub, WorktreeName: "pair-slug"}
	return startTargets{
		pair:    Target{Location: pair, Slug: "pair-slug", Pair: true},
		prime:   Target{Location: prime},
		board:   BoardTarget(prime),
		landing: LandingTarget(prime),
	}
}

// TestStart covers when a pusher is spawned, what the spawn carries and the ledger entry StartIfAhead keeps.
func TestStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pick := func(name string) func(startTargets) Target {
		return func(targets startTargets) Target {
			return map[string]Target{"pair": targets.pair, "prime": targets.prime, "board": targets.board, "landing": targets.landing}[name]
		}
	}
	// request builds the spawn the named target of hub asks for.
	request := func(hub, name string) spawnRequest {
		args := map[string][]string{
			"pair":    {"loom", "pusher", "--worktree", filepath.Join(hub, "pair-slug"), "--slug", "pair-slug"},
			"prime":   {"loom", "pusher", "--worktree", filepath.Join(hub, "prime")},
			"board":   {"loom", "pusher", "--worktree", filepath.Join(hub, "prime"), "--board"},
			"landing": {"loom", "pusher", "--worktree", filepath.Join(hub, "prime"), "--landing"},
		}[name]
		return spawnRequest{
			Args:       args,
			Dir:        filepath.Join(hub, lymcwd.BoltDirName),
			RunProcDir: filepath.Join(hub, lymcwd.BoltDirName, ".lym", "runproc"),
			Owner:      lymcwd.BoltDirName,
			Role:       "push",
		}
	}

	tests := []struct {
		name    string
		target  string
		ifAhead bool
		setup   func(f *startFake)
		// seeded is an entry written for the target before the start.
		seeded      *pushledger.Entry
		wantSpawn   bool
		wantErr     bool
		wantNoReads bool
		// wantEntry reports whether the target's ledger entry stands after the start.
		wantEntry bool
	}{
		{name: "EveryConditionHoldsForAPair", target: "pair", wantSpawn: true},
		{name: "APrimeSpawnsWithoutASlug", target: "prime", wantSpawn: true},
		{name: "TheBoardSpawnsWithItsFlag", target: "board", wantSpawn: true},
		{name: "TheLandingTargetSpawnsWithItsFlag", target: "landing", wantSpawn: true},
		{name: "LockHeld", target: "pair", setup: func(f *startFake) { f.lockFree = false }},
		{name: "RefusedPushStands", target: "pair", setup: func(f *startFake) { f.stands = true }},
		{name: "PauseCoversAPair", target: "pair", setup: func(f *startFake) { f.paused = true }},
		{name: "HandOverMarkNamesAPair", target: "pair", setup: func(f *startFake) { f.marked = true }},
		{name: "PauseAndMarkDoNotApplyToThePrime", target: "prime", wantSpawn: true, setup: func(f *startFake) { f.paused, f.marked = true, true }},
		{name: "PauseAndMarkDoNotApplyToTheBoard", target: "board", wantSpawn: true, setup: func(f *startFake) { f.paused, f.marked = true, true }},
		{name: "PauseAndMarkDoNotApplyToTheLandingTarget", target: "landing", wantSpawn: true, setup: func(f *startFake) { f.paused, f.marked = true, true }},
		{name: "GitOrPushesOffReadsNothing", target: "pair", setup: func(f *startFake) { f.skipped = true }, wantNoReads: true},
		{name: "IfAheadWithTheWorktreeLevel", target: "pair", ifAhead: true, setup: func(f *startFake) { f.ahead = false }},
		{
			name: "IfAheadWithTheWorktreeLevelDropsItsEntry", target: "pair", ifAhead: true,
			setup:  func(f *startFake) { f.ahead = false },
			seeded: &pushledger.Entry{FirstOwed: time.Unix(1, 0)},
		},
		{name: "IfAheadWithTheWorktreeAheadWritesItsEntry", target: "pair", ifAhead: true, wantSpawn: true, wantEntry: true},
		{name: "IfAheadWithARefusedRecordsTargetWritesNothing", target: "pair", ifAhead: true, setup: func(f *startFake) { f.stands = true }},
		{name: "IfAheadWithARefusedBoardTargetWritesNothing", target: "board", ifAhead: true, setup: func(f *startFake) { f.stands = true }},
		{name: "ARefusedLandingTargetSpawnsNothing", target: "landing", setup: func(f *startFake) { f.stands = true }},
		{name: "LevelStartsWithoutTheAheadRead", target: "pair", setup: func(f *startFake) { f.ahead = false }, wantSpawn: true},
		{name: "LockReadFails", target: "pair", setup: func(f *startFake) { f.failing = "lock" }, wantErr: true},
		{name: "RefusedPushReadFails", target: "pair", setup: func(f *startFake) { f.failing = "stands" }, wantErr: true},
		{name: "PauseReadFails", target: "pair", setup: func(f *startFake) { f.failing = "paused" }, wantErr: true},
		{name: "HandOverMarkReadFails", target: "pair", setup: func(f *startFake) { f.failing = "marked" }, wantErr: true},
		{name: "AheadReadFails", target: "pair", ifAhead: true, setup: func(f *startFake) { f.failing = "ahead" }, wantErr: true},
		{name: "SpawnFails", target: "pair", setup: func(f *startFake) { f.failing = "spawn" }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			target := pick(tt.target)(targetsOfHub(hub))
			f := &startFake{lockFree: true, ahead: true}
			if tt.setup != nil {
				tt.setup(f)
			}
			if tt.seeded != nil {
				seedEntry(t, hub, ledgerKey(target), *tt.seeded)
			}

			spawned, err := f.starter().start(target, tt.ifAhead)

			if (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, errFakeRead)) {
				t.Fatalf("start() error = %v; want error %v", err, tt.wantErr)
			}
			if spawned != tt.wantSpawn {
				t.Errorf("start() = %v; want %v", spawned, tt.wantSpawn)
			}
			var wantSpawned []spawnRequest
			if tt.wantSpawn {
				wantSpawned = []spawnRequest{request(hub, tt.target)}
			}
			if !reflect.DeepEqual(f.spawned, wantSpawned) {
				t.Errorf("spawns = %+v; want %+v", f.spawned, wantSpawned)
			}
			if tt.wantNoReads && len(f.reads) != 0 {
				t.Errorf("reads = %v; want none", f.reads)
			}
			entry, found, err := pushledger.Read(hub, ledgerKey(target))
			if err != nil {
				t.Fatalf("Read() = %v; want nil", err)
			}
			if found != tt.wantEntry {
				t.Fatalf("entry = %+v, found %v; want found %v", entry, found, tt.wantEntry)
			}
			if tt.wantEntry {
				wantKey := pushledger.RecordsKey(fabricengine.RecordsWorktree(target.Location))
				if ledgerKey(target) != wantKey || entry.Worktree != filepath.Join(hub, "pair-slug") || entry.Slug != "pair-slug" || entry.FirstOwed.IsZero() {
					t.Errorf("entry = %+v; want the pair's code worktree and slug, owed", entry)
				}
			}
		})
	}
}

// seedEntry writes entry as the ledger's entry of key.
func seedEntry(t *testing.T, hub string, key pushledger.Key, entry pushledger.Entry) {
	t.Helper()
	err := pushledger.Update(hub, key, time.Unix(2, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return entry, true, nil
	})
	if err != nil {
		t.Fatalf("Update(%s) = %v; want nil", key, err)
	}
}

// TestPruneLedger covers which entries the pruning drops: only that of a gone worktree holding no landing.
func TestPruneLedger(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	standing := filepath.Join(hub, "standing")
	if err := os.Mkdir(standing, 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(hub, "gone")
	entries := map[pushledger.Key]pushledger.Entry{
		"standing":            {Worktree: standing, Slug: "standing"},
		"gone":                {Worktree: gone, Slug: "gone"},
		"gone-with-a-landing": {Worktree: gone, Slug: "landed", Landings: []pushledger.Landing{{Run: "landed"}}},
		pushledger.BoardKey:   {},
		pushledger.LandingKey: {},
	}
	for key, entry := range entries {
		seedEntry(t, hub, key, entry)
	}

	if err := PruneLedger(hub, time.Unix(3, 0)); err != nil {
		t.Fatalf("PruneLedger() = %v; want nil", err)
	}

	left, err := pushledger.List(hub)
	if err != nil {
		t.Fatalf("List() = %v; want nil", err)
	}
	for key := range entries {
		_, found := left[key]
		if want := key != "gone"; found != want {
			t.Errorf("entry %s found = %v; want %v", key, found, want)
		}
	}
}
