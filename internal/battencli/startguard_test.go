//go:build tier0

// startguard_test.go pins the pre-run guard's refusals over fake seams, and the seed write's re-read of the cancel marker.

package battencli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestStartGuard_Refuse drives startGuard.refuse over fake seams: which verbs the cancel marker refuses, when a dirty source refuses, which left pair parts refuse a start, and that a failing probe refuses naming the re-run.
func TestStartGuard_Refuse(t *testing.T) {
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
		// project and beam are what the Project seam reads; marked and unreadable describe the PairMark seam.
		project, beam, marked, unreadable bool
		projectErr                        error
		// draft is what the Draft seam reads: a task whose board status is draft.
		draft    bool
		draftErr error
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
			// A released task reads a nil status, so the Draft seam reads false.
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
			name:      "DraftTaskRefusesAFreshStartNamingTheRelease",
			verbs:     []string{"run", "step"},
			draft:     true,
			wantExact: "some-slug is a draft task, which no run starts; way forward: run `lym board set-status '{\"slug\":\"some-slug\",\"status\":null}'` to release it, then `lym batten {verb} some-slug`",
		},
		{
			name:     "DraftTaskWithARunStatusFileResumes",
			verbs:    []string{"run", "step"},
			draft:    true,
			found:    true,
			producer: battenrecipe.NameRunShed,
		},
		{
			name:       "DraftProbeErrorRefusesNamingTheProbeAndTheRerun",
			verbs:      []string{"run", "step"},
			draftErr:   errors.New("board unreadable"),
			wantErrors: []string{"the board entry's status", "board unreadable", `run "lym batten {verb} some-slug" again`},
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
			parts: fabricengine.PairParts{TaskBranch: true, Launchers: true},
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
			name:    "BeamProjectWhoseStatusIsDraftPassesWhereADraftTaskIsRefused",
			verbs:   []string{"run", "step"},
			project: true,
			beam:    true,
			draft:   true,
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
						Project:     func() (bool, bool, error) { return tt.project, tt.beam, tt.projectErr },
						PairMark:    func() (bool, bool, error) { return tt.marked, !tt.unreadable, nil },
						Draft:       func() (bool, error) { return tt.draft, tt.draftErr },
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

// TestArmSeed_CancelMarkerRefusesTheWrite asserts a cancel marker read at the seed write refuses it with the cancel refusal, writes no seed and registers no process, and that an absent marker seeds as before.
func TestArmSeed_CancelMarkerRefusesTheWrite(t *testing.T) {
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
