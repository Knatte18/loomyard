// dependency_test.go — when a depends_on dependency is met (dependency.go), for every kind of entry that lands.

package boardengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDependenciesPending pins which dependencies hold a task back.
// An entry holds a task back until it is done, and a beam until it landed.
// A plain project holds it until it is marked done or every landing member is done.
// A stored membership loop holds a task back and ends the walk.
// Such a loop covers two projects naming each other, or one naming itself.
func TestDependenciesPending(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	done := stringPtr("done")
	entries := []boardengine.Task{
		{Slug: "open-task", Kind: boardengine.KindTask},
		{Slug: "done-task", Kind: boardengine.KindTask, Status: done},
		{Slug: "beam", Kind: boardengine.KindProject, Beam: true, Status: stringPtr(boardengine.RunStatus("running", "Beam-Await"))},
		{Slug: "beam-member", Kind: boardengine.KindTask, Project: "beam", Status: done},
		{Slug: "landed-beam", Kind: boardengine.KindProject, Beam: true, Status: done},
		{Slug: "finished", Kind: boardengine.KindProject},
		{Slug: "finished-member", Kind: boardengine.KindTask, Project: "finished", Status: done},
		{Slug: "finished-note", Kind: boardengine.KindNote, Project: "finished"},
		{Slug: "unfinished", Kind: boardengine.KindProject},
		{Slug: "unfinished-done", Kind: boardengine.KindTask, Project: "unfinished", Status: done},
		{Slug: "unfinished-open", Kind: boardengine.KindTask, Project: "unfinished"},
		{Slug: "empty", Kind: boardengine.KindProject},
		{Slug: "marked", Kind: boardengine.KindProject, Status: done},
		{Slug: "outer", Kind: boardengine.KindProject},
		{Slug: "inner-beam", Kind: boardengine.KindProject, Beam: true, Project: "outer"},
		{Slug: "inner-beam-member", Kind: boardengine.KindTask, Project: "inner-beam", Status: done},
		{Slug: "loop-a", Kind: boardengine.KindProject, Project: "loop-b"},
		{Slug: "loop-b", Kind: boardengine.KindProject, Project: "loop-a"},
		{Slug: "self-member", Kind: boardengine.KindProject, Project: "self-member"},
	}
	tests := []struct {
		name        string
		dep         string
		wantPending bool
	}{
		{"OpenTask", "open-task", true},
		{"DoneTask", "done-task", false},
		{"PrunedEntry", "pruned", false},
		{"BeamWhoseMembersLandedWhileItHasNot", "beam", true},
		{"LandedBeam", "landed-beam", false},
		{"PlainProjectWhoseEveryLandingMemberIsDone", "finished", false},
		{"PlainProjectWithAnOpenMember", "unfinished", true},
		{"PlainProjectWithNoMember", "empty", true},
		{"PlainProjectMarkedDone", "marked", false},
		{"PlainProjectHoldingABeamThatHasNotLanded", "outer", true},
		{"ProjectsNamingEachOtherAsTheirProject", "loop-a", true},
		{"ProjectNamingItself", "self-member", true},
	}
	dependencies := boardengine.NewDependencies(entries)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var want []string
			if tt.wantPending {
				want = []string{tt.dep}
			}
			waiter := boardengine.Task{Slug: "waiter", Kind: boardengine.KindTask, DependsOn: []string{tt.dep}}
			if got := dependencies.Pending(waiter); !slices.Equal(got, want) {
				t.Errorf("Pending(waiter on %s) = %v; want %v", tt.dep, got, want)
			}
		})
	}
}
