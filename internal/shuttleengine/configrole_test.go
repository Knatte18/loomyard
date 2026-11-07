// configrole_test.go verifies that Start resolves a spec's config role into its skill list before anything is created.

package shuttleengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStart_ConfigRoleResolvesSkills(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	agents, err := agentconfig.Load(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("agentconfig.Load() error = %v", err)
	}
	withAgents := fastConfig
	withAgents.Agents = agents
	tests := []struct {
		name string
		cfg  Config
		// wantErr is a substring of the refusal; every row's engine has no skill loader, so a resolved list is refused too.
		wantErr string
	}{
		{"RoleListLeadsTheSpecSkills", withAgents, "[scribe:prose scribe:code-quality scribe:testing scribe:golang-testing]"},
		{"RoleWithoutListIsRefused", fastConfig, `role "darn" has no skill list`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}, StatusQueue: liveStrandStatus(true)}
			engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "s"}, StartupScript: []StartupState{StartupReady}}
			fx := newFixture(t, reed, engine, withConfig(tt.cfg))
			spec := Spec{
				Prompt:      "x",
				OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")},
				ConfigRole:  agentconfig.RoleDarn,
				Skills:      []string{"scribe:testing", "scribe:golang-testing"},
			}
			_, err := fx.Runner.Start(spec)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Start() error = %v; want one containing %q", err, tt.wantErr)
			}
			if len(reed.AddStrandCalls) != 0 {
				t.Errorf("AddStrand calls = %d; want none before the refusal", len(reed.AddStrandCalls))
			}
		})
	}
}
