// land_test.go covers the Land row's pure wiring helper: which board tasks count as ready.

package battencli

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestPullStopOutcome asserts a lone records-side stop comes back as its message with a nil error, and a stop joined with a code-side error or any other error comes back as that error with no stop.
func TestPullStopOutcome(t *testing.T) {
	t.Parallel()

	stop := fabricengine.NewRecordsStopError("main", errors.New("replay conflict"), "settle it")
	codeSideErr := errors.New("code side fetch failed")
	joined := errors.Join(stop, codeSideErr)
	unrelated := errors.New("open fabric failed")

	tests := []struct {
		name        string
		err         error
		wantStop    string
		wantPullErr error
	}{
		{"LoneRecordsStop", stop, stop.Error(), nil},
		{"RecordsStopJoinedWithCodeSideError", joined, "", joined},
		{"UnrelatedError", unrelated, "", unrelated},
		{"NoError", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStop, gotErr := pullStopOutcome(tt.err)
			if gotStop != tt.wantStop || gotErr != tt.wantPullErr {
				t.Errorf("pullStopOutcome(%v) = %q, %v; want %q, %v", tt.err, gotStop, gotErr, tt.wantStop, tt.wantPullErr)
			}
		})
	}
}

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
