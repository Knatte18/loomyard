// discussion_test.go — untagged Tier-1 unit tests for the loom producers' prompts and the shared twill fixtures.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeLensesYAML plants content as the lenses.yaml of layout's anchor, creating its _lym/config/ directory.
func writeLensesYAML(t *testing.T, layout *lymcwd.Location, content string) {
	t.Helper()
	path := configengine.ConfigFile(layout.AnchorPath(), "lenses")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// lensNames returns the names of lenses, in order.
func lensNames(resolved []lenses.Lens) []string {
	names := make([]string, len(resolved))
	for i, lens := range resolved {
		names[i] = lens.Name
	}
	return names
}

// TestProducerSpecs_SkillsAndParentDirective verifies each loom producer carries its role's skills in order.
// It checks that the prompt names the told parent, and that an empty parent renders the no-parent variant.
// Every row reads the lead's skills and prompt through seatengine.Prompts, under a geometry that names the parent.
// The operator-ban line is absent only for the interactive Discussion.
func TestProducerSpecs_SkillsAndParentDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lymcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", Plan: "opus[effort=high]", SubagentModel: "sonnet"}
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	// leadOfTable builds a table over a seeded stencils directory and returns the lead's skills and prompt.
	// The prompt is composed under a geometry that names parent.
	leadOfTable := func(build func(stencilsDir string) (seatengine.Table, error), parent string) ([]string, string, error) {
		stencilsDir := newTestStencilsDir(t)
		table, err := build(stencilsDir)
		if err != nil {
			return nil, "", err
		}
		prompts, err := seatengine.Prompts(seatengine.Geometry{
			WorktreeRoot: layout.WorktreePath(),
			AnchorPath:   layout.AnchorPath(),
			StencilsDir:  stencilsDir,
			ParentName:   parent,
			Shortname:    "ly",
			Slug:         "add-json-flag",
		}, table)
		if err != nil {
			return nil, "", err
		}
		return table.Chair().Skills, prompts[""], nil
	}
	// leadOf returns the discussion lead's skills and prompt for a run that is interactive or not.
	leadOf := func(interactive bool, parent string) ([]string, string, error) {
		discussionCfg := cfg
		discussionCfg.DiscussionInteractive = interactive
		return leadOfTable(func(stencilsDir string) (seatengine.Table, error) {
			return DiscussionTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), discussionCfg, reg, "add-json-flag", false)
		}, parent)
	}

	const operatorBan = "never the way forward"
	build := map[string]func(parent string) ([]string, string, error){
		"discussion-interactive": func(parent string) ([]string, string, error) { return leadOf(true, parent) },
		"discussion-autonomous":  func(parent string) ([]string, string, error) { return leadOf(false, parent) },
		"plan": func(parent string) ([]string, string, error) {
			return leadOfTable(func(stencilsDir string) (seatengine.Table, error) {
				return PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), cfg, reg, "none", false)
			}, parent)
		},
	}
	wantSkills := map[string][]string{
		"discussion-interactive": {"scribe:prose", "scribe:conversation"},
		"discussion-autonomous":  {"scribe:prose", "scribe:conversation"},
		"plan":                   {"scribe:prose", "scribe:testing"},
	}

	for name, mk := range build {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			skills, prompt, err := mk("shortname:orch")
			if err != nil {
				t.Fatalf("prompt with parent = _, %v; want nil error", err)
			}
			if !reflect.DeepEqual(skills, wantSkills[name]) {
				t.Errorf("Skills = %v; want %v", skills, wantSkills[name])
			}
			if !strings.Contains(prompt, "`shortname:orch`") {
				t.Errorf("Prompt does not name the told parent %q", "shortname:orch")
			}
			wantBan := name != "discussion-interactive"
			if got := strings.Contains(prompt, operatorBan); got != wantBan {
				t.Errorf("Prompt carries the operator-ban line = %v; want %v", got, wantBan)
			}

			_, noParent, err := mk("")
			if err != nil {
				t.Fatalf("prompt without parent = _, %v; want nil error", err)
			}
			if !strings.Contains(noParent, "No parent is recorded for this run.") {
				t.Error("Prompt does not carry the no-parent variant for an empty parent name")
			}
			if strings.Contains(noParent, "{{") {
				t.Error("Prompt contains an unrendered marker")
			}
		})
	}
}
