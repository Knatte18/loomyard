// subagents_test.go verifies that Prepare takes a missing effort from agents.yaml.
// It also verifies that Prepare sets the subagent fallback effort in the settings.
// It also verifies that Prepare tells the session what its subagents get.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// templateAgentsConfig returns a shuttle config whose Agents holds the shipped agents.yaml template.
func templateAgentsConfig(t *testing.T) shuttleengine.Config {
	t.Helper()
	agents, err := agentconfig.Load(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("agentconfig.Load() error = %v", err)
	}
	return shuttleengine.Config{Agents: agents}
}

func TestPrepare_EffortFromAgentsConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		spec shuttleengine.Spec
		want string
	}{
		{"NoModel_Default", shuttleengine.Spec{ConfigRole: agentconfig.RoleDarn}, "--effort 'medium'"},
		{"ModelAlias", shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "fable"}}, "--effort 'high'"},
		{"FullModelID", shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "claude-fable-5-1"}}, "--effort 'high'"},
		{"SpecEffortWins", shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "fable", Effort: "low"}}, "--effort 'low'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.spec.Prompt = "do the thing"
			launch, err := engineWith(t, templateConfig(t)).Prepare(t.TempDir(), tt.spec, templateAgentsConfig(t))
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			for name, line := range map[string]string{"Cmd": launch.Cmd, "ResumeCmd": launch.ResumeCmd} {
				if !strings.Contains(line, tt.want) {
					t.Errorf("Launch.%s = %q; want it to contain %q", name, line, tt.want)
				}
			}
		})
	}
}

func TestPrepare_TellsTheSessionWhatItsSubagentsGet(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		skills      []string
		wantPassage []string
		wantAbsent  string
	}{
		{
			name:        "WithSkills",
			skills:      []string{"scribe:prose", "scribe:golang-comments"},
			wantPassage: []string{"`high` on fable, `medium` on any other model", "Your skills are scribe:prose, scribe:golang-comments."},
		},
		{
			name:        "WithoutSkills",
			wantPassage: []string{"`high` on fable, `medium` on any other model"},
			wantAbsent:  "Your skills are",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "do the thing", ConfigRole: agentconfig.RoleDarn, Skills: tt.skills}
			if _, err := engineWith(t, templateConfig(t)).Prepare(runDir, spec, templateAgentsConfig(t)); err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			prompt, err := os.ReadFile(filepath.Join(runDir, appendPromptFileName))
			if err != nil {
				t.Fatalf("read the appended prompt: %v", err)
			}
			for _, want := range tt.wantPassage {
				if !strings.Contains(string(prompt), want) {
					t.Errorf("appended prompt = %q; want it to contain %q", prompt, want)
				}
			}
			if tt.wantAbsent != "" && strings.Contains(string(prompt), tt.wantAbsent) {
				t.Errorf("appended prompt = %q; want no %q", prompt, tt.wantAbsent)
			}
			settings, err := os.ReadFile(filepath.Join(runDir, settingsFileName))
			if err != nil {
				t.Fatalf("read the settings: %v", err)
			}
			if want := `"env":{"CLAUDE_CODE_SUBAGENT_EFFORT":"medium"}`; !strings.Contains(string(settings), want) {
				t.Errorf("settings = %s; want them to contain %s", settings, want)
			}
		})
	}
}

func TestPrepare_NoAgentsConfigWritesNoSubagentRule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Skills: []string{"scribe:prose"}}
	launch, err := engineWith(t, templateConfig(t)).Prepare(runDir, spec, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if strings.Contains(launch.Cmd, "--effort") {
		t.Errorf("Launch.Cmd = %q; want no --effort", launch.Cmd)
	}
	settings, err := os.ReadFile(filepath.Join(runDir, settingsFileName))
	if err != nil {
		t.Fatalf("read the settings: %v", err)
	}
	if strings.Contains(string(settings), subagentEffortEnvKey) {
		t.Errorf("settings = %s; want no %s", settings, subagentEffortEnvKey)
	}
}
