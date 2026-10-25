//go:build tier0

// start_test.go covers EnsureLive over fake seams and Live over a real lock file in a temp directory.

package dobbyengine

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// fakeStart is an in-memory stand-in for the hub: it serves the owners it holds, records the claims and spawns, and reads a spawned worktree as live afterwards.
type fakeStart struct {
	primeOwner string
	claimErr   error
	claimLost  string
	beams      []BeamPair
	failSpawn  map[string]bool

	claims  []string
	spawned []string
	live    map[string]bool
}

func (f *fakeStart) deps() StartDeps {
	f.live = map[string]bool{}
	return StartDeps{
		Machine:    func() (string, error) { return "here", nil },
		PrimeOwner: func() (string, error) { return f.primeOwner, nil },
		ClaimPrime: func(machine string) (string, error) {
			f.claims = append(f.claims, machine)
			if f.claimErr == nil && f.claimLost == "" {
				f.primeOwner = machine
			}
			return f.claimLost, f.claimErr
		},
		PrimeName: "hub-prime",
		Beams:     func() ([]BeamPair, error) { return f.beams, nil },
		Geometry:  func(worktree string) (Geometry, error) { return Geometry{WorktreeName: worktree}, nil },
		Live:      func(geom Geometry) (bool, error) { return f.live[geom.WorktreeName], nil },
		Spawn: func(geom Geometry) error {
			if f.failSpawn[geom.WorktreeName] {
				return errors.New("exec failed")
			}
			f.spawned = append(f.spawned, geom.WorktreeName)
			f.live[geom.WorktreeName] = true
			return nil
		},
	}
}

func TestEnsureLive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fake        fakeStart
		calls       int
		wantEntries []StartEntry
		wantClaims  []string
		wantSpawned []string
		// wantErr is a text the returned error carries; empty wants none.
		wantErr string
		// wantDetails are the texts the prime's entry detail carries.
		wantDetails []string
	}{
		{
			name:        "NoPrimeOwnerRecordsThisMachineAndStartsThePrimeAndAnOwnedBeam",
			fake:        fakeStart{beams: []BeamPair{{Slug: "b1"}}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeStarted}},
			wantClaims:  []string{"here"},
			wantSpawned: []string{"hub-prime", "b1"},
		},
		{
			name:        "FailedPrimeClaimIsReportedAndStartsNoPrimeButTheBeam",
			fake:        fakeStart{claimErr: errors.New("origin unreachable"), beams: []BeamPair{{Slug: "b1", Owner: "here"}}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeFailed}, {Worktree: "b1", Outcome: OutcomeStarted}},
			wantClaims:  []string{"here"},
			wantSpawned: []string{"b1"},
			wantDetails: []string{"origin unreachable", "lyx beam start"},
		},
		{
			name:        "ASecondCallReportsEveryDobbyLiveAndSpawnsNothing",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}}},
			calls:       2,
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeLive}, {Worktree: "b1", Outcome: OutcomeLive}},
			wantSpawned: []string{"hub-prime", "b1"},
		},
		{
			name:        "APrimeAnotherMachineOwnsStartsNoPrimeAndNamesItsOwner",
			fake:        fakeStart{primeOwner: "there", beams: []BeamPair{{Slug: "b1", Owner: "here"}}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeRefused}, {Worktree: "b1", Outcome: OutcomeStarted}},
			wantSpawned: []string{"b1"},
			wantDetails: []string{"owned by there; lyx adopt --prime takes it over here"},
		},
		{
			name:        "ALostPrimeClaimReadsTheOtherMachinesClaim",
			fake:        fakeStart{claimLost: "there"},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeRefused}},
			wantClaims:  []string{"here"},
			wantDetails: []string{"owned by there"},
		},
		{
			name:        "ABeamOwnedElsewhereIsSkipped",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1", Owner: "there"}, {Slug: "b2", Owner: "here"}}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b2"},
		},
		{
			name:        "OneFailedSpawnStillStartsTheRestAndNamesItInTheError",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, failSpawn: map[string]bool{"b1": true}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeFailed}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b2"},
			wantErr:     "b1: start the dobby: exec failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := tt.fake
			deps := fake.deps()
			calls := max(tt.calls, 1)
			var report StartReport
			var err error
			for range calls {
				report, err = EnsureLive(deps)
			}

			if tt.wantErr == "" && err != nil {
				t.Fatalf("EnsureLive() error = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("EnsureLive() error = %v; want it to carry %q", err, tt.wantErr)
			}
			if len(report.Entries) != len(tt.wantEntries) {
				t.Fatalf("EnsureLive() entries = %+v; want %d entries %+v", report.Entries, len(tt.wantEntries), tt.wantEntries)
			}
			for i, want := range tt.wantEntries {
				got := report.Entries[i]
				if got.Worktree != want.Worktree || got.Outcome != want.Outcome {
					t.Errorf("EnsureLive() entry %d = %s %s; want %s %s", i, got.Worktree, got.Outcome, want.Worktree, want.Outcome)
				}
			}
			for _, want := range tt.wantDetails {
				if !strings.Contains(report.Entries[0].Detail, want) {
					t.Errorf("EnsureLive() prime detail = %q; want it to carry %q", report.Entries[0].Detail, want)
				}
			}
			if !slices.Equal(fake.claims, tt.wantClaims) {
				t.Errorf("EnsureLive() claims = %v; want %v", fake.claims, tt.wantClaims)
			}
			if !slices.Equal(fake.spawned, tt.wantSpawned) {
				t.Errorf("EnsureLive() spawned = %v; want %v", fake.spawned, tt.wantSpawned)
			}
		})
	}
}

// TestLive asserts a dobby reads live exactly while its lock is held, and a worktree with no marker directory reads not live.
func TestLive(t *testing.T) {
	t.Parallel()

	t.Run("NoMarkerDirectoryReadsNotLive", func(t *testing.T) {
		t.Parallel()

		geom := Geometry{LockPath: filepath.Join(t.TempDir(), "beam", "dobby.lock")}
		if live, err := Live(geom); live || err != nil {
			t.Errorf("Live() = (%v, %v); want (false, nil)", live, err)
		}
	})

	t.Run("AFreeLockReadsNotLiveAndStaysFree", func(t *testing.T) {
		t.Parallel()

		geom := Geometry{LockPath: filepath.Join(t.TempDir(), "dobby.lock")}
		if live, err := Live(geom); live || err != nil {
			t.Fatalf("Live() = (%v, %v); want (false, nil)", live, err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock() = (%v, %v); want the lock free after Live", acquired, err)
		}
		if err := held.Release(); err != nil {
			t.Fatalf("Release() = %v; want nil", err)
		}
	})

	t.Run("AHeldLockReadsLive", func(t *testing.T) {
		t.Parallel()

		geom := Geometry{LockPath: filepath.Join(t.TempDir(), "dobby.lock")}
		held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock() = (%v, %v); want the lock", acquired, err)
		}
		t.Cleanup(func() { _ = held.Release() })
		if live, err := Live(geom); !live || err != nil {
			t.Errorf("Live() = (%v, %v); want (true, nil)", live, err)
		}
	})
}
