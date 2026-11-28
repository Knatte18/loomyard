// workspacecontent_test.go covers the members WorkspaceContent hands a workspace file.

package ideengine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorkspaceContent pins the beam, anchor and lym command of the beam start task a workspace file gets.
// It is not parallel: it swaps the package-level LymBinary seam.
func TestWorkspaceContent(t *testing.T) {
	tiergate.Require(t, 0)
	original := LymBinary
	t.Cleanup(func() { LymBinary = original })
	resolved := func() (string, error) { return "/opt/lym/bin/lym", nil }
	unresolved := func() (string, error) { return "", errors.New("no lym") }

	tests := []struct {
		name        string
		lymBinary   func() (string, error)
		beam        string
		wantBeam    string
		wantCommand string
	}{
		{"HubFileStartsTheRootProject", resolved, "", "root", "/opt/lym/bin/lym"},
		{"BeamFileStartsItsBeam", resolved, "b", "b", "/opt/lym/bin/lym"},
		{"UnresolvedBinaryIsBareLym", unresolved, "b", "b", "lym"},
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
				t.Fatalf("WorkspaceContent(%q) = %s, decode error %v; want one tasks member with one task", tt.beam, members, err)
			}
			task := got.Tasks[0]
			if want := []string{"beam", "start", tt.wantBeam}; !slices.Equal(task.Args, want) || task.Command != tt.wantCommand || task.Options.Cwd != "/hub/anchor" {
				t.Errorf("WorkspaceContent(%q) task = %+v; want command %q, args %v in /hub/anchor", tt.beam, task, tt.wantCommand, want)
			}
		})
	}
}
