// discussion_test.go — untagged Tier-1 unit tests shared by the loom producers' prompts and the twill fixtures DiscussionTable's tests use.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
)

// writeBurlerYAML plants content as the burler.yaml of layout's anchor, creating its _lyx/config/ directory.
func writeBurlerYAML(t *testing.T, layout *lyxcwd.Location, content string) {
	t.Helper()
	path := configengine.ConfigFile(layout.AnchorPath(), "burler")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// lensNames returns the names of lenses, in order.
func lensNames(lenses []burlerengine.Lens) []string {
	names := make([]string, len(lenses))
	for i, lens := range lenses {
		names[i] = lens.Name
	}
	return names
}

// TestProducerSpecs_SkillsAndParentDirective verifies each loom producer carries its role's skills in order, names the told parent in its prompt, and renders the no-parent variant for an empty name.
// The Discussion-Write rows read the lead's skills and prompt through seatengine.Prompts under a geometry naming the parent.
// The operator-ban line is absent only for the interactive Discussion.
func TestProducerSpecs_SkillsAndParentDirective(t *testing.T) {
	t.Parallel()

	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", Plan: "opus[effort=high]"}
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	// leadOf returns the discussion lead's skills and prompt for a run that is interactive or not.
	leadOf := func(interactive bool, parent string) ([]string, string, error) {
		stencilsDir := newTestStencilsDir(t)
		discussionCfg := cfg
		discussionCfg.DiscussionInteractive = interactive
		table, err := DiscussionTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, discussionCfg, reg, "add-json-flag")
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

	const operatorBan = "A question to the operator in your pane is never the way forward."
	build := map[string]func(parent string) ([]string, string, error){
		"discussion-interactive": func(parent string) ([]string, string, error) { return leadOf(true, parent) },
		"discussion-autonomous":  func(parent string) ([]string, string, error) { return leadOf(false, parent) },
		"plan": func(parent string) ([]string, string, error) {
			spec, err := PlanSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), newTestSpecsDir(t), parent, cfg, reg)
			return spec.Skills, spec.Prompt, err
		},
		"rework": func(parent string) ([]string, string, error) {
			spec, err := ReworkSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), parent, cfg, reg, 4, "prior")
			return spec.Skills, spec.Prompt, err
		},
	}
	wantSkills := map[string][]string{
		"discussion-interactive": {"scribe:prose", "scribe:conversation"},
		"discussion-autonomous":  {"scribe:prose", "scribe:conversation"},
		"plan":                   {"scribe:prose", "scribe:testing"},
		"rework":                 {"scribe:prose", "scribe:testing"},
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
