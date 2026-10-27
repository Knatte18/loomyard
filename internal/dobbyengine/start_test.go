//go:build tier0

// start_test.go covers EnsureLive over fake seams and Live over a real lock file in a temp directory.

package dobbyengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// fakeStart is an in-memory stand-in for the hub: it serves the owners it holds, records the claims and spawns, and reads a spawned worktree as live afterwards.
// A worktree in moving has an anchor change standing, and one in moveErr fails the read of whether it does.
type fakeStart struct {
	primeOwner string
	claimErr   error
	claimLost  string
	beams      []BeamPair
	failSpawn  map[string]bool
	moving     map[string]bool
	moveErr    map[string]error
	// held are the worktrees whose landing hold stands, and heldErr fails the hold read of one.
	held    map[string]bool
	heldErr map[string]error
	// noHoldSeam leaves LandingHeld nil.
	noHoldSeam bool

	claims  []string
	spawned []string
	live    map[string]bool
}

func (f *fakeStart) deps() StartDeps {
	f.live = map[string]bool{}
	deps := f.baseDeps()
	if !f.noHoldSeam {
		deps.LandingHeld = func(worktree string) (bool, error) {
			if err := f.heldErr[worktree]; err != nil {
				return false, err
			}
			return f.held[worktree], nil
		}
	}
	return deps
}

// allWorktrees names the prime and every beam the fake holds, the set a caller that names everything passes.
func (f *fakeStart) allWorktrees() []string {
	worktrees := []string{"hub-prime"}
	for _, beam := range f.beams {
		worktrees = append(worktrees, beam.Slug)
	}
	return worktrees
}

func (f *fakeStart) baseDeps() StartDeps {
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
		MoveInProgress: func(worktree string) (string, bool, error) {
			if err := f.moveErr[worktree]; err != nil {
				return "", false, err
			}
			if f.moving[worktree] {
				return "run `lym fabric anchor " + worktree + " --set sub`", true, nil
			}
			return "", false, nil
		},
		Live: func(geom Geometry) (bool, error) { return f.live[geom.WorktreeName], nil },
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
		name string
		fake fakeStart
		// named are the worktrees passed to EnsureLive; nil names the prime and every beam.
		named       []string
		calls       int
		wantEntries []StartEntry
		wantClaims  []string
		wantSpawned []string
		// wantErr is a text the returned error carries; empty wants none.
		wantErr string
		// wantDetails are the texts the prime's entry detail carries.
		wantDetails []string
		// wantBeamDetail is a text the first beam's entry detail carries; empty wants none.
		wantBeamDetail string
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
			wantDetails: []string{"origin unreachable", "lym beam start"},
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
			wantDetails: []string{"owned by there; lym adopt root takes it over here"},
		},
		{
			name:        "ALostPrimeClaimReadsTheOtherMachinesClaim",
			fake:        fakeStart{claimLost: "there"},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeRefused}},
			wantClaims:  []string{"here"},
			wantDetails: []string{"owned by there"},
		},
		{
			name:           "ANamedBeamOwnedElsewhereIsRefusedNamingAdoptAndItsOwner",
			fake:           fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1", Owner: "there"}, {Slug: "b2", Owner: "here"}}},
			wantEntries:    []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeRefused}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned:    []string{"hub-prime", "b2"},
			wantBeamDetail: "owned by there; lym adopt b1",
		},
		{
			name:        "ABeamNotNamedIsNotStarted",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}},
			named:       []string{"b2"},
			wantEntries: []StartEntry{{Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"b2"},
		},
		{
			name:        "AnUnnamedPrimeIsNeitherClaimedNorStarted",
			fake:        fakeStart{beams: []BeamPair{{Slug: "b1"}}},
			named:       []string{"b1"},
			wantEntries: []StartEntry{{Worktree: "b1", Outcome: OutcomeStarted}},
			wantSpawned: []string{"b1"},
		},
		{
			name:           "ABeamWhoseLandingHoldStandsIsRefusedNamingBattenStatusAndSpawnsNothing",
			fake:           fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, held: map[string]bool{"b1": true}},
			wantEntries:    []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeRefused}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned:    []string{"hub-prime", "b2"},
			wantBeamDetail: "lym batten status b1",
		},
		{
			name:        "AFailedLandingHoldReadIsThatWorktreesFailureAndTheOthersStart",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, heldErr: map[string]error{"b1": errors.New("board unreadable")}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeFailed}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b2"},
			wantErr:     "b1: read whether the landing hold stands: board unreadable",
		},
		{
			name:        "ANilLandingHeldReadsAsNoHold",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}}, held: map[string]bool{"b1": true}, noHoldSeam: true},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b1"},
		},
		{
			name:        "OneFailedSpawnStillStartsTheRestAndNamesItInTheError",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, failSpawn: map[string]bool{"b1": true}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeFailed}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b2"},
			wantErr:     "b1: start the dobby: exec failed",
		},
		{
			name:           "ABeamWithAnAnchorChangeStandingIsRefusedWithTheWayForwardWhileTheOthersStart",
			fake:           fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, moving: map[string]bool{"b1": true}},
			wantEntries:    []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeRefused}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned:    []string{"hub-prime", "b2"},
			wantBeamDetail: "lym fabric anchor b1 --set sub",
		},
		{
			name:        "APrimeWithAnAnchorChangeStandingIsRefusedAndTheBeamsStart",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}}, moving: map[string]bool{"hub-prime": true}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeRefused}, {Worktree: "b1", Outcome: OutcomeStarted}},
			wantSpawned: []string{"b1"},
			wantDetails: []string{"lym fabric anchor hub-prime --set sub"},
		},
		{
			name:        "AFailedReadOfWhetherAChangeStandsReportsTheWorktreeFailed",
			fake:        fakeStart{primeOwner: "here", beams: []BeamPair{{Slug: "b1"}, {Slug: "b2"}}, moveErr: map[string]error{"b1": errors.New("record unreadable")}},
			wantEntries: []StartEntry{{Worktree: PrimeLabel, Outcome: OutcomeStarted}, {Worktree: "b1", Outcome: OutcomeFailed}, {Worktree: "b2", Outcome: OutcomeStarted}},
			wantSpawned: []string{"hub-prime", "b2"},
			wantErr:     "b1: read whether an anchor change stands: record unreadable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := tt.fake
			deps := fake.deps()
			calls := max(tt.calls, 1)
			named := tt.named
			if named == nil {
				named = fake.allWorktrees()
			}
			var report StartReport
			var err error
			for range calls {
				report, err = EnsureLive(deps, named)
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
			if tt.wantBeamDetail != "" && !strings.Contains(report.Entries[1].Detail, tt.wantBeamDetail) {
				t.Errorf("EnsureLive() first beam detail = %q; want it to carry %q", report.Entries[1].Detail, tt.wantBeamDetail)
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

// TestOwnedWorktrees asserts the list holds the prime when this machine owns it or none is recorded, then every beam not owned elsewhere.
func TestOwnedWorktrees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fake fakeStart
		want []string
	}{
		{
			name: "NoPrimeOwnerListsThePrimeAndEveryUnownedOrOwnedBeam",
			fake: fakeStart{beams: []BeamPair{{Slug: "b1"}, {Slug: "b2", Owner: "here"}}},
			want: []string{"hub-prime", "b1", "b2"},
		},
		{
			name: "APrimeAndABeamOwnedElsewhereAreLeftOut",
			fake: fakeStart{primeOwner: "there", beams: []BeamPair{{Slug: "b1", Owner: "there"}, {Slug: "b2", Owner: "here"}}},
			want: []string{"b2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := tt.fake
			got, err := OwnedWorktrees(fake.deps())
			if err != nil {
				t.Fatalf("OwnedWorktrees() error = %v; want nil", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("OwnedWorktrees() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestLive asserts a dobby reads live exactly while its lock is held, and a worktree with no beam directory reads not live.
func TestLive(t *testing.T) {
	t.Parallel()

	t.Run("NoBeamDirectoryReadsNotLive", func(t *testing.T) {
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

func TestSpawnDetached_SendsOutputToOutPath(t *testing.T) {
	t.Parallel()
	beamDir := filepath.Join(t.TempDir(), ".lym", "beam")
	if err := os.MkdirAll(beamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	geom := Geometry{
		HubRoot:       "/hub",
		WorktreeRoot:  "/hub/prime",
		LymExecutable: "/bin/lym",
		LockPath:      filepath.Join(beamDir, LockFileName),
		LogPath:       filepath.Join(beamDir, LogFileName),
		OutPath:       filepath.Join(beamDir, OutFileName),
	}
	writeFile(t, geom.LogPath, "own line\n")

	var captured *exec.Cmd
	err := spawnDetached(geom, func(cmd *exec.Cmd) error {
		captured = cmd
		return nil
	})

	if err != nil {
		t.Fatalf("spawnDetached() = %v; want nil", err)
	}
	if _, err := os.Stat(geom.OutPath); err != nil {
		t.Errorf("stat of OutPath = %v; want it created", err)
	}
	for name, stream := range map[string]any{"stdout": captured.Stdout, "stderr": captured.Stderr} {
		if file, ok := stream.(*os.File); !ok || file.Name() != geom.OutPath {
			t.Errorf("command %s = %v; want the file opened at OutPath", name, stream)
		}
	}
	if want := []string{"/bin/lym", "beam", "dobby", "/hub/prime"}; !slices.Equal(captured.Args, want) || captured.Dir != "/hub" {
		t.Errorf("command = %v in %q; want %v in /hub", captured.Args, captured.Dir, want)
	}
	if got := readFile(t, geom.LogPath); got != "own line\n" {
		t.Errorf("dobby.log = %q; want it untouched", got)
	}

	failure := errors.New("no such binary")
	if err := spawnDetached(geom, func(*exec.Cmd) error { return failure }); !errors.Is(err, failure) {
		t.Errorf("spawnDetached() with a failing start = %v; want %v", err, failure)
	}
}
