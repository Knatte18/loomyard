// bench_test.go pins which slugs read as a bench pair of a task.

package darnengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestIsBenchPairSlug(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"FirstBenchPair", "task-b1", true},
		{"MultiDigitNumber", "task-b12", true},
		{"TaskSlugAlone", "task", false},
		{"NoNumber", "task-b", false},
		{"ZeroNumber", "task-b0", false},
		{"LeadingZero", "task-b01", false},
		{"WordAfterTheInfix", "task-bench", false},
		{"AnotherTaskSharingThePrefix", "task-notes", false},
		{"BenchPairOfALongerSlug", "task-x-b1", false},
		{"BenchPairOfAnotherTask", "other-b1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := darnengine.IsBenchPairSlug("task", tt.slug); got != tt.want {
				t.Errorf("IsBenchPairSlug(%q, %q) = %v; want %v", "task", tt.slug, got, tt.want)
			}
		})
	}
}
