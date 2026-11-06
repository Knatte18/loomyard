// mergetask_test.go pins which slugs read as a merge task's and which strand names as its writer's.

package darnengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestIsMergeTaskWriter(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		strand agentname.Name
		want   bool
	}{
		{"FirstMergeTask", agentname.Name{Slug: "beam-merge-in", Role: "darn"}, true},
		{"NumberedMergeTask", agentname.Name{Slug: "beam-merge-in-2", Role: "darn"}, true},
		{"NumberedRole", agentname.Name{Slug: "beam-merge-in", Role: "darn-2"}, true},
		{"OtherRoleOfAMergeTask", agentname.Name{Slug: "beam-merge-in", Role: "code-fix"}, false},
		{"WriterOfAnotherTask", agentname.Name{Slug: "beam-task", Role: "darn"}, false},
		{"SuffixInsideTheSlug", agentname.Name{Slug: "beam-merge-in-notes", Role: "darn"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := darnengine.IsMergeTaskWriter(tt.strand); got != tt.want {
				t.Errorf("IsMergeTaskWriter(%+v) = %v; want %v", tt.strand, got, tt.want)
			}
		})
	}
}
