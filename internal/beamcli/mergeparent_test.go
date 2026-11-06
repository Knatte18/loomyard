// mergeparent_test.go drives the merge-parent sequence over the land test's fake hub: the entries that are not a beam, a beam with no parent branch or an open merge task, and the darn task filed with its perch on under the first free slug.

package beamcli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMergeParent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	done := boardengine.StatusDone
	longBeam := strings.Repeat("x", boardengine.MaxSlugLength-2)
	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a filing, else every substring the refusal names.
		wantErr []string
		// wantTask is the filed task's slug, and wantRounds and wantHard the Code-Review caps its run settings carry.
		wantTask   string
		wantRounds int
		wantHard   int
	}{
		{name: "TaskRefusedNamingItsOwnLanding", slug: "task1", wantErr: []string{"lym beam merge-parent:", "lym batten run task1"}},
		{name: "GroupingProjectRefusedNamingCreate", slug: "group", wantErr: []string{"grouping project", "lym beam create group"}},
		{name: "DoneBeamRefused", slug: "landed", wantErr: []string{"already landed"}},
		{
			name: "BeamWithNoParentBranchRefusedNamingReconcile", slug: "b",
			setup:   func(w *landWorld) { w.origins["b"] = fabricengine.Origin{Beam: true} },
			wantErr: []string{"names no parent branch", "lym fabric reconcile"},
		},
		{
			name: "OpenMergeTaskRefused", slug: "b",
			setup: func(w *landWorld) {
				w.tasks = append(w.tasks, boardengine.Task{Slug: "b-merge-in-2", Kind: boardengine.KindTask, Project: "b"})
			},
			wantErr: []string{"b-merge-in-2 stands open", "lym batten cancel b-merge-in-2 --remove"},
		},
		{name: "OffPerchTurnedOnForOneRound", slug: "b", wantTask: "b-merge-in", wantRounds: 1, wantHard: 2},
		{
			name: "HubBudgetKept", slug: "b",
			setup:    func(w *landWorld) { w.caps = reviewbudget.Caps{Soft: 3, Hard: 5} },
			wantTask: "b-merge-in", wantRounds: 3, wantHard: 5,
		},
		{
			name: "DoneMergeTaskTakesTheNextNumber", slug: "b",
			setup: func(w *landWorld) {
				w.tasks = append(w.tasks, boardengine.Task{Slug: "b-merge-in", Kind: boardengine.KindTask, Project: "b", Status: &done})
			},
			wantTask: "b-merge-in-2", wantRounds: 1, wantHard: 2,
		},
		{
			name: "LongBeamSlugCutToTheBoardsLength", slug: longBeam,
			setup: func(w *landWorld) {
				w.owners.Entries[longBeam] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
				w.pairs[longBeam] = true
				w.origins[longBeam] = fabricengine.Origin{Beam: true, ParentBranch: "main"}
			},
			wantTask: longBeam[:boardengine.MaxSlugLength-len(darnengine.MergeTaskSuffix)] + darnengine.MergeTaskSuffix, wantRounds: 1, wantHard: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := mergeParent(w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("mergeParent(%q) = %v, nil; want a refusal naming %v", tt.slug, got, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("mergeParent(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if len(w.filed) != 0 {
					t.Errorf("mergeParent(%q) filed %v; want nothing filed on a refusal", tt.slug, w.filed)
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeParent(%q) = %v; want the merge task filed", tt.slug, err)
			}
			if len(w.filed) != 1 {
				t.Fatalf("mergeParent(%q) filed %d tasks; want 1", tt.slug, len(w.filed))
			}
			fields := w.filed[0]
			if len(tt.wantTask) > boardengine.MaxSlugLength {
				t.Fatalf("want slug %q is longer than the board's slug length", tt.wantTask)
			}
			wantSettings := map[string]any{"loom": map[string]any{"code_max_bounces": tt.wantRounds, "code_hard_max_bounces": tt.wantHard}}
			if fields["slug"] != tt.wantTask || fields["kind"] != boardengine.KindTask || fields["project"] != tt.slug || fields["recipe"] != "darn" || !reflect.DeepEqual(fields["run_settings"], wantSettings) {
				t.Errorf("mergeParent(%q) filed %v; want slug %q, a darn task in project %q with run settings %v", tt.slug, fields, tt.wantTask, tt.slug, wantSettings)
			}
			if fields["pull_request"] != boardengine.PullRequestMergeIn {
				t.Errorf("mergeParent(%q) pull_request = %v; want %q", tt.slug, fields["pull_request"], boardengine.PullRequestMergeIn)
			}
			if body, _ := fields["body"].(string); !strings.Contains(body, "lym fabric merge-in main") || !strings.Contains(body, "reviews the merge alone") {
				t.Errorf("mergeParent(%q) body = %q; want it to name `lym fabric merge-in main` and the perch's review of the merge alone", tt.slug, body)
			}
			if got["task"] != tt.wantTask || got["parent_branch"] != "main" || !strings.Contains(got["way_forward"].(string), "lym batten status "+tt.wantTask) {
				t.Errorf("mergeParent(%q) = %v; want task %q, parent_branch main and a way forward naming its batten status", tt.slug, got, tt.wantTask)
			}
		})
	}
}

func TestMergeParentCarriesTheBeamsLabels(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	w := newLandWorld()

	if _, err := mergeParent(w.deps(), "b"); err != nil {
		t.Fatalf("mergeParent(b) = %v; want the merge task filed", err)
	}

	if got, want := w.filed[0]["labels"], []string{"enhancement", "reed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mergeParent(b) labels = %v; want the beam's %v", got, want)
	}
}
