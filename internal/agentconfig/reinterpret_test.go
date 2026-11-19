// reinterpret_test.go verifies that DropRetiredRoles carries an agents.yaml across the retired treadle roles.
// It seeds its fixtures under t.TempDir, so the tests spawn no process.

package agentconfig_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDropRetiredRoles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const namesRetired = "efforts:\n  default: medium\n  treadle-judge: low\n  treadle-targeting/opus: high\n  opus: max\n" +
		"skills:\n  darn: [scribe:x]\n  treadle-judge: [scribe:prose]\n  treadle-targeting: [scribe:prose]\n"
	tests := []struct {
		name  string
		input string
		// wantRewrote is the rewrote lines; nil means the file must come back unchanged.
		wantRewrote []string
		// wantInvalid means the document is refused as configengine.ErrInvalid.
		wantInvalid bool
	}{
		{
			name:  "RetiredRolesInSkillsAndEffortsAreRemoved",
			input: namesRetired,
			wantRewrote: []string{
				"efforts.treadle-judge: retired role, removed",
				"efforts.treadle-targeting/opus: retired role, removed",
				"skills.treadle-judge: retired role, removed",
				"skills.treadle-targeting: retired role, removed",
			},
		},
		{name: "FileNamingNoRetiredRoleIsUnchanged", input: "efforts:\n  default: medium\nskills:\n  darn: [scribe:x]\n"},
		{name: "EmptyFileIsUnchanged", input: ""},
		{name: "UnparsableDocumentIsInvalid", input: "skills: [unclosed\n", wantInvalid: true},
		{name: "NonMappingTopLevelIsInvalid", input: "- a\n", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := agentconfig.DropRetiredRoles([]byte(tt.input), "", "")
			if tt.wantInvalid {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("DropRetiredRoles(%q) error = %v; want one wrapping configengine.ErrInvalid", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DropRetiredRoles(%q) error = %v", tt.input, err)
			}
			if len(flagged) != 0 {
				t.Errorf("DropRetiredRoles(%q) flagged %v; want none", tt.input, flagged)
			}
			if !slices.Equal(rewrote, tt.wantRewrote) {
				t.Errorf("DropRetiredRoles(%q) rewrote %v; want %v", tt.input, rewrote, tt.wantRewrote)
			}
			if tt.wantRewrote == nil {
				if !bytes.Equal(got, []byte(tt.input)) {
					t.Errorf("DropRetiredRoles(%q) = %q; want the file unchanged", tt.input, got)
				}
				return
			}

			if _, err := loadHub(t, tt.input); !errors.Is(err, agentconfig.ErrUnusable) {
				t.Errorf("Load of the file before the rewrite error = %v; want it refused as ErrUnusable", err)
			}
			if strings.Contains(string(got), "treadle") {
				t.Errorf("DropRetiredRoles(%q) = %q; want no retired role left", tt.input, got)
			}
			cfg, err := loadHub(t, string(got))
			if err != nil {
				t.Fatalf("Load of the rewritten file error = %v", err)
			}
			if skills, err := cfg.Skills(agentconfig.RoleDarn, nil); err != nil || !slices.Equal(skills, []string{"scribe:x"}) {
				t.Errorf("Skills(darn) = %v, %v; want [scribe:x] kept", skills, err)
			}
			if effort := cfg.Effort(agentconfig.RolePlan, "opus"); effort != "max" {
				t.Errorf("Effort(plan, opus) = %q; want max kept", effort)
			}
			again, rewroteAgain, _, err := agentconfig.DropRetiredRoles(got, "", "")
			if err != nil || len(rewroteAgain) != 0 || !bytes.Equal(again, got) {
				t.Errorf("second DropRetiredRoles = %q, %v, %v; want the file unchanged with no lines", again, rewroteAgain, err)
			}
		})
	}
}
