// config_test.go verifies Load's resolution and refusals for agents.yaml, and the skill and effort answers of a Config.
// It seeds its fixtures with os.MkdirAll and os.WriteFile under t.TempDir, so the tests spawn no process.

package agentconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// loadHub writes content as the hub's agents.yaml under a fresh board dir, unless it is empty, and loads it.
func loadHub(t *testing.T, content string) (agentconfig.Config, error) {
	t.Helper()
	board := t.TempDir()
	if content != "" {
		path := configengine.ConfigFile(board, agentconfig.ModuleName)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return agentconfig.Load(configengine.Dirs{Board: board})
}

func TestLoad_ResolvesSkillsPerRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		role    agentconfig.Role
		want    []string
	}{
		{"AbsentFile_TemplateList", "", agentconfig.RoleDarn, []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}},
		{"AbsentFile_EmptyList", "", agentconfig.RoleTackler, []string{}},
		{"HubFile_OverridesRole", "skills:\n  darn: [scribe:x]\n", agentconfig.RoleDarn, []string{"scribe:x"}},
		{"HubFile_OtherRoleKeepsTemplate", "skills:\n  darn: [scribe:x]\n", agentconfig.RoleGateFixer, []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := loadHub(t, tt.content)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			got, err := cfg.Skills(tt.role, nil)
			if err != nil {
				t.Fatalf("Skills(%q) error = %v", tt.role, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Skills(%q) = %v; want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestLoad_TemplateListsEveryRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg, err := loadHub(t, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, role := range agentconfig.Roles() {
		if _, err := cfg.Skills(role, nil); err != nil {
			t.Errorf("Skills(%q) error = %v; want the template's list", role, err)
		}
	}
}

func TestLoad_RefusesUnusableContent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		wantKey string
	}{
		{"UnknownRole", "skills:\n  nobody: []\n", ""},
		{"NullRole", "skills:\n  darn: null\n", ""},
		{"NonStringEntry", "skills:\n  darn: [[a]]\n", ""},
		{"EmptyEntry", "skills:\n  darn: [\"\"]\n", ""},
		{"PaddedEntry", "skills:\n  darn: [\" scribe:x\"]\n", ""},
		{"NoDefaultEffort", "efforts:\n  haiku: low\n", ""},
		{"EmptyEffort", "efforts:\n  default: medium\n  haiku: \"\"\n", ""},
		{"UnknownEffort", "efforts:\n  default: medium\n  haiku: turbo\n", "efforts.haiku"},
		{"PairWithUnknownRole", "efforts:\n  default: medium\n  nobody/haiku: low\n", ""},
		{"PairWithoutModel", "efforts:\n  default: medium\n  darn/: low\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadHub(t, tt.content)
			if !errors.Is(err, agentconfig.ErrUnusable) || !errors.Is(err, configengine.ErrInvalid) {
				t.Errorf("Load(%q) error = %v; want one wrapping ErrUnusable and configengine.ErrInvalid", tt.content, err)
				return
			}
			if tt.wantKey != "" && !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("Load(%q) error = %v; want it to name %q", tt.content, err, tt.wantKey)
			}
		})
	}
}

func TestConfig_SkillsAppendsExtraWithoutRepeats(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg, err := loadHub(t, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got, err := cfg.Skills(agentconfig.RoleDarn, []string{"scribe:testing", "scribe:golang-comments"})
	if err != nil {
		t.Fatalf("Skills() error = %v", err)
	}
	want := []string{"scribe:prose", "scribe:code-quality", "scribe:testing", "scribe:golang-comments"}
	if !slices.Equal(got, want) {
		t.Errorf("Skills(darn, extra) = %v; want %v", got, want)
	}
}

func TestConfig_SkillsRefusesRoleWithoutList(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if _, err := (agentconfig.Config{}).Skills(agentconfig.RoleDarn, nil); err == nil {
		t.Errorf("Config{}.Skills(darn) error = nil; want a refusal")
	}
}

func TestConfig_EffortTakesTheMostSpecificKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg, err := loadHub(t, "efforts:\n  default: medium\n  haiku: low\n  darn: high\n  darn/haiku: max\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := []struct {
		name  string
		role  agentconfig.Role
		model string
		want  string
	}{
		{"RoleAndModel", agentconfig.RoleDarn, "haiku", "max"},
		{"RoleOverModel", agentconfig.RoleDarn, "opus", "high"},
		{"ModelWithoutRoleEntry", agentconfig.RolePlan, "haiku", "low"},
		{"Default", agentconfig.RolePlan, "opus", "medium"},
		{"NoRole", "", "haiku", "low"},
		{"NoModel", agentconfig.RoleDarn, "", "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := cfg.Effort(tt.role, tt.model); got != tt.want {
				t.Errorf("Effort(%q, %q) = %q; want %q", tt.role, tt.model, got, tt.want)
			}
		})
	}
	if got := (agentconfig.Config{}).Effort(agentconfig.RoleDarn, "haiku"); got != "" {
		t.Errorf("Config{}.Effort() = %q; want empty", got)
	}
}

func TestConfig_ModelEffortsListsTheModelsTheRoleMeets(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg, err := loadHub(t, "efforts:\n  default: medium\n  haiku: low\n  darn: high\n  darn/sonnet: max\n  plan/opus: xhigh\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	gotModels, gotOther := cfg.ModelEfforts(agentconfig.RoleDarn)
	wantModels := []agentconfig.ModelEffort{{Model: "haiku", Effort: "high"}, {Model: "sonnet", Effort: "max"}}
	if !slices.Equal(gotModels, wantModels) || gotOther != "high" {
		t.Errorf("ModelEfforts(darn) = %v, %q; want %v, %q", gotModels, gotOther, wantModels, "high")
	}
}
