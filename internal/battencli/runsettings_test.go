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

// TestPrepareRunSettings pins how Seed-Child's prepare step lays a task's run settings over its run profile, and that an unknown profile, a profiles file that does not decode, and a refused key each route to Stuck through ErrInvalidRunSettings.
func TestPrepareRunSettings(t *testing.T) {
	t.Parallel()
	const profiles = "light:\n  loom: {plan_max_bounces: 1, merge_max_bounces: 1}\n"
	tests := []struct {
		name        string
		profilesYML string
		settings    battenshed.RunSettings
		wantDocs    map[string]string
		wantStuck   []string
	}{
		{
			name:     "no profile writes the task's settings alone",
			settings: battenshed.RunSettings{Settings: map[string]any{"darn": map[string]any{"review_max_bounces": float64(1)}}},
			wantDocs: map[string]string{"darn": "review_max_bounces: 1\n"},
		},
		{
			name:        "the task's settings lie over the profile's",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Profile: "light", Settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}}},
			wantDocs:    map[string]string{"loom": "merge_max_bounces: 1\nplan_max_bounces: 3\n"},
		},
		{
			name:        "an unknown profile is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Profile: "heavy"},
			wantStuck:   []string{`"heavy"`, "known: light"},
		},
		{
			name:        "a profiles file that does not decode is Stuck",
			profilesYML: "light: 3\n",
			settings:    battenshed.RunSettings{Profile: "light"},
			wantStuck:   []string{`profile "light"`},
		},
		{
			name:        "a refused task key is Stuck naming it",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Profile: "light", Settings: map[string]any{"loom": map[string]any{"no_such_key": float64(1)}}},
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
