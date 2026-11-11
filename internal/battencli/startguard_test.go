// startguard_test.go pins the pre-run guard's refusals over fake seams.
// It also pins the seed write's re-read of the cancel marker.

package battencli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStartGuard_Refuse drives startGuard.refuse over fake seams.
// It covers which verbs the cancel marker refuses, when a dirty source refuses, and which pair parts refuse a start.
// It also covers a failing probe, which refuses naming the re-run.
func TestStartGuard_Refuse(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "some-slug"
	markerAlive := &cancelMarkerReading{Present: true, WriterAlive: true, Marker: cancelMarker{PID: 4242, StartedAt: "2026-10-23T08:00:00Z"}}
	markerGone := &cancelMarkerReading{Present: true}
	tests := []struct {
		name  string
		verbs []string
		// marker is what the Marker seam reads; nil reads absent.
		marker *cancelMarkerReading
		dirty  []string
		parts  fabricengine.PairParts
		// found, producer and taskPresent describe the run's status file and task worktree.
		found       bool
		producer    string
		taskPresent bool
		// parentBeam is the beam the ParentBeam seam reads; empty reads the prime.
		parentBeam string
		markerErr  error
		dirtyErr   error
		partsErr   error
		beamErr    error
		// promoted is what the Promoted seam reads: a task whose pair carries the beam mark.
		promoted    bool
		promotedErr error
		// reached is what the ReachedParent seam reads: the run's work reached its parent.
		reached    bool
		reachedErr error
		// project and beam feed the Project seam; marked and unreadable feed the PairMark seam.
		project, beam, marked, unreadable bool
		projectErr                        error
		// unreleased is the board status of a task no run starts; it is empty for a released entry.
		unreleased    string
		unreleasedErr error
		// raddle is the Raddle seam; nil leaves it unset.
		raddle func() error
		// noSeams leaves every seam nil.
		noSeams bool
		// wantExact is the refusal's whole text, "{verb}" standing for the verb under test.
		wantExact string
		// wantErrors are substrings the refusal carries; "{verb}" stands for the verb under test.
		wantErrors []string
	}{
		{
			name:       "MarkerWithTheWriterAliveRefusesTheDrivingVerbs",
			verbs:      []string{"run", "step", "goto"},
			marker:     markerAlive,
			found:      true,
			producer:   battenrecipe.NameRunShed,
			wantErrors: []string{"a cancel of some-slug is under way", "way forward: wait for the cancel (pid 4242)", `"ps -p 4242"`},
		},
		{
			name:       "MarkerWithTheWriterGoneRefusesTheDrivingVerbs",
			verbs:      []string{"run", "step", "goto"},
			marker:     markerGone,
			found:      true,
			producer:   battenrecipe.NameRunShed,
			wantErrors: []string{"a cancel of some-slug was interrupted", `way forward: run "lym batten cancel some-slug"`},
		},
		{
			name:     "MarkerPassesTheVerbsThatDoNotDrive",
			verbs:    []string{"status", "pause"},
			marker:   markerAlive,
			dirty:    []string{"a.txt"},
			parts:    fabricengine.PairParts{TaskWorktree: true},
			found:    false,
			producer: "",
		},
		{
			name:       "DirtyStartRefusesNamingEachPathAndTheRerun",
			verbs:      []string{"run", "step"},
			dirty:      []string{"a.txt", "dir/b.txt"},
			wantErrors: []string{"a.txt, dir/b.txt", "commit or revert", `run "lym batten {verb} some-slug" again`},
		},
		{
			// A released task reads a nil status, so the Unreleased seam reads empty.
			name:  "CleanStartPasses",
			verbs: []string{"run", "step"},
		},
		{
			name:       "DirtySourcePassesABeamMembersStart",
			verbs:      []string{"run", "step"},
			dirty:      []string{"a.txt"},
			parentBeam: "b",
		},
		{
			name:       "DirtySourceStillRefusesAPrimeTasksStart",
			verbs:      []string{"run", "step"},
			dirty:      []string{"a.txt"},
			wantErrors: []string{"a.txt", "commit or revert"},
		},
		{
			name:       "BeamProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			beamErr:    errors.New("board unreadable"),
			wantErrors: []string{"the task's beam", "board unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "LeftPartsStillRefuseABeamMembersStart",
			verbs:      []string{"run", "step"},
			parts:      fabricengine.PairParts{TaskWorktree: true},
			parentBeam: "b",
			wantErrors: []string{"some-slug left a pair from an earlier run (task worktree)"},
		},
		{
			name:       "ResumeAtWorktreeCreateWithNoTaskWorktreeRefusesWhenDirty",
			verbs:      []string{"run", "step"},
			dirty:      []string{"a.txt"},
			found:      true,
			producer:   battenrecipe.NameWorktreeCreate,
			wantErrors: []string{"a.txt", "commit or revert"},
		},
		{
			name:        "ResumeAtWorktreeCreateWithTheTaskWorktreePresentPassesWhileDirty",
			verbs:       []string{"run", "step"},
			dirty:       []string{"a.txt"},
			found:       true,
			producer:    battenrecipe.NameWorktreeCreate,
			taskPresent: true,
		},
		{
			name:     "ResumePastWorktreeCreatePassesWhileDirty",
			verbs:    []string{"run", "step"},
			dirty:    []string{"a.txt"},
			found:    true,
			producer: battenrecipe.NameRunShed,
		},
		{
			name:  "GotoRunsNoDirtyCheck",
			verbs: []string{"goto"},
			dirty: []string{"a.txt"},
		},
		{
			name:       "LeftTaskWorktreeRefusesAStart",
			verbs:      []string{"run", "step"},
			parts:      fabricengine.PairParts{TaskWorktree: true},
			wantErrors: []string{"some-slug left a pair from an earlier run (task worktree)", `run "lym batten cancel some-slug --remove" from the prime`, `then "lym board set-status '{"slug":"some-slug","status":null}'" and "lym batten {verb} some-slug"`},
		},
		{
			name:       "DraftTaskRefusesAFreshStartNamingTheRelease",
			verbs:      []string{"run", "step"},
			unreleased: boardengine.StatusDraft,
			wantExact:  "some-slug is a draft task, which no run starts; way forward: run `lym board set-status '{\"slug\":\"some-slug\",\"status\":null}'` to release it, then `lym batten {verb} some-slug`",
		},
		{
			name:       "DiscussedTaskRefusesAFreshStartNamingTheRelease",
			verbs:      []string{"run", "step"},
			unreleased: boardengine.LegacyStatusDiscussed,
			wantExact:  "some-slug is a discussed task, which no run starts; way forward: run `lym board set-status '{\"slug\":\"some-slug\",\"status\":null}'` to release it, then `lym batten {verb} some-slug`",
		},
		{
			name:       "UnreleasedTaskWithARunStatusFileResumes",
			verbs:      []string{"run", "step"},
			unreleased: boardengine.StatusDraft,
			found:      true,
			producer:   battenrecipe.NameRunShed,
		},
		{
			name:          "UnreleasedProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:         []string{"run", "step"},
			unreleasedErr: errors.New("board unreadable"),
			wantErrors:    []string{"the board entry's status", "board unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "LeftRecordsWorktreeRefusesAStart",
			verbs:      []string{"run", "step"},
			parts:      fabricengine.PairParts{RecordsWorktree: true},
			wantErrors: []string{"(records worktree)", "lym batten cancel some-slug --remove"},
		},
		{
			name:       "LeftRecordsBranchRefusesAStart",
			verbs:      []string{"run", "step"},
			parts:      fabricengine.PairParts{RecordsBranch: true},
			wantErrors: []string{"(records branch)", "lym batten cancel some-slug --remove"},
		},
		{
			name:  "LeftTaskBranchAlonePasses",
			verbs: []string{"run", "step"},
			parts: fabricengine.PairParts{TaskBranch: true},
		},
		{
			name:     "LeftPartsDoNotRefuseAResume",
			verbs:    []string{"run", "step"},
			parts:    fabricengine.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true},
			found:    true,
			producer: battenrecipe.NameRunShed,
		},
		{
			name:       "MarkerProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step", "goto"},
			markerErr:  errors.New("marker unreadable"),
			wantErrors: []string{"the cancel marker", "marker unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "DirtyProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			dirtyErr:   errors.New("git status failed"),
			wantErrors: []string{"the source worktree's tracked changes", "git status failed", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "PartsProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			partsErr:   errors.New("worktree list failed"),
			wantErrors: []string{"the pair's parts", "worktree list failed", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "MarkedPairTaskRefusesNamingThePromoteWithNoStatusFile",
			verbs:      []string{"run", "step"},
			promoted:   true,
			wantErrors: []string{"carries the beam mark", `run "lym batten cancel some-slug --remove" from the prime`},
		},
		{
			name:       "MarkedPairTaskRefusesNamingThePromoteWithAStatusFileFound",
			verbs:      []string{"run", "step"},
			promoted:   true,
			found:      true,
			producer:   battenrecipe.NameRunShed,
			wantErrors: []string{"carries the beam mark", `run "lym batten cancel some-slug --remove" from the prime`},
		},
		{
			name:     "MarkedPairTaskWhoseWorkReachedItsParentPasses",
			verbs:    []string{"run", "step"},
			promoted: true,
			reached:  true,
			found:    true,
			producer: battenrecipe.NameWorktreeTeardown,
		},
		{
			name:       "ReachedParentProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			promoted:   true,
			reachedErr: errors.New("child status unreadable"),
			found:      true,
			producer:   battenrecipe.NameLand,
			wantErrors: []string{"whether the work reached its parent", "child status unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:     "MarkedPairPassesTheVerbsThatDoNotStartARun",
			verbs:    []string{"goto"},
			promoted: true,
		},
		{
			name:        "PromotedProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:       []string{"run", "step"},
			promotedErr: errors.New("origin record unreadable"),
			wantErrors:  []string{"the pair's origin record", "origin record unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:       "GroupingProjectRefusedNamingTheCreate",
			verbs:      []string{"run", "step"},
			project:    true,
			wantErrors: []string{"grouping project", `run "lym beam create some-slug"`},
		},
		{
			name:    "FreshBeamProjectWithNoPairPassesOverACleanPrime",
			verbs:   []string{"run", "step"},
			project: true,
			beam:    true,
		},
		{
			name:       "FreshBeamProjectWithNoPairIsRefusedOverADirtyPrimeWhenItForksFromThePrime",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			dirty:      []string{"a.txt"},
			wantErrors: []string{"a.txt", "commit or revert"},
		},
		{
			name:       "FreshBeamProjectForkedFromAParentBeamPassesOverADirtyPrime",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			dirty:      []string{"a.txt"},
			parentBeam: "outer",
		},
		{
			name:       "FreshBeamProjectWithLeftRecordPartsIsRefused",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			parts:      fabricengine.PairParts{RecordsWorktree: true},
			wantErrors: []string{"some-slug left a pair from an earlier run (records worktree)"},
		},
		{
			name:       "BeamProjectWhoseStatusIsDraftPassesWhereADraftTaskIsRefused",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			unreleased: boardengine.StatusDraft,
		},
		{
			name:       "UnmarkedBeamPairIsJudgedAsAFreshStart",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			parts:      fabricengine.PairParts{TaskWorktree: true},
			wantErrors: []string{"some-slug left a pair from an earlier run (task worktree)"},
		},
		{
			name:       "UnreadableBeamPairRefusedNamingReconcileAndNotTheCancel",
			verbs:      []string{"run", "step"},
			project:    true,
			beam:       true,
			unreadable: true,
			parts:      fabricengine.PairParts{TaskWorktree: true},
			wantErrors: []string{"no readable origin record", `"lym fabric reconcile"`},
		},
		{
			name:    "MarkedBeamPairPassesTheDirtAndLeftPairChecks",
			verbs:   []string{"run", "step"},
			project: true,
			beam:    true,
			marked:  true,
			parts:   fabricengine.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true},
			dirty:   []string{"a.txt"},
		},
		{
			name:     "BeamResumePastWorktreeCreateJudgesNothing",
			verbs:    []string{"run", "step"},
			project:  true,
			beam:     true,
			found:    true,
			producer: battenrecipe.NameLand,
			dirty:    []string{"a.txt"},
		},
		{
			name:       "ProjectProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			projectErr: errors.New("board unreadable"),
			wantErrors: []string{"the board entry", "board unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:      "RaddleRefusalOfAFreshStartIsReturnedAsItStands",
			verbs:     []string{"run", "step"},
			raddle:    func() error { return raddleRefusal{"some-slug holds no record"} },
			wantExact: "some-slug holds no record",
		},
		{
			name:     "RaddleRefusalIsNotJudgedForAResume",
			verbs:    []string{"run", "step"},
			raddle:   func() error { return raddleRefusal{"some-slug holds no record"} },
			found:    true,
			producer: battenrecipe.NameRunShed,
		},
		{
			name:       "RaddleProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			raddle:     func() error { return errors.New("store unreadable") },
			wantErrors: []string{"the raddle entry's start", "store unreadable", `run "lym batten {verb} some-slug" again`},
		},
		{
			name:    "NilSeamsCheckNothing",
			verbs:   []string{"run", "step", "goto"},
			noSeams: true,
		},
	}
	for _, tt := range tests {
		for _, verb := range tt.verbs {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				var guard startGuard
				if !tt.noSeams {
					guard = startGuard{
						Marker: func() (cancelMarkerReading, error) {
							if tt.marker == nil {
								return cancelMarkerReading{}, tt.markerErr
							}
							return *tt.marker, tt.markerErr
						},
						SourceDirty: func() ([]string, error) { return tt.dirty, tt.dirtyErr },
						PairParts:   func() (fabricengine.PairParts, error) { return tt.parts, tt.partsErr },
						ParentBeam:  func() (string, error) { return tt.parentBeam, tt.beamErr },
						Promoted:    func() (bool, error) { return tt.promoted, tt.promotedErr },
						ReachedParent: func(shedengine.Status, bool) (bool, error) {
							return tt.reached, tt.reachedErr
						},
						Project:    func() (bool, bool, error) { return tt.project, tt.beam, tt.projectErr },
						PairMark:   func() (bool, bool, error) { return tt.marked, !tt.unreadable, nil },
						Unreleased: func() (string, error) { return tt.unreleased, tt.unreleasedErr },
						Raddle:     tt.raddle,
					}
				}

				err := guard.refuse(slug, verb, shedengine.Status{CurrentProducer: tt.producer}, tt.found, tt.taskPresent)

				if tt.wantExact != "" {
					want := strings.ReplaceAll(tt.wantExact, "{verb}", verb)
					if err == nil || err.Error() != want {
						t.Errorf("refuse(%s) = %v; want %q", verb, err, want)
					}
					return
				}
				if len(tt.wantErrors) == 0 {
					if err != nil {
						t.Fatalf("refuse(%s) = %v; want nil", verb, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("refuse(%s) = nil; want a refusal carrying %q", verb, tt.wantErrors)
				}
				for _, want := range tt.wantErrors {
					want = strings.ReplaceAll(want, "{verb}", verb)
					if !strings.Contains(err.Error(), want) {
						t.Errorf("refuse(%s) = %q; want it to carry %q", verb, err.Error(), want)
					}
				}
			})
		}
	}
}

// writeBoardHub writes the board of the hub at location: board.json holding boardJSON.
// It also writes the Bolt checkout's board config, which names defaultRecipe when that is not empty.
func writeBoardHub(t *testing.T, location *lymcwd.Location, defaultRecipe, boardJSON string) {
	t.Helper()
	boltDir := fabricengine.BoltDir(location.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(boltDir), 0o755); err != nil {
		t.Fatalf("create the config directory: %v", err)
	}
	config := "readme: Home.md\n"
	if defaultRecipe != "" {
		config = "default_recipe: " + defaultRecipe + "\n"
	}
	if err := os.WriteFile(configengine.ConfigFile(boltDir, "board"), []byte(config), 0o644); err != nil {
		t.Fatalf("write the board config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(location.HubPath), "board.json"), []byte(boardJSON), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}

// Over a status file and fake seams, only a run or step over a done status asks whether the start clears the run.
// A start that does is judged as a fresh start, with the removal left pending.
// Nothing is removed whatever the guard answers, and a failed read refuses naming the re-run.
func TestRefuseAtStart_JudgesADoneMotifRunAsTheFreshStartItsClearMakes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	clearsErr := errors.New("child recipe unreadable")
	refusal := raddleRefusal{"some-slug holds no record"}
	tests := []struct {
		name         string
		verb         string
		state        shedengine.State
		clears       bool
		clearsErr    error
		guardRefusal error
		wantAsked    bool
		wantFresh    bool
		wantPending  bool
	}{
		{name: "RunOverAClearingStartIsAFreshStart", verb: "run", state: shedengine.StateDone, clears: true, wantAsked: true, wantFresh: true, wantPending: true},
		{name: "StepOverAClearingStartIsAFreshStart", verb: "step", state: shedengine.StateDone, clears: true, wantAsked: true, wantFresh: true, wantPending: true},
		{name: "ADoneRunTheStartDoesNotClearIsNoFreshStart", verb: "run", state: shedengine.StateDone, wantAsked: true},
		{name: "AFailedReadRefuses", verb: "run", state: shedengine.StateDone, clearsErr: clearsErr, wantAsked: true},
		{name: "AGuardRefusalOfAClearingStartLeavesTheRunInPlace", verb: "run", state: shedengine.StateDone, clears: true, guardRefusal: refusal, wantAsked: true, wantFresh: true, wantPending: true},
		{name: "ARunningRunIsNotAsked", verb: "run", state: shedengine.StateRunning},
		{name: "GotoIsNotAsked", verb: "goto", state: shedengine.StateDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			path := shedrun.StatusFile(location, "some-slug")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create the run directory: %v", err)
			}
			if err := os.WriteFile(path, []byte(`{"state":"`+string(tt.state)+`"}`), 0o644); err != nil {
				t.Fatalf("write the run status: %v", err)
			}

			asked, fresh, removed := false, false, false
			c := &battenCLI{location: location, slug: "some-slug", guard: startGuard{
				ClearsMotifRun: func() (bool, error) {
					asked = true
					return tt.clears, tt.clearsErr
				},
				ClearMotifRun: func() (bool, error) {
					removed = true
					return true, nil
				},
				Raddle: func() error {
					fresh = true
					return tt.guardRefusal
				},
			}}

			err := c.refuseAtStart(tt.verb)

			wantErr := tt.clearsErr
			if tt.guardRefusal != nil {
				wantErr = tt.guardRefusal
			}
			if wantErr == nil && err != nil {
				t.Fatalf("refuseAtStart(%q) error = %v; want nil", tt.verb, err)
			}
			if wantErr != nil && !errors.Is(err, wantErr) {
				t.Fatalf("refuseAtStart(%q) error = %v; want it to wrap %v", tt.verb, err, wantErr)
			}
			if tt.clearsErr != nil && !strings.Contains(err.Error(), `"lym batten run some-slug"`) {
				t.Errorf("refuseAtStart(%q) error = %q; want it to name the re-run", tt.verb, err)
			}
			if asked != tt.wantAsked {
				t.Errorf("ClearsMotifRun called = %v; want %v", asked, tt.wantAsked)
			}
			if fresh != tt.wantFresh {
				t.Errorf("the start judged fresh = %v; want %v", fresh, tt.wantFresh)
			}
			if c.motifRunClearPending != tt.wantPending {
				t.Errorf("motifRunClearPending = %v; want %v", c.motifRunClearPending, tt.wantPending)
			}
			if removed {
				t.Errorf("ClearMotifRun called by refuseAtStart; want the removal left to claimStart")
			}
		})
	}
}

// Over a status file and the fake owners, a start replacing a done motif run is judged by the owner check first.
// The run is removed only when that check passes.
// A refused owner check leaves the run and its pending removal in place.
// An entry the owner check's pull turned away from raddle leaves the run in place.
// A failed removal refuses naming the re-run.
func TestClaimStart_RemovesTheDoneMotifRunOnlyOnceTheOwnerCheckPasses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	removeErr := errors.New("records commit refused")
	tests := []struct {
		name        string
		verb        string
		pending     bool
		owner       string
		removeErr   error
		wantOwned   bool
		wantPulls   int
		wantRemoved int
		wantPending bool
		// turnedAway makes the guard read the entry as not replacing the run after the owner check pulls the board.
		turnedAway bool
	}{
		{name: "AnOwnerCheckThatPassesRemovesTheRunOnce", verb: "run", pending: true, owner: "this", wantPulls: 1, wantRemoved: 1},
		{name: "AnOwnerCheckThatRefusesLeavesTheRunInPlace", verb: "run", pending: true, owner: "other", wantOwned: true, wantPulls: 1, wantPending: true},
		{name: "AFailedRemovalRefusesNamingTheRerun", verb: "step", pending: true, owner: "this", removeErr: removeErr, wantPulls: 1, wantRemoved: 1, wantPending: true},
		{name: "AnEntryThePullTurnedAwayFromRaddleLeavesTheRunInPlace", verb: "run", pending: true, owner: "this", turnedAway: true, wantPulls: 1},
		{name: "ADoneRunTheStartDoesNotReplaceIsNeitherPulledNorRemoved", verb: "run", owner: "other"},
		{name: "GotoJudgesNoOwnerAndRemovesNothing", verb: "goto", pending: true, owner: "other", wantPending: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			path := shedrun.StatusFile(location, "some-slug")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create the run directory: %v", err)
			}
			if err := os.WriteFile(path, []byte(`{"state":"done"}`), 0o644); err != nil {
				t.Fatalf("write the run status: %v", err)
			}
			owners := &fakeOwners{machine: "this", owners: ownedBy("this", tt.owner, false)}
			removed := 0
			c := &battenCLI{location: location, slug: "some-slug", owner: owners.seams(), motifRunClearPending: tt.pending, guard: startGuard{
				ClearsMotifRun: func() (bool, error) { return !tt.turnedAway, nil },
				ClearMotifRun: func() (bool, error) {
					removed++
					return true, tt.removeErr
				},
			}}

			err := c.claimStart(tt.verb)

			var owned *OwnedElsewhereError
			if got := errors.As(err, &owned); got != tt.wantOwned {
				t.Errorf("claimStart(%q) error = %v; want an owned-elsewhere refusal = %v", tt.verb, err, tt.wantOwned)
			}
			if tt.removeErr != nil && (!errors.Is(err, tt.removeErr) || !strings.Contains(err.Error(), `"lym batten step some-slug"`)) {
				t.Errorf("claimStart(%q) error = %v; want it to wrap the removal error and name the re-run", tt.verb, err)
			}
			if !tt.wantOwned && tt.removeErr == nil && err != nil {
				t.Errorf("claimStart(%q) error = %v; want nil", tt.verb, err)
			}
			if owners.pulls != tt.wantPulls {
				t.Errorf("owner pulls = %d; want %d", owners.pulls, tt.wantPulls)
			}
			if removed != tt.wantRemoved {
				t.Errorf("ClearMotifRun calls = %d; want %d", removed, tt.wantRemoved)
			}
			if c.motifRunClearPending != tt.wantPending {
				t.Errorf("motifRunClearPending = %v; want %v", c.motifRunClearPending, tt.wantPending)
			}
		})
	}
}

// Over a fixture board and run directory, the guard reads whether a start clears the done run under its slug.
// A slug the board lacks and an entry of another recipe clear nothing.
// A raddle entry clears a done run whose child ran motif, not one whose child ran raddle.
// An entry with no recipe takes the board's default.
// A board that does not read is an error, and the read removes nothing.
func TestClearsMotifRunOfRaddleEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const slug = "some-slug"
	entryOf := func(name, recipe string) string {
		return `{"id":0,"slug":"` + name + `","title":"R","kind":"task","labels":["bug"],"recipe":"` + recipe + `"}`
	}
	tests := []struct {
		name string
		// board is the entry list of board.json, or the whole file when rawBoard is set.
		board         string
		rawBoard      bool
		defaultRecipe string
		childRecipe   string
		want          bool
		wantErr       bool
	}{
		{name: "ASlugTheBoardLacksClearsNothing", board: entryOf("other-slug", shedrun.RecipeRaddle), childRecipe: shedrun.RecipeMotif},
		{name: "ABoardThatDoesNotReadIsAnError", board: "{not json", rawBoard: true, childRecipe: shedrun.RecipeMotif, wantErr: true},
		{name: "ADarnEntryWithADoneMotifRunClearsNothing", board: entryOf(slug, shedrun.RecipeDarn), childRecipe: shedrun.RecipeMotif},
		{name: "ARaddleEntryWhoseDoneRunRanRaddleClearsNothing", board: entryOf(slug, shedrun.RecipeRaddle), childRecipe: shedrun.RecipeRaddle},
		{name: "ARaddleEntryWhoseDoneRunRanMotifIsCleared", board: entryOf(slug, shedrun.RecipeRaddle), childRecipe: shedrun.RecipeMotif, want: true},
		{name: "AnEntryWithNoRecipeUnderARaddleDefaultIsCleared", board: entryOf(slug, ""), defaultRecipe: shedrun.RecipeRaddle, childRecipe: shedrun.RecipeMotif, want: true},
		{name: "AnEntryWithNoRecipeUnderTheTemplateDefaultClearsNothing", board: entryOf(slug, ""), childRecipe: shedrun.RecipeMotif},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			board := tt.board
			if !tt.rawBoard {
				board = `{"version":1,"entries":[` + board + `]}`
			}
			writeBoardHub(t, location, tt.defaultRecipe, board)
			statusPath := shedrun.StatusFile(location, slug)
			if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
				t.Fatalf("create the run directory: %v", err)
			}
			if err := os.WriteFile(statusPath, []byte(`{"state":"done"}`), 0o644); err != nil {
				t.Fatalf("write the run status: %v", err)
			}
			if err := shedrun.WriteChildRecipe(location, slug, tt.childRecipe); err != nil {
				t.Fatalf("write the child recipe: %v", err)
			}

			got, err := clearsMotifRunOfRaddleEntry(location, slug)

			if (err != nil) != tt.wantErr {
				t.Fatalf("clearsMotifRunOfRaddleEntry() error = %v; want an error = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("clearsMotifRunOfRaddleEntry() = %v; want %v", got, tt.want)
			}
			if _, statErr := os.Stat(statusPath); statErr != nil {
				t.Errorf("run status after the read: %v; want the run left in place", statErr)
			}
		})
	}
}

// Over fixture entries and stores, raddleStartRefusal refuses an entry with no stored record.
// It refuses a slug held as another Sketch's, or as its own Sketch under another created stamp.
// A filed slug is refused too; an entry that matches none of these passes.
func TestRaddleStartRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "some-slug"
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	other := created.Add(time.Hour)
	tests := []struct {
		name         string
		recordStored bool
		// stores maps a Sketch slug to its store.json record.
		stores map[string]boardengine.RaddleStoreEntry
		// wantErrors are substrings the refusal carries; none means the entry passes.
		wantErrors []string
	}{
		{
			name:       "NoStoredRecordRefusedNamingTheMotifUpsertAndTheRaddlePromote",
			wantErrors: []string{"holds none", `lym board upsert '{"slug":"some-slug","recipe":"motif"}'`, `lym board promote '{"slug":"some-slug"}'`},
		},
		{
			name:         "SlugFiledByAnotherSketchRefusedNamingTheRename",
			recordStored: true,
			stores:       map[string]boardengine.RaddleStoreEntry{"older-sketch": {Created: created, Filed: []string{"older-sketch", slug}}},
			wantErrors:   []string{"older-sketch", "lym board rename some-slug some-slug-2"},
		},
		{
			name:         "SlugAsItsOwnSketchUnderAnotherCreatedStampRefusedNamingTheRename",
			recordStored: true,
			stores:       map[string]boardengine.RaddleStoreEntry{slug: {Created: other, Filed: []string{slug}}},
			wantErrors:   []string{"lym board rename some-slug some-slug-2"},
		},
		{
			name:         "RenameTargetSkipsASlugTheStoreHolds",
			recordStored: true,
			stores: map[string]boardengine.RaddleStoreEntry{
				"older-sketch": {Created: created, Filed: []string{slug, slug + "-2"}},
			},
			wantErrors: []string{"lym board rename some-slug some-slug-3"},
		},
		{
			name:         "SlugAsItsOwnSketchUnderItsOwnStampPasses",
			recordStored: true,
			stores:       map[string]boardengine.RaddleStoreEntry{slug: {Created: created, Filed: []string{slug}}},
		},
		{
			name:         "StoreNotHoldingTheSlugPasses",
			recordStored: true,
			stores:       map[string]boardengine.RaddleStoreEntry{"unrelated": {Created: created, Filed: []string{"unrelated"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			boardDir := t.TempDir()
			for sketch, record := range tt.stores {
				directory := boardengine.RaddleStoreDir(boardDir, sketch)
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatalf("create store directory: %v", err)
				}
				data, err := json.Marshal(record)
				if err != nil {
					t.Fatalf("encode store record: %v", err)
				}
				if err := os.WriteFile(filepath.Join(directory, "store.json"), data, 0o644); err != nil {
					t.Fatalf("write store record: %v", err)
				}
			}
			store, err := boardengine.ReadRaddleStore(boardDir)
			if err != nil {
				t.Fatalf("ReadRaddleStore: %v", err)
			}
			entry := boardengine.Task{Slug: slug, Created: created}

			got := raddleStartRefusal(entry, tt.recordStored, store)

			if len(tt.wantErrors) == 0 {
				if got != nil {
					t.Fatalf("raddleStartRefusal = %v; want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("raddleStartRefusal = nil; want a refusal carrying %q", tt.wantErrors)
			}
			for _, want := range tt.wantErrors {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("raddleStartRefusal = %q; want it to carry %q", got.Error(), want)
				}
			}
		})
	}
}

// A cancel marker read at the seed write refuses the write with the cancel refusal.
// The seed is not written and no process is registered; an absent marker seeds as before.
func TestArmSeed_CancelMarkerRefusesTheWrite(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "some-slug"
	tests := []struct {
		name       string
		marker     cancelMarkerReading
		wantErr    string
		wantSeeded bool
	}{
		{name: "PresentMarkerRefusesAndWritesNoSeed", marker: cancelMarkerReading{Present: true, WriterAlive: true, Marker: cancelMarker{PID: 4242}}, wantErr: "a cancel of some-slug is under way"},
		{name: "AbsentMarkerSeedsAsBefore", wantSeeded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
			c := &battenCLI{slug: slug, guard: startGuard{Marker: func() (cancelMarkerReading, error) { return tt.marker, nil }}}

			err := c.armSeed(location, slug, "run")

			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("armSeed(run) = %v; want a refusal carrying %q", err, tt.wantErr)
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("armSeed(run) = %v; want nil", err)
			}
			if _, found, readErr := shedrun.ReadSeed(location, slug); readErr != nil || found != tt.wantSeeded {
				t.Errorf("ReadSeed = (found %v, err %v); want found %v", found, readErr, tt.wantSeeded)
			}
			records := listRunRecords(t, location, slug)
			if tt.wantSeeded && (len(records) != 1 || records[0].Role != runproc.RoleBatten) {
				t.Errorf("run records = %+v; want one batten record", records)
			}
			if !tt.wantSeeded && len(records) != 0 {
				t.Errorf("run records = %+v; want none after the refusal", records)
			}
		})
	}
}
