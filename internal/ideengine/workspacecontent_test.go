// workspacecontent_test.go covers which beam start task WorkspaceContent hands each workspace file.

package ideengine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorkspaceContent is not parallel: it swaps the package-level LymBinary.
func TestWorkspaceContent(t *testing.T) {
	tiergate.Require(t, 0)
	original := LymBinary
	t.Cleanup(func() { LymBinary = original })
	resolved := func() (string, error) { return "/opt/lym/bin/lym", nil }
	unresolved := func() (string, error) { return "/never/used", errors.New("no lym binary") }

	tests := []struct {
		name        string
		lymBinary   func() (string, error)
		beam        string
		wantCommand string
		wantArgs    []string
	}{
		{"HubFileStartsRoot", resolved, "", "/opt/lym/bin/lym", []string{"beam", "start", "root"}},
		{"BeamFileStartsItsBeam", resolved, "b", "/opt/lym/bin/lym", []string{"beam", "start", "b"}},
		{"UnresolvedBinaryRunsBareLym", unresolved, "b", "lym", []string{"beam", "start", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			LymBinary = tt.lymBinary

			members := WorkspaceContent("/hub", tt.beam, "/hub/anchor")

			var tasks struct {
				Tasks []struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
					Options struct {
						Cwd string `json:"cwd"`
					} `json:"options"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(members["tasks"], &tasks); err != nil {
				t.Fatalf("tasks member is not valid JSON: %v\n%s", err, members["tasks"])
			}
			if len(members) != 1 || len(tasks.Tasks) != 1 {
				t.Fatalf("WorkspaceContent(%q) = %s; want the one tasks member holding one task", tt.beam, members)
			}
			task := tasks.Tasks[0]
			if task.Command != tt.wantCommand || !slices.Equal(task.Args, tt.wantArgs) || task.Options.Cwd != "/hub/anchor" {
				t.Errorf("task = %+v; want command %q, args %v, cwd /hub/anchor", task, tt.wantCommand, tt.wantArgs)
			}
		})
	}
}
