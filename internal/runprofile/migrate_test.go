// migrate_test.go table-drives the profiles reconcile hooks, MigrateConfig and LackingTemplateBlocks, and the Seed-Child lookups' refusal of a recipe held under its former name.

package runprofile_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMigrateConfig pins the rename of a recipe's former name in each profile and under defaults, keeping values and comments, the drop of a former block equal to the current one and the keeping of a differing one, an untouched document with no former name, and the refusal of a document that does not parse.
func TestMigrateConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name         string
		existing     string
		wantLines    []string
		wantContains []string
		// wantProfiles is the light profile Load reads from the migrated document, and wantDefaults its defaults; nil skips the load.
		wantProfiles map[string]map[string]any
		wantDefaults map[string]string
		wantRefusal  bool
	}{
		{
			name:     "a hub seeded before the rename gets tapestry blocks and defaults with the operator's values",
			existing: "# operator note\ndefaults:\n  loom: light\n  darn: light\nlight:\n  loom:\n    loom: {plan_max_bounces: 7}\n  darn:\n    loom: {code_max_bounces: 2}\n",
			wantLines: []string{
				"defaults.loom: renamed to defaults.tapestry",
				"light.loom: renamed to light.tapestry",
			},
			wantContains: []string{"# operator note", "loom: {plan_max_bounces: 7}"},
			wantProfiles: map[string]map[string]any{
				"tapestry": {"loom": map[string]any{"plan_max_bounces": 7}},
				"darn":     {"loom": map[string]any{"code_max_bounces": 2}},
			},
			wantDefaults: map[string]string{"tapestry": "light", "darn": "light"},
		},
		{
			name:      "a former block equal to the current one is dropped and the current one kept",
			existing:  "light:\n  loom:\n    loom: {plan_max_bounces: 1}\n  tapestry:\n    loom: {plan_max_bounces: 1}\n",
			wantLines: []string{"light.loom: removed, light.tapestry already set"},
			wantProfiles: map[string]map[string]any{
				"tapestry": {"loom": map[string]any{"plan_max_bounces": 1}},
			},
			wantDefaults: map[string]string{},
		},
		{
			name:         "a former block differing from the current one is left in place with the document unchanged",
			existing:     "light:\n  loom:\n    loom: {plan_max_bounces: 7}\n  tapestry:\n    loom: {plan_max_bounces: 1}\n",
			wantContains: []string{"light:\n  loom:\n    loom: {plan_max_bounces: 7}\n  tapestry:\n    loom: {plan_max_bounces: 1}\n"},
		},
		{
			name:         "a document with no former name comes back unchanged",
			existing:     "# kept as written\nlight:\n  darn:\n    loom:   {code_max_bounces: 2}\n",
			wantContains: []string{"# kept as written\nlight:\n  darn:\n    loom:   {code_max_bounces: 2}\n"},
		},
		{name: "an empty document comes back unchanged", existing: ""},
		{name: "a document that does not parse is refused", existing: "light: [\n", wantRefusal: true},
		{name: "a document whose root is not a mapping is refused", existing: "- a\n", wantRefusal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := runprofile.MigrateConfig([]byte(tt.existing))
			if tt.wantRefusal {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("MigrateConfig() error = %v; want one marked invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("MigrateConfig() error = %v; want none", err)
			}
			if !slices.Equal(lines, tt.wantLines) {
				t.Errorf("MigrateConfig() lines = %q; want %q", lines, tt.wantLines)
			}
			if tt.wantLines == nil && string(got) != tt.existing {
				t.Errorf("MigrateConfig() = %q; want the document unchanged", got)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(string(got), want) {
					t.Errorf("MigrateConfig() = %q; want it to contain %q", got, want)
				}
			}
			if tt.wantProfiles == nil {
				return
			}
			profiles, err := runprofile.Load(configengine.Dirs{Board: boardWithProfiles(t, string(got))})
			if err != nil {
				t.Fatalf("Load(migrated) error = %v; want none", err)
			}
			if light := map[string]map[string]any(profiles.ByName["light"]); !reflect.DeepEqual(light, tt.wantProfiles) {
				t.Errorf("Load(migrated) light = %v; want %v", light, tt.wantProfiles)
			}
			if !reflect.DeepEqual(profiles.Defaults, tt.wantDefaults) {
				t.Errorf("Load(migrated) defaults = %v; want %v", profiles.Defaults, tt.wantDefaults)
			}
		})
	}
}

// TestLackingTemplateBlocks pins the report of the template's recipe blocks and defaults entries a hub's entry of the same name lacks and the former blocks held beside a differing current one, and that an entry the template lacks or the hub removed whole lists nothing.
func TestLackingTemplateBlocks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		existing string
		want     []string
	}{
		{name: "the template itself lacks nothing", existing: runprofile.ConfigTemplate()},
		{
			name:     "a starting profile seeded before motif and sampler lists their blocks",
			existing: "light:\n  tapestry:\n    loom: {plan_max_bounces: 1}\n  darn:\n    loom: {code_max_bounces: 1}\n  pawl:\n    loom: {code_max_bounces: 1}\n",
			want:     []string{"light.motif", "light.sampler"},
		},
		{
			name:     "defaults lacking a template recipe list it",
			existing: "defaults: {darn: docs}\ndocs:\n  darn:\n    darn: {writer: opus}\n",
			want:     []string{"defaults.motif", "defaults.pawl", "defaults.tapestry"},
		},
		{
			name:     "a former block differing from the current one is listed and an equal one is not",
			existing: "light:\n  loom:\n    loom: {plan_max_bounces: 7}\n  tapestry:\n    loom: {plan_max_bounces: 1}\n  darn:\n    loom: {code_max_bounces: 1}\n  pawl:\n    loom: {code_max_bounces: 1}\n  sampler:\n    loom: {code_max_bounces: 1}\nsolid:\n  loom:\n    loom: {plan_max_bounces: 2}\n  tapestry:\n    loom: {plan_max_bounces: 2}\n  darn:\n    loom: {code_max_bounces: 1}\n  pawl:\n    loom: {code_max_bounces: 1}\n  sampler:\n    loom: {code_max_bounces: 1}\n",
			want:     []string{"light.loom: differs from light.tapestry", "light.motif", "solid.motif"},
		},
		{name: "a profile the template lacks lists nothing", existing: "broad:\n  darn:\n    loom: {code_max_bounces: 2}\n"},
		{name: "an empty document lists nothing", existing: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := runprofile.LackingTemplateBlocks([]byte(tt.existing))
			if err != nil {
				t.Fatalf("LackingTemplateBlocks() error = %v; want none", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("LackingTemplateBlocks() = %q; want %q", got, tt.want)
			}
		})
	}
	if _, err := runprofile.LackingTemplateBlocks([]byte("light: [\n")); !errors.Is(err, configengine.ErrInvalid) {
		t.Errorf("LackingTemplateBlocks(unparseable) error = %v; want one marked invalid", err)
	}
}

// TestFormerRecipeNameRefusal pins that Settings and Default refuse a recipe a profile or the defaults hold only under its former name, naming the reconcile that renames it.
func TestFormerRecipeNameRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{Board: boardWithProfiles(t, "defaults: {loom: light}\nlight:\n  loom:\n    loom: {plan_max_bounces: 1}\n")})
	if err != nil {
		t.Fatal(err)
	}
	const wayForward = `lym config reconcile --layer default --apply`

	_, err = profiles.Settings("light", "tapestry")
	for _, want := range []string{`"light"`, "no tapestry block but a loom block", wayForward} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Settings(light, tapestry) error = %v; want it to name %q", err, want)
		}
	}

	name, err := profiles.Default("tapestry")
	for _, want := range []string{"no tapestry entry but a loom entry", wayForward} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Default(tapestry) = %q, %v; want an error naming %q", name, err, want)
		}
	}
	if name, err := profiles.Default("darn"); name != "" || err != nil {
		t.Errorf("Default(darn) = %q, %v; want empty and no error for a recipe with no default under any name", name, err)
	}
}
