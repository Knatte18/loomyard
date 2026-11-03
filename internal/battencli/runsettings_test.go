// runsettings_test.go table-drives Seed-Child's prepare step against a profiles.yaml under t.TempDir.

package battencli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrepareRunSettings pins how Seed-Child's prepare step lays a task's run settings over its run profile's block for the task's recipe, or the recipe's default profile when the task names none.
// It also pins that an unknown profile, a profile without the recipe's block, a profiles file that does not decode, and a refused key each route to Stuck through ErrInvalidRunSettings.
func TestPrepareRunSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const profiles = "defaults: {darn: light}\n" +
		"light:\n  tapestry:\n    loom: {plan_max_bounces: 1, code_max_bounces: 1, merge_max_bounces: 1}\n  darn:\n    loom: {code_max_bounces: 2}\n" +
		"docs:\n  darn:\n    darn: {writer: opus}\n"
	tests := []struct {
		name        string
		profilesYML string
		settings    battenshed.RunSettings
		wantDocs    map[string]string
		wantStuck   []string
		// like, when set, replaces wantDocs with the documents a task of these settings gets over the same profiles file.
		like *battenshed.RunSettings
	}{
		{
			name:     "an arras task naming no profile takes the template's tapestry default profile's tapestry block",
			settings: battenshed.RunSettings{Recipe: "arras"},
			like:     &battenshed.RunSettings{Recipe: "tapestry"},
		},
		{
			name:        "an arras task naming light takes light's tapestry block",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "arras", Profile: "light"},
			like:        &battenshed.RunSettings{Recipe: "tapestry", Profile: "light"},
		},
		{
			name:        "a task whose recipe names no embedded recipe and no profile writes the task's settings alone",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "no-such-recipe", Settings: map[string]any{"darn": map[string]any{"verify_attempts": float64(1)}}},
			wantDocs:    map[string]string{"darn": "verify_attempts: 1\n"},
		},
		{
			name:        "no profile and no default for the recipe writes the task's settings alone",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Settings: map[string]any{"darn": map[string]any{"verify_attempts": float64(1)}}},
			wantDocs:    map[string]string{"darn": "verify_attempts: 1\n"},
		},
		{
			name:        "no profile takes the recipe's default profile",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "darn"},
			wantDocs:    map[string]string{"loom": "code_max_bounces: 2\n"},
		},
		{
			name:     "no profiles file takes the template's default for the recipe",
			settings: battenshed.RunSettings{Recipe: "darn"},
			wantDocs: map[string]string{"loom": "code_hard_max_bounces: 2\ncode_max_bounces: 1\ncode_reviewer_fan: \"\"\n"},
		},
		{
			name:     "a sampler task naming the template's light profile writes its loom values and the review on",
			settings: battenshed.RunSettings{Recipe: "sampler", Profile: "light"},
			wantDocs: map[string]string{"loom": "code_hard_max_bounces: 2\ncode_max_bounces: 1\ncode_reviewer_fan: \"\"\n", "sampler": "code_review: true\n"},
		},
		{
			name:        "the task's settings lie over the tapestry block with its plan keys in the tapestry document, and the profile's retired merge_max_bounces is dropped",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "light", Settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}}},
			wantDocs:    map[string]string{"loom": "code_max_bounces: 1\n", "tapestry": "plan_max_bounces: 3\n"},
		},
		{
			name:        "the same profile writes its darn block for a darn task",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "darn", Profile: "light"},
			wantDocs:    map[string]string{"loom": "code_max_bounces: 2\n"},
		},
		{
			name:        "an unknown profile is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "heavy"},
			wantStuck:   []string{`"heavy"`, "known: docs, light"},
		},
		{
			name:        "a profile without the recipe's block is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "docs"},
			wantStuck:   []string{`"docs"`, "no tapestry block"},
		},
		{
			name:        "a profiles file that does not decode is Stuck",
			profilesYML: "light: 3\n",
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "light"},
			wantStuck:   []string{`profile "light"`},
		},
		{
			name:        "a profiles file that does not decode is Stuck for a task naming no profile",
			profilesYML: "light: 3\n",
			settings:    battenshed.RunSettings{Recipe: "tapestry"},
			wantStuck:   []string{`profile "light"`},
		},
		{
			name:        "a refused task key is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "light", Settings: map[string]any{"loom": map[string]any{"no_such_key": float64(1)}}},
			wantStuck:   []string{"loom", "no_such_key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir()}
			if tt.profilesYML != "" {
				if err := os.MkdirAll(configengine.ConfigDir(dirs.Board), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dirs.Board, "profiles"), []byte(tt.profilesYML), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := prepareRunSettings(dirs, tt.settings)
			if tt.wantStuck != nil {
				if !errors.Is(err, battenshed.ErrInvalidRunSettings) {
					t.Fatalf("prepareRunSettings() error = %v; want one wrapping ErrInvalidRunSettings", err)
				}
				for _, want := range tt.wantStuck {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("prepareRunSettings() error = %q; want it to name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("prepareRunSettings() error = %v; want none", err)
			}
			wantDocs := tt.wantDocs
			if tt.like != nil {
				reference, err := prepareRunSettings(dirs, *tt.like)
				if err != nil || len(reference) == 0 {
					t.Fatalf("prepareRunSettings(%+v) = %v, %v; want documents and no error", *tt.like, reference, err)
				}
				wantDocs = map[string]string{}
				for module, document := range reference {
					wantDocs[module] = string(document)
				}
			}
			if len(got) != len(wantDocs) {
				t.Fatalf("prepareRunSettings() = %v; want %d modules", got, len(wantDocs))
			}
			for module, want := range wantDocs {
				if string(got[module]) != want {
					t.Errorf("document for %s = %q; want %q", module, got[module], want)
				}
			}
		})
	}
}
