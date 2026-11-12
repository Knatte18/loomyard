// handover_test.go pins what the loop does on a probe tick that reads the beam owned by another machine:
// the adopt hold, the hand-over's pause and archive, and the Bolt pull through which an idle machine learns the owner.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// handOverStop is the log line the prime's dobby writes as it exits for machine m2.
const handOverStop = "stop: another machine owns this beam (m2); lym adopt root takes it over here"

// loggedContaining reports whether any logged line contains text.
func (h *loopHarness) loggedContaining(text string) bool {
	return slices.ContainsFunc(h.logged(), func(call loopLogCall) bool { return strings.Contains(call.line, text) })
}

func TestRun_ForeignOwner(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		underAdopt bool
		// archiveErrs answers the archive calls in order; a call past the list succeeds.
		archiveErrs []error
		// runs replaces the default running run r1 when set, and runsErr fails the run listing instead.
		runs    []RunView
		runsErr error
		// recordedElsewhere names the runs the state record already holds owned elsewhere.
		recordedElsewhere []string
		wantArchives      int
		wantPauses        []string
		wantExit          bool
		wantLine          string
	}{
		{name: "AnAdoptInFlightKeepsTheDobbyUpAndIdle", underAdopt: true, wantLine: "adopt hold:"},
		{name: "NoAdoptPausesTheRunsArchivesAndExits", wantArchives: 1, wantPauses: []string{"r1"}, wantExit: true, wantLine: handOverStop},
		{
			name: "ADoneRunIsNotPaused", runs: []RunView{{Slug: "r1", State: runStateDone, Producer: producerRunShed}},
			wantArchives: 1, wantExit: true, wantLine: handOverStop,
		},
		{
			name: "ARunAlreadyOwnedElsewhereIsNotPausedAgain", recordedElsewhere: []string{"r1"},
			wantArchives: 1, wantExit: true, wantLine: handOverStop,
		},
		{
			name: "AFailingRunListingPausesNothingAndStillArchivesAndExits", runsErr: errors.New("status unreadable"),
			wantArchives: 1, wantExit: true, wantLine: handOverStop,
		},
		{
			name: "AFailedArchiveKeepsTheDobbyUpAndRetriesAtTheNextProbe", archiveErrs: []error{errors.New("origin unreachable")},
			wantArchives: 2, wantPauses: []string{"r1"}, wantExit: true, wantLine: "archive failing: origin unreachable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newLoopHarness(t, 3)
			var pauses, spawns []string
			archives := 0
			h.deps.Owner = func() (string, error) { return "m2", nil }
			h.deps.SlugOwner = func(string) (string, error) { return "m2", nil }
			h.deps.UnderAdopt = func() bool { return tt.underAdopt }
			h.deps.Runs = func() ([]RunView, error) {
				if tt.runsErr != nil || tt.runs != nil {
					return tt.runs, tt.runsErr
				}
				return []RunView{{Slug: "r1", State: runStateRunning, Producer: producerRunShed, Covered: len(pauses) > 0}}, nil
			}
			if len(tt.recordedElsewhere) > 0 {
				err := UpdateState(h.geom.StatePath, func(s *State) error {
					s.Runs = map[string]RunState{}
					for _, slug := range tt.recordedElsewhere {
						s.Runs[slug] = RunState{OwnedElsewhere: true}
					}
					return nil
				})
				if err != nil {
					t.Fatalf("UpdateState() = %v; want nil", err)
				}
			}
			h.deps.PauseOwned = func(_ context.Context, slug string) error {
				pauses = append(pauses, slug)
				return nil
			}
			h.deps.ReleaseParentLock = func(context.Context, string) error { return nil }
			h.deps.SpawnPass = func(slug string, _, _ bool) (<-chan PassExit, error) {
				spawns = append(spawns, slug)
				return nil, ErrSpawnUnderTest
			}
			h.deps.ArchiveOwner = func() error {
				archives++
				if archives <= len(tt.archiveErrs) {
					return tt.archiveErrs[archives-1]
				}
				return nil
			}

			if err := h.run(t); err != nil {
				t.Fatalf("Run() = %v; want nil", err)
			}

			if archives != tt.wantArchives {
				t.Errorf("archives = %d; want %d", archives, tt.wantArchives)
			}
			if !slices.Equal(pauses, tt.wantPauses) {
				t.Errorf("owner pauses = %v; want %v", pauses, tt.wantPauses)
			}
			if len(spawns) != 0 {
				t.Errorf("passes spawned = %v; want none, held or paused", spawns)
			}
			if got := h.loggedContaining(handOverStop); got != tt.wantExit {
				t.Errorf("logged the hand-over stop = %v; want %v", got, tt.wantExit)
			}
			if !h.loggedContaining(tt.wantLine) {
				t.Errorf("log = %v; want a line containing %q", h.logged(), tt.wantLine)
			}
		})
	}
}

// adoptHoldRun is what a beam dobby did across a run that held it under an adopt for two ticks and then lifted the adopt.
type adoptHoldRun struct {
	h                    *loopHarness
	spawns, pauses       []string
	tidyAgents, archives int
	refreshes, landReads int
	landings             int
}

// runAnAdoptHold runs a beam dobby of a beam another machine owns, under an adopt for its first two ticks.
// Every spy must stay silent through those ticks.
// The adopt then lifts, and the owner reads ownerAfterLift from the next probe on.
func runAnAdoptHold(t *testing.T, ownerAfterLift string, stopAt int) adoptHoldRun {
	t.Helper()

	h := newLoopHarness(t, stopAt)
	h.geom.Slug, h.geom.WorktreeRoot = "beam-a", t.TempDir()
	if err := os.WriteFile(filepath.Join(h.geom.WorktreeRoot, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)
	h.geom.TidyDir = filepath.Join(filepath.Dir(h.geom.StatePath), TidyDirName)

	underAdopt, lifted := true, false
	var spawns, pauses []string
	shuttle := &tidyShuttleFake{}
	archives, refreshes, landReads, landings, landingReads := 0, 0, 0, 0, 0
	h.deps.Owner = func() (string, error) {
		if lifted {
			return ownerAfterLift, nil
		}
		return "m2", nil
	}
	h.deps.SlugOwner = func(string) (string, error) { return "m2", nil }
	h.deps.UnderAdopt = func() bool { return underAdopt }
	h.deps.Runs = func() ([]RunView, error) { return []RunView{runningRun("due")}, nil }
	h.deps.SpawnPass = func(slug string, _, _ bool) (<-chan PassExit, error) {
		spawns = append(spawns, slug)
		exit := make(chan PassExit, 1)
		exit <- PassExit{}
		return exit, nil
	}
	h.deps.PauseOwned = func(_ context.Context, slug string) error {
		pauses = append(pauses, slug)
		return nil
	}
	h.deps.ReleaseParentLock = func(context.Context, string) error { return nil }
	h.deps.StatusExists = func(string) (bool, error) { return true, nil }
	h.deps.PauseCovers = func(string) (bool, error) { return false, nil }
	h.deps.Tasks = func() ([]TaskView, error) {
		return []TaskView{{Slug: "fresh", Kind: boardengine.KindTask, Parent: h.geom.WorktreeName}}, nil
	}
	h.deps.LoadConfig = func() (Config, error) {
		return Config{MaxRuns: 5, BoardTidyEvery: 1, BoardTidyTimeoutMin: 30, TestMapUpkeep: true}, nil
	}
	h.deps.TestMapRunning = func() (bool, error) { return false, nil }
	h.deps.TestMapExitRecord = func() (TestMapExit, bool, error) { return TestMapExit{}, false, nil }
	h.deps.StartTestMap = func() (<-chan PassExit, error) {
		refreshes++
		return make(chan PassExit), nil
	}
	h.deps.Landings = func() ([]string, error) {
		landingReads++
		if landingReads == 1 {
			return nil, nil
		}
		return []string{"landed"}, nil
	}
	h.deps.TidyShuttle = shuttle
	h.deps.TidyModel = shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"}
	h.deps.StencilsDir = stencils
	h.deps.NearestOrch = func() (string, error) { return "ly:orch", nil }
	h.deps.CanLand = func() (LandReading, error) {
		landReads++
		return LandReading{}, nil
	}
	h.deps.Land = func() error {
		landings++
		return nil
	}
	h.deps.ArchiveOwner = func() error {
		archives++
		return nil
	}
	h.perTick = func(n int) error {
		if n > 2 {
			return nil
		}
		if len(spawns) != 0 || len(shuttle.started) != 0 || refreshes != 0 || landReads != 0 || landings != 0 {
			t.Errorf("after tick %d under the hold, passes = %v, tidy agents = %d, refreshes = %d, landing reads = %d, landings = %d; want none",
				n, spawns, len(shuttle.started), refreshes, landReads, landings)
		}
		if n == 2 {
			underAdopt, lifted = false, true
		}
		return nil
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	return adoptHoldRun{
		h: h, spawns: spawns, pauses: pauses, tidyAgents: len(shuttle.started),
		archives: archives, refreshes: refreshes, landReads: landReads, landings: landings,
	}
}

// TestRun_AnAdoptHoldSchedulesNothingUntilItLifts pins that the hold gates every scheduling step.
// It also pins that the probe after the lift hands over.
func TestRun_AnAdoptHoldSchedulesNothingUntilItLifts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := runAnAdoptHold(t, "m2", 10)

	if len(got.h.ticks) != 2 {
		t.Errorf("ticks = %d; want the two held ticks, the probe after the lift handing over before a third", len(got.h.ticks))
	}
	if got.archives != 1 || !slices.Equal(got.pauses, []string{"due"}) {
		t.Errorf("archives = %d, owner pauses = %v; want one archive and the run paused", got.archives, got.pauses)
	}
	for _, want := range []string{"adopt hold:", "adopt hold ended", "lym adopt beam-a takes it over here"} {
		if !got.h.loggedContaining(want) {
			t.Errorf("log = %v; want a line containing %q", got.h.logged(), want)
		}
	}
}

// TestRun_ABeamTheHoldReleasedSchedulesAgain is the control of the hold test.
// It runs the same beam, with the owner leaving as the adopt lifts.
// It proves each spy fires once nothing holds the beam, so the held ticks' silence is the hold's doing.
func TestRun_ABeamTheHoldReleasedSchedulesAgain(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := runAnAdoptHold(t, "", 5)

	if len(got.h.ticks) != 5 || got.archives != 0 {
		t.Errorf("ticks = %d, archives = %d; want all five ticks and no archive", len(got.h.ticks), got.archives)
	}
	if len(got.spawns) == 0 || got.tidyAgents == 0 || got.refreshes == 0 || got.landReads == 0 {
		t.Errorf("after the hold, passes = %v, tidy agents = %d, refreshes = %d, landing reads = %d; want each step to have run",
			got.spawns, got.tidyAgents, got.refreshes, got.landReads)
	}
	if !got.h.loggedContaining("adopt hold ended") {
		t.Errorf("log = %v; want a line containing %q", got.h.logged(), "adopt hold ended")
	}
}

// TestRun_ALaterArchiveFailureLogsAgainAfterTheOwnerLeaves reads the beam owned elsewhere, then not.
// It then reads it owned elsewhere again.
// Each archive failure episode writes its own dobby log line.
func TestRun_ALaterArchiveFailureLogsAgainAfterTheOwnerLeaves(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 4)
	owners := []string{"m2", "", "m2", "m2"}
	reads := 0
	h.deps.Owner = func() (string, error) {
		owner := owners[min(reads, len(owners)-1)]
		reads++
		return owner, nil
	}
	h.deps.ArchiveOwner = func() error { return errors.New("origin unreachable") }

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}

	lines := 0
	for _, call := range h.logged() {
		if strings.Contains(call.line, "archive failing:") {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("archive failing lines = %d; want one for each of the two episodes", lines)
	}
}

func TestRun_AFailedBoltPullLeavesTheOwnerReadToTheLocalBolt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 1000)
	pulls, archives := 0, 0
	h.deps.PullBolt = func() error {
		pulls++
		return errors.New("origin unreachable")
	}
	h.deps.Owner = func() (string, error) { return "m2", nil }
	h.deps.ArchiveOwner = func() error {
		archives++
		return nil
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}

	if pulls != 1 || archives != 1 || !h.loggedContaining(handOverStop) {
		t.Errorf("pulls = %d, archives = %d, logged the hand-over stop = %v; want one failed pull, then the local owner handed over",
			pulls, archives, h.loggedContaining(handOverStop))
	}
}

func TestRun_BoltPullBringsInANewOwner(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 1000)
	owner := ""
	var pulls []time.Time
	archives := 0
	h.deps.PullBolt = func() error {
		pulls = append(pulls, h.clock.now())
		if len(pulls) == 2 {
			owner = "m2"
		}
		return nil
	}
	h.deps.Owner = func() (string, error) { return owner, nil }
	h.deps.ArchiveOwner = func() error {
		archives++
		return nil
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}

	if len(pulls) != 2 {
		t.Fatalf("Bolt pulls = %d; want 2, the second bringing in the owner its own probe acts on", len(pulls))
	}
	if gap := pulls[1].Sub(pulls[0]); gap < boltPullInterval {
		t.Errorf("gap between Bolt pulls = %v; want at least %v", gap, boltPullInterval)
	}
	if archives != 1 || !h.loggedContaining(handOverStop) {
		t.Errorf("archives = %d, logged the hand-over stop = %v; want one archive and the stop", archives, h.loggedContaining(handOverStop))
	}
}
