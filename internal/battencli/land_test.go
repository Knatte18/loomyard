// land_test.go covers the Land row's pure wiring helper: which board tasks count as ready.

package battencli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestReadySlugs asserts readySlugs lists, sorted, only the unclaimed tasks with no dependency, and skips a note, a claimed task, a done task and a task with a dependency.
func TestReadySlugs(t *testing.T) {
	t.Parallel()

	claimed, done := "running", "done"
	tasks := []boardengine.Task{
		{Slug: "zeta", Kind: boardengine.KindTask},
		{Slug: "alpha", Kind: boardengine.KindTask, DependsOn: []string{}},
		{Slug: "a-note", Kind: boardengine.KindNote},
		{Slug: "claimed", Kind: boardengine.KindTask, Status: &claimed},
		{Slug: "finished", Kind: boardengine.KindTask, Status: &done},
		{Slug: "blocked", Kind: boardengine.KindTask, DependsOn: []string{"alpha"}},
	}

	if got, want := readySlugs(tasks), []string{"alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("readySlugs() = %v; want %v", got, want)
	}
}
