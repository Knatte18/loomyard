//go:build tier0

// runsettings_test.go table-drives Seed-Child's prepare step against a profiles.yaml under t.TempDir.

package battencli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// TestPrepareRunSettings pins how Seed-Child's prepare step lays a task's run settings over its run profile's block for the task's recipe, or the recipe's default profile when the task names none.
// It also pins that an unknown profile, a profile without the recipe's block, a profiles file that does not decode, and a refused key each route to Stuck through ErrInvalidRunSettings.
func TestPrepareRunSettings(t *testing.T) {
	t.Parallel()
	const profiles = "defaults: {darn: light}\n" +
		"light:\n  loom:\n    loom: {plan_max_bounces: 1, code_max_bounces: 1, merge_max_bounces: 1}\n  darn:\n    loom: {code_max_bounces: 2}\n" +
		"docs:\n  darn:\n    darn: {writer: opus}\n"
	tests := []struct {
		name        string
		profilesYML string
		settings    battenshed.RunSettings
		wantDocs    map[string]string
		wantStuck   []string
	}{
		{
			name:        "no profile and no default for the recipe writes the task's settings alone",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "loom", Settings: map[string]any{"darn": map[string]any{"verify_attempts": float64(1)}}},
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
			wantDocs: map[string]string{"loom": "code_hard_max_bounces: 2\ncode_max_bounces: 1\n"},
		},
		{
			name:        "the task's settings lie over the loom block, and the profile's retired merge_max_bounces is dropped",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "loom", Profile: "light", Settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}}},
			wantDocs:    map[string]string{"loom": "code_max_bounces: 1\nplan_max_bounces: 3\n"},
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
			settings:    battenshed.RunSettings{Recipe: "loom", Profile: "heavy"},
			wantStuck:   []string{`"heavy"`, "known: docs, light"},
		},
		{
			name:        "a profile without the recipe's block is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "loom", Profile: "docs"},
			wantStuck:   []string{`"docs"`, "no loom block"},
		},
		{
			name:        "a profiles file that does not decode is Stuck",
			profilesYML: "light: 3\n",
			settings:    battenshed.RunSettings{Recipe: "loom", Profile: "light"},
			wantStuck:   []string{`profile "light"`},
		},
		{
			name:        "a profiles file that does not decode is Stuck for a task naming no profile",
			profilesYML: "light: 3\n",
			settings:    battenshed.RunSettings{Recipe: "loom"},
			wantStuck:   []string{`profile "light"`},
		},
		{
			name:        "a refused task key is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "loom", Profile: "light", Settings: map[string]any{"loom": map[string]any{"no_such_key": float64(1)}}},
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
			if len(got) != len(tt.wantDocs) {
				t.Fatalf("prepareRunSettings() = %v; want %d modules", got, len(tt.wantDocs))
			}
			for module, want := range tt.wantDocs {
				if string(got[module]) != want {
					t.Errorf("document for %s = %q; want %q", module, got[module], want)
				}
			}
		})
	}
}
