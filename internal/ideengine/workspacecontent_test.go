// workspacecontent_test.go covers the members a workspace file is handed.

package ideengine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorkspaceContent pins which beam the workspace's task starts, where it runs and which command runs it.
// It is not parallel, since it swaps the package's LymBinary seam.
func TestWorkspaceContent(t *testing.T) {
	tiergate.Require(t, 0)
	original := LymBinary
	t.Cleanup(func() { LymBinary = original })

	resolved := func() (string, error) { return "/opt/lym/bin/lym", nil }
	failing := func() (string, error) { return "", errors.New("no lym binary") }
	tests := []struct {
		name        string
		lymBinary   func() (string, error)
		beam        string
		wantCommand string
		wantArgs    []string
	}{
		{"HubFileStartsTheRoot", resolved, "", "/opt/lym/bin/lym", []string{"beam", "start", "root"}},
		{"BeamFileStartsItsBeam", resolved, "my-beam", "/opt/lym/bin/lym", []string{"beam", "start", "my-beam"}},
		{"UnresolvedBinaryRunsTheBareName", failing, "my-beam", "lym", []string{"beam", "start", "my-beam"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			LymBinary = tt.lymBinary

			members := WorkspaceContent("/hub", tt.beam, "/hub/anchor")

			var got struct {
				Tasks []struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
					Options struct {
						Cwd string `json:"cwd"`
					} `json:"options"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(members["tasks"], &got); err != nil || len(members) != 1 || len(got.Tasks) != 1 {
				t.Fatalf("WorkspaceContent(%q) = %s (decode err %v); want one tasks member holding one task", tt.beam, members["tasks"], err)
			}
			task := got.Tasks[0]
			if task.Command != tt.wantCommand || !slices.Equal(task.Args, tt.wantArgs) || task.Options.Cwd != "/hub/anchor" {
				t.Errorf("WorkspaceContent(%q) task = %+v; want command %q, args %v in /hub/anchor", tt.beam, task, tt.wantCommand, tt.wantArgs)
			}
		})
	}
}
