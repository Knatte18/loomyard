// workspacecontent_test.go covers the workspace-content hook that adds the beam start task to workspace files.

package ideengine

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorkspaceContent is not parallel: it swaps the package's LymBinary seam and restores it.
func TestWorkspaceContent(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name        string
		resolve     func() (string, error)
		beam        string
		wantCommand string
		wantBeam    string
	}{
		{"EmptyBeamRunsForRoot", func() (string, error) { return "/opt/lym", nil }, "", "/opt/lym", "root"},
		{"BeamSlugRunsForThatBeam", func() (string, error) { return "/opt/lym", nil }, "some-beam", "/opt/lym", "some-beam"},
		{"UnresolvedBinaryFallsBackToBareLym", func() (string, error) { return "", errors.New("no binary") }, "some-beam", "lym", "some-beam"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := LymBinary
			t.Cleanup(func() { LymBinary = previous })
			LymBinary = tt.resolve

			members := WorkspaceContent("/hub", tt.beam, "/hub/anchor")

			if len(members) != 1 {
				t.Fatalf("members = %v; want only tasks", members)
			}
			var got struct {
				Tasks []map[string]any `json:"tasks"`
			}
			if err := json.Unmarshal(members["tasks"], &got); err != nil {
				t.Fatalf("tasks member is not valid JSON: %v\n%s", err, members["tasks"])
			}
			if len(got.Tasks) != 1 {
				t.Fatalf("tasks = %v; want one task", got.Tasks)
			}
			task := got.Tasks[0]
			if task["command"] != tt.wantCommand ||
				!reflect.DeepEqual(task["args"], []any{"beam", "start", tt.wantBeam}) ||
				!reflect.DeepEqual(task["options"], map[string]any{"cwd": "/hub/anchor"}) {
				t.Errorf("task = %v; want command %q, args [beam start %s], cwd /hub/anchor", task, tt.wantCommand, tt.wantBeam)
			}
		})
	}
}
