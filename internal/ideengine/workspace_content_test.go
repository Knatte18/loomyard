// workspace_content_test.go covers the members ideengine adds to a workspace file.

package ideengine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorkspaceContent is not parallel: it swaps the package-level LymBinary seam.
func TestWorkspaceContent(t *testing.T) {
	tiergate.Require(t, 0)
	original := LymBinary
	t.Cleanup(func() { LymBinary = original })

	resolved := func() (string, error) { return "/opt/lym", nil }
	unresolved := func() (string, error) { return "", errors.New("no lym binary") }
	tests := []struct {
		name        string
		lymBinary   func() (string, error)
		beam        string
		wantCommand string
		wantArgs    []string
	}{
		{"HubFileStartsTheRootProject", resolved, "", "/opt/lym", []string{"beam", "start", "root"}},
		{"BeamFileStartsItsBeam", resolved, "my-beam", "/opt/lym", []string{"beam", "start", "my-beam"}},
		{"UnresolvedBinaryFallsBackToBareName", unresolved, "my-beam", "lym", []string{"beam", "start", "my-beam"}},
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
			if err := json.Unmarshal(members["tasks"], &got); err != nil {
				t.Fatalf("WorkspaceContent tasks member is not JSON: %v\n%s", err, members["tasks"])
			}
			if len(members) != 1 || len(got.Tasks) != 1 {
				t.Fatalf("WorkspaceContent = %s; want only a tasks member holding one task", members)
			}
			task := got.Tasks[0]
			if task.Command != tt.wantCommand || !slices.Equal(task.Args, tt.wantArgs) || task.Options.Cwd != "/hub/anchor" {
				t.Errorf("task = %+v; want command %q, args %v and cwd /hub/anchor", task, tt.wantCommand, tt.wantArgs)
			}
		})
	}
}
