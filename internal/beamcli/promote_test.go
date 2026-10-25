//go:build tier0

// promote_test.go drives the promote sequence over fake deps: each refusal with its text and no step run, the steps in order for a running task with the dobby start last, a re-run finishing the rest after a stop at each step, and a project made a beam by create's steps.

package beamcli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// promoteWorld is the fake hub of createWorld with the seams promote adds.
type promoteWorld struct {
	*createWorld
	tasks        []boardengine.Task
	parentBranch map[string]string
	endErr       error
	resetCommits bool
	// anchors maps a slug to the own and parent anchors Anchors answers; a slug it lacks answers equal ones.
	anchors    map[string][2]string
	anchorsErr error
}

// newPromoteWorld returns a hub with a running task "run1" owned by here, its pair present, and the fixtures of newCreateWorld.
func newPromoteWorld() *promoteWorld {
	w := &promoteWorld{createWorld: newCreateWorld(), parentBranch: map[string]string{}, resetCommits: true, anchors: map[string][2]string{}}
	w.owners.Entries["run1"] = boardengine.OwnerEntry{Owner: here, IsTask: true, RunHeld: true}
	w.pairs["run1"] = true
	return w
}

func (w *promoteWorld) deps() promoteDeps {
	return promoteDeps{
		createDeps: w.createWorld.deps(),
		ReadTasks:  func() ([]boardengine.Task, error) { return w.tasks, nil },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			parent, set := w.parentBranch[slug]
			if !set {
				parent = "main"
			}
			return fabricengine.Origin{ParentBranch: parent, Beam: w.marked[slug]}, w.pairs[slug], nil
		},
		EndRun: func(context.Context, string) error {
			w.calls = append(w.calls, "end")
			return w.endErr
		},
		ClearRun: func(context.Context, string) error {
			w.calls = append(w.calls, "clear")
			return nil
		},
		ResetRecords: func(*fabricengine.Mutations, string) (fabricengine.ResetRecordsResult, error) {
			w.calls = append(w.calls, "reset")
			return fabricengine.ResetRecordsResult{Committed: w.resetCommits}, nil
		},
		Anchors: func(slug string) (string, string, error) {
			if pair, set := w.anchors[slug]; set {
				return pair[0], pair[1], w.anchorsErr
			}
			return ".", ".", w.anchorsErr
		},
	}
}

func TestPromote(t *testing.T) {
	t.Parallel()

	open := "active"
	tests := []struct {
		name  string
		slug  string
		setup func(w *promoteWorld)
		// wantErr is empty for a promote that finishes, else every substring the refusal names.
		wantErr   []string
		wantCalls []string
	}{
		{
			name: "NeverStartedTaskRefusedNamingANewProjectAndItsCreate",
			slug: "fresh",
			setup: func(w *promoteWorld) {
				w.owners.Entries["fresh"] = boardengine.OwnerEntry{IsTask: true}
			},
			wantErr: []string{"never started", "lyx board upsert", "lyx beam create"},
		},
		{name: "AbsentEntryRefused", slug: "ghost", wantErr: []string{"not on the board", "lyx board list"}},
		{name: "NoteRefused", slug: "note1", wantErr: []string{"note", "lyx board upsert"}},
		{
			name: "TaskAnotherMachineOwnsRefusedNamingAdopt",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.owners.Entries["run1"] = boardengine.OwnerEntry{Owner: there, IsTask: true, RunHeld: true}
			},
			wantErr: []string{there, "lyx adopt run1"},
		},
		{
			name: "StartedTaskWithNoPairRefusedNamingAdopt",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.pairs["run1"] = false
			},
			wantErr: []string{"no pair on this machine", "lyx adopt run1"},
		},
		{
			name: "TaskAnOpenTaskDependsOnRefusedNamingEachDependent",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.tasks = []boardengine.Task{
					{Slug: "dep-b", Kind: boardengine.KindTask, DependsOn: []string{"run1"}, Status: &open},
					{Slug: "dep-a", Kind: boardengine.KindTask, DependsOn: []string{"run1"}},
				}
			},
			wantErr: []string{"dep-a, dep-b", "lyx board set-deps"},
		},
		{
			name: "RootPairRefusedNamingANewProjectAndItsCreate",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.parentBranch["run1"] = ""
			},
			wantErr: []string{"root pair", "lyx board upsert", "lyx beam create"},
		},
		{
			name: "RunPastTheParentMergeRefusedBeforeAnyLaterStep",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.endErr = errors.New(`the work of run1 reached its parent; way forward: run "lyx batten run run1" to finish the landing`)
			},
			wantErr:   []string{"reached its parent", "lyx batten run run1"},
			wantCalls: []string{"end"},
		},
		{
			name: "EndOnlyFirstHalfStopsAfterStepOneNamingTheRerun",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.endErr = fmt.Errorf("%w; way forward: run \"lyx beam promote run1\" again once origin answers", battencli.ErrPromoteEndOnly)
			},
			wantErr:   []string{`"lyx beam promote run1" again once origin answers`},
			wantCalls: []string{"end"},
		},
		{
			name:      "RunningTaskRunsTheSevenStepsInOrderAndStartsTheDobbyLast",
			slug:      "run1",
			wantCalls: []string{"end", "mark", "push", "clear", "reset", "push", "owner", "workspace", "dobby"},
		},
		{
			name: "TaskAtAnotherAnchorThanItsParentsRecordsRefusedAfterTheRunIsEndedAndBeforeTheReset",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.anchors["run1"] = [2]string{".", "sub"}
			},
			wantErr:   []string{"run1 sits at anchor . and its parent's records at sub", "`lyx fabric anchor run1 --set sub`", "`lyx beam promote run1`"},
			wantCalls: []string{"end", "mark", "push", "clear"},
		},
		{
			name: "UnreadableAnchorsRefusedNamingTheSlugAndTheRerun",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.anchorsErr = errors.New("parent records branch is gone")
			},
			wantErr:   []string{"run1", "parent records branch is gone", "run `lyx beam promote run1` again"},
			wantCalls: []string{"end", "mark", "push", "clear"},
		},
		{
			name: "FailedDobbyStartNamesBeamStart",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.dobbyErr = errors.New("spawn failed")
			},
			wantErr:   []string{"spawn failed", "lyx beam start"},
			wantCalls: []string{"end", "mark", "push", "clear", "reset", "push", "owner", "workspace", "dobby"},
		},
		{
			name: "RerunAfterStepTwoFinishesTheRest",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.marked["run1"] = true
			},
			wantCalls: []string{"clear", "reset", "push", "owner", "workspace", "dobby"},
		},
		{
			name: "RerunAfterStepThreeFinishesTheRest",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.marked["run1"] = true
				w.owners.Entries["run1"] = boardengine.OwnerEntry{Owner: here, IsTask: true}
			},
			wantCalls: []string{"reset", "push", "owner", "workspace", "dobby"},
		},
		{
			name: "RerunAfterStepFourSpawnsNoPushForAnUnchangedReset",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.marked["run1"] = true
				w.owners.Entries["run1"] = boardengine.OwnerEntry{Owner: here, IsTask: true}
				w.resetCommits = false
			},
			wantCalls: []string{"reset", "owner", "workspace", "dobby"},
		},
		{
			name: "RerunAfterStepFiveFinishesTheWorkspaceAndDobbySteps",
			slug: "run1",
			setup: func(w *promoteWorld) {
				w.marked["run1"] = true
				w.owners.Entries["run1"] = boardengine.OwnerEntry{Owner: here, IsProject: true, Beam: true}
			},
			wantCalls: []string{"workspace", "dobby"},
		},
		{
			name:      "GroupingProjectRunsCreatesSteps",
			slug:      "group",
			wantCalls: []string{"owner", "add", "push", "workspace", "dobby"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newPromoteWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := promote(context.Background(), w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("promote(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("promote(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
			} else if err != nil {
				t.Fatalf("promote(%q) = %v; want it to finish", tt.slug, err)
			}
			if !slices.Equal(w.calls, tt.wantCalls) {
				t.Errorf("promote(%q) ran %v; want %v", tt.slug, w.calls, tt.wantCalls)
			}
			if len(tt.wantErr) == 0 && !strings.Contains(got.WayForward, "lyx orch start") {
				t.Errorf("promote(%q) way forward = %q; want `lyx orch start` in the beam's worktree", tt.slug, got.WayForward)
			}
		})
	}
}
