// endsubject_test.go pins which registered runs the end of a subject is handed.

package pairteardown

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSubjectRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owners := boardengine.Owners{Entries: map[string]boardengine.OwnerEntry{
		"beam":       {IsProject: true, Beam: true},
		"beam-task":  {IsTask: true},
		"beam-other": {IsTask: true},
		"other":      {IsProject: true, Beam: true},
		"other-task": {IsTask: true},
		"root-task":  {IsTask: true},
		"broken":     {IsTask: true},
	}}
	parents := map[string]string{"beam-task": "beam", "beam-other": "beam", "other-task": "other", "root-task": ""}
	parentOf := func(slug string) (string, error) {
		if slug == "broken" {
			return "", errors.New("pair unreadable")
		}
		return parents[slug], nil
	}
	registered := []string{"root-task", "other-task", "other", "ghost", "broken", "beam-task", "beam-other", "beam"}
	tests := []struct {
		name  string
		owner string
		want  []string
	}{
		{"ABeamTakesItsOwnTasksNeverItsOwnRecordNorAnUnreadableOne", "beam", []string{"beam-other", "beam-task"}},
		{"AnotherBeamTakesItsOwnTasksOnly", "other", []string{"other-task"}},
		{"TheHubsRootTakesTheRootTasksOnly", runproc.HubOwner, []string{"root-task"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := subjectRuns(owners, registered, tt.owner, parentOf); !slices.Equal(got, tt.want) {
				t.Errorf("subjectRuns(%q) = %v; want %v", tt.owner, got, tt.want)
			}
		})
	}
}
