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

// TestPrepareRunSettings pins how Seed-Child's prepare step lays a task's run settings over its run profile's block.
// The block is the recipe's block in the task's profile, or in the recipe's default profile when the task names none.
// It also pins that each of these routes to Stuck through ErrInvalidRunSettings:
// an unknown profile, a profile without the recipe's block, a profiles file that does not decode, or a refused key.
func TestPrepareRunSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const profiles = "defaults: {darn: light}\n" +
		"light:\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\n    loom: {code: {max_bounces: 1}}\n  darn:\n    loom: {code: {max_bounces: 2}}\n" +
		"docs:\n  darn:\n    darn: {writer: opus}\n"
	tests := []struct {
		name        string
		profilesYML string
		settings    battenshed.RunSettings
		wantDocs    map[string]string
		wantStuck   []string
		// wantNoWayForward pins that the refusal leaves the way forward to Seed-Child's wrapper.
		wantNoWayForward bool
	}{
		{
			name:             "a moved task key is Stuck naming both full names and no way forward",
			profilesYML:      profiles,
			settings:         battenshed.RunSettings{Recipe: "tapestry", Settings: map[string]any{"loom": map[string]any{"code_max_bounces": float64(2)}}},
			wantStuck:        []string{"key loom.code_max_bounces was moved to loom.code.max_bounces"},
			wantNoWayForward: true,
		},
		{
			name:     "a motif task naming no profile takes the template's motif default profile's motif block",
			settings: battenshed.RunSettings{Recipe: "motif"},
			wantDocs: map[string]string{"loom": "discussion:\n    hard_max_bounces: 3\n    max_bounces: 2\n"},
		},
		{
			name:        "a motif task naming light takes light's motif block, never its tapestry block",
			profilesYML: "light:\n  tapestry:\n    loom: {discussion: {max_bounces: 3}}\n  motif:\n    loom: {discussion: {max_bounces: 1}}\n",
			settings:    battenshed.RunSettings{Recipe: "motif", Profile: "light"},
			wantDocs:    map[string]string{"loom": "discussion:\n    max_bounces: 1\n"},
		},
		{
			name:        "a motif task naming a profile with a tapestry block and no motif block is refused",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "motif", Profile: "light"},
			wantStuck:   []string{`run profile "light" holds no motif block`},
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
			wantDocs:    map[string]string{"loom": "code:\n    max_bounces: 2\n"},
		},
		{
			name:     "no profiles file takes the template's default for the recipe",
			settings: battenshed.RunSettings{Recipe: "darn"},
			wantDocs: map[string]string{"loom": "code:\n    hard_max_bounces: 2\n    max_bounces: 1\n    reviewer_model: opus[medium]\n"},
		},
		{
			name:     "a sampler task naming the template's light profile writes its loom values and the review on",
			settings: battenshed.RunSettings{Recipe: "sampler", Profile: "light"},
			wantDocs: map[string]string{"loom": "code:\n    hard_max_bounces: 2\n    max_bounces: 1\n", "sampler": "code_review: true\n"},
		},
		{
			name:        "the task's settings lie over the tapestry block with its plan keys in the tapestry document",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "light", Settings: map[string]any{"tapestry": map[string]any{"plan": map[string]any{"max_bounces": float64(3)}}}},
			wantDocs:    map[string]string{"loom": "code:\n    max_bounces: 1\n", "tapestry": "plan:\n    max_bounces: 3\n"},
		},
		{
			name:        "the same profile writes its darn block for a darn task",
			profilesYML: profiles,
			settings:    battenshed.RunSettings{Recipe: "darn", Profile: "light"},
			wantDocs:    map[string]string{"loom": "code:\n    max_bounces: 2\n"},
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
			name:        "a profile holding the recipe's block under its former name is Stuck naming the hand rename",
			profilesYML: "old:\n  loom:\n    loom: {code: {max_bounces: 1}}\n",
			settings:    battenshed.RunSettings{Recipe: "tapestry", Profile: "old"},
			wantStuck:   []string{`"old"`, "no tapestry block but a loom block", "lym config profiles"},
		},
		{
			name:        "a default held under the recipe's former name is Stuck naming the hand rename",
			profilesYML: "defaults: {loom: old}\nold:\n  loom:\n    loom: {code: {max_bounces: 1}}\n",
			settings:    battenshed.RunSettings{Recipe: "tapestry"},
			wantStuck:   []string{"no tapestry entry but a loom entry", "lym config profiles"},
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
				if tt.wantNoWayForward && strings.Contains(err.Error(), "way forward:") {
					t.Errorf("prepareRunSettings() error = %q; want no way forward clause", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("prepareRunSettings() error = %v; want none", err)
			}
			wantDocs := tt.wantDocs
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
