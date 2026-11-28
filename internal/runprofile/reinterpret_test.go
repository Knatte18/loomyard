// reinterpret_test.go tests the one-time rewrites of a hub's profiles file over files held as strings.

package runprofile_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReinterpretJacquardBlocks pins the one-time carry of a hub's tapestry values into its jacquard blocks and default.
// The shipped template comes back unchanged.
// A tuned or operator-added tapestry block yields a jacquard block without loom.discussion,
// and the jacquard default follows the tapestry one or is dropped.
func TestReinterpretJacquardBlocks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		existing string
		// wantJacquard maps a profile name to its jacquard block after the rewrite, nil for none.
		wantJacquard map[string]any
		// wantDefault is defaults.jacquard after the rewrite, empty for none.
		wantDefault string
		// wantRewrote and wantFlagged hold the opening of each line, in order.
		wantRewrote, wantFlagged []string
		wantKept                 []string
		// wantUnchanged asks for the file back byte for byte.
		wantUnchanged bool
	}{
		{
			name: "a shipped profile's tuned tapestry block carries into its jacquard block",
			existing: "defaults: {tapestry: solid, jacquard: solid}\n" +
				"solid:\n  tapestry:\n    loom: {discussion: {max_bounces: 2}, code: {max_bounces: 4}}\n    tapestry: {plan: {max_bounces: 5}}\n" +
				"  jacquard:\n    loom: {code: {max_bounces: 1}}\n    tapestry: {plan: {max_bounces: 2}}\n",
			wantJacquard: map[string]any{"solid": map[string]any{
				"loom":     map[string]any{"code": map[string]any{"max_bounces": 4}},
				"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 5}},
			}},
			wantDefault: "solid",
			wantRewrote: []string{"profiles.solid.jacquard: set to the profile's tapestry block"},
		},
		{
			name:         "an operator-added profile gains a jacquard block and comments stay",
			existing:     "# my profiles\nbroad:\n  tapestry:\n    loom: {discussion: {max_bounces: 1}} # one round\n    drift: {scope: \"off\"}\n",
			wantJacquard: map[string]any{"broad": map[string]any{"drift": map[string]any{"scope": "off"}}},
			wantRewrote:  []string{"profiles.broad.jacquard: set to the profile's tapestry block"},
			wantKept:     []string{"# my profiles", "# one round"},
		},
		{
			name: "a changed tapestry default carries into the jacquard default",
			existing: "defaults: {tapestry: light, jacquard: solid}\n" +
				"light:\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\n  jacquard:\n    tapestry: {plan: {max_bounces: 1}}\n" +
				"solid:\n  tapestry:\n    tapestry: {plan: {max_bounces: 2}}\n  jacquard:\n    tapestry: {plan: {max_bounces: 2}}\n",
			wantJacquard: map[string]any{
				"light": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 1}}},
				"solid": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 2}}},
			},
			wantDefault: "light",
			wantRewrote: []string{"profiles.defaults.jacquard: set to light"},
		},
		{
			name: "a jacquard default is dropped when tapestry has none",
			existing: "defaults: {darn: docs, jacquard: solid}\n" +
				"solid:\n  tapestry:\n    tapestry: {plan: {max_bounces: 2}}\n  jacquard:\n    tapestry: {plan: {max_bounces: 2}}\n" +
				"docs:\n  darn:\n    darn: {writer: opus}\n",
			wantJacquard: map[string]any{
				"solid": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 2}}},
				"docs":  nil,
			},
			wantRewrote: []string{"profiles.defaults.jacquard: dropped"},
		},
		{
			name:     "an aliased profile gains its block in the mapping it aliases",
			existing: "base: &b\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\nquick: *b\n",
			wantJacquard: map[string]any{
				"base":  map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 1}}},
				"quick": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 1}}},
			},
			wantRewrote: []string{"profiles.base.jacquard: set", "profiles.quick.jacquard: set"},
		},
		{
			name: "a profile supplied through a merge key is flagged and the default naming it dropped",
			existing: "defaults: {tapestry: quick, jacquard: quick}\n" +
				"<<: {quick: {tapestry: {tapestry: {plan: {max_bounces: 1}}}}}\n",
			wantJacquard: map[string]any{"quick": nil},
			wantRewrote:  []string{"profiles.defaults.jacquard: dropped"},
			wantFlagged:  []string{"profiles.quick.jacquard: profile quick holds a tapestry block but is supplied through a merge key"},
		},
		{
			name:         "a defaults entry supplied through a merge key is flagged when the default would be set",
			existing:     "<<: {defaults: {tapestry: solid}}\nsolid:\n  tapestry:\n    tapestry: {plan: {max_bounces: 2}}\n",
			wantJacquard: map[string]any{"solid": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 2}}}},
			wantRewrote:  []string{"profiles.solid.jacquard: set"},
			wantFlagged:  []string{"profiles.defaults.jacquard: the default is supplied through a merge key"},
		},
		{
			name:          "a defaults entry supplied through a merge key is flagged when the default would be dropped",
			existing:      "<<: {defaults: {jacquard: solid}}\nsolid:\n  darn:\n    darn: {writer: opus}\n",
			wantDefault:   "solid",
			wantFlagged:   []string{"profiles.defaults.jacquard: the default is supplied through a merge key"},
			wantUnchanged: true,
		},
		{
			name:          "a default held as an own key and through a merge key is flagged, not half dropped",
			existing:      "defaults: {<<: {jacquard: solid}, jacquard: solid}\nsolid:\n  darn:\n    darn: {writer: opus}\n",
			wantDefault:   "solid",
			wantFlagged:   []string{"profiles.defaults.jacquard: the default is supplied through a merge key"},
			wantUnchanged: true,
		},
		{
			name:        "an own default beside a merge key supplying other keys is dropped",
			existing:    "defaults: {<<: {darn: docs}, jacquard: solid}\ndocs:\n  darn:\n    darn: {writer: opus}\n",
			wantRewrote: []string{"profiles.defaults.jacquard: dropped"},
		},
		{
			name:          "a jacquard block on a profile holding no tapestry block is left as it is",
			existing:      "solo:\n  jacquard:\n    tapestry: {plan: {max_bounces: 1}}\n",
			wantJacquard:  map[string]any{"solo": map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": 1}}}},
			wantUnchanged: true,
		},
		{
			name: "an anchored jacquard block that differs is flagged, so its aliases keep their anchor",
			existing: "solid:\n  tapestry:\n    tapestry: {plan: {max_bounces: 5}}\n  jacquard: &j\n    tapestry: {plan: {max_bounces: 2}}\n" +
				"wide:\n  tapestry:\n    tapestry: {plan: {max_bounces: 2}}\n  jacquard: *j\n",
			wantFlagged:   []string{"profiles.solid.jacquard: profile solid's jacquard block carries a YAML anchor"},
			wantUnchanged: true,
		},
		{
			name: "an anchored default that would be set is flagged",
			existing: "defaults: {tapestry: light, jacquard: &d solid, darn: *d}\n" +
				"light:\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\n  jacquard:\n    tapestry: {plan: {max_bounces: 1}}\n",
			wantDefault:   "solid",
			wantFlagged:   []string{"profiles.defaults.jacquard: the default carries a YAML anchor"},
			wantUnchanged: true,
		},
		{
			name:          "an anchored default that would be dropped is flagged",
			existing:      "defaults: {jacquard: &d docs, darn: *d}\ndocs:\n  darn:\n    darn: {writer: opus}\n",
			wantDefault:   "docs",
			wantFlagged:   []string{"profiles.defaults.jacquard: the default carries a YAML anchor"},
			wantUnchanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := runprofile.ReinterpretJacquardBlocks([]byte(tt.existing), nil)
			if err != nil {
				t.Fatalf("ReinterpretJacquardBlocks() error = %v; want none", err)
			}
			if !slices.EqualFunc(rewrote, tt.wantRewrote, strings.HasPrefix) {
				t.Errorf("rewrote = %q; want lines opening %q", rewrote, tt.wantRewrote)
			}
			if !slices.EqualFunc(flagged, tt.wantFlagged, strings.HasPrefix) {
				t.Errorf("flagged = %q; want lines opening %q", flagged, tt.wantFlagged)
			}
			var entries map[string]map[string]any
			if err := yaml.Unmarshal(got, &entries); err != nil {
				t.Fatalf("parse the rewritten file: %v\n%s", err, got)
			}
			for profile, want := range tt.wantJacquard {
				if block := entries[profile]["jacquard"]; !reflect.DeepEqual(block, want) {
					t.Errorf("%s.jacquard = %v; want %v\n%s", profile, block, want, got)
				}
			}
			if gotDefault, _ := entries["defaults"]["jacquard"].(string); gotDefault != tt.wantDefault {
				t.Errorf("defaults.jacquard = %q; want %q\n%s", gotDefault, tt.wantDefault, got)
			}
			if tt.wantUnchanged && string(got) != tt.existing {
				t.Errorf("output = %q; want the file unchanged", got)
			}
			for _, kept := range tt.wantKept {
				if !strings.Contains(string(got), kept) {
					t.Errorf("output = %q; want it to keep %q", got, kept)
				}
			}
		})
	}
}

// TestReinterpretArrasDefault_HalfMergedDefaultIsFlagged pins the flag on an arras default a merge key also supplies.
// The file is left as it is, since dropping the own key alone would not drop the default.
func TestReinterpretArrasDefault_HalfMergedDefaultIsFlagged(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	existing := "defaults: {<<: {arras: docs}, arras: docs}\ndocs:\n  darn:\n    darn: {writer: opus}\n"
	got, rewrote, flagged, err := runprofile.ReinterpretArrasDefault([]byte(existing), nil)
	if err != nil || len(rewrote) != 0 {
		t.Fatalf("ReinterpretArrasDefault() rewrote %q, error %v; want none", rewrote, err)
	}
	want := []string{"defaults.arras: docs names a profile with no arras block"}
	if !slices.EqualFunc(flagged, want, strings.HasPrefix) {
		t.Errorf("flagged = %q; want lines opening %q", flagged, want)
	}
	if string(got) != existing {
		t.Errorf("output = %q; want the file unchanged", got)
	}
}

// TestReinterpretJacquardBlocks_ShippedTemplateUnchanged pins the shipped template as a fixed point of the rewrite.
// Its jacquard blocks and default are what the rewrite derives.
// A hub left as shipped therefore comes back byte for byte, with no line.
func TestReinterpretJacquardBlocks_ShippedTemplateUnchanged(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	template := runprofile.ConfigTemplate()
	got, rewrote, flagged, err := runprofile.ReinterpretJacquardBlocks([]byte(template), nil)
	if err != nil || len(rewrote) != 0 || len(flagged) != 0 {
		t.Fatalf("ReinterpretJacquardBlocks(template) rewrote %q, flagged %q, error %v; want none", rewrote, flagged, err)
	}
	if string(got) != template {
		t.Errorf("ReinterpretJacquardBlocks(template) = %q; want the template unchanged", got)
	}
}
