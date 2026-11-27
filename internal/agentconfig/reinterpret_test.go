// reinterpret_test.go pins the removal of the retired pr-reviewer role from a hub agents.yaml.

package agentconfig_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDropPRReviewerRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const skillsRemoved = "skills.pr-reviewer is removed, since lym no longer starts the pr-reviewer role"
	tests := []struct {
		name        string
		existing    string
		wantRewrote []string
		// wantKept are texts the output must still hold.
		wantKept []string
	}{
		{
			name: "the role's list and efforts go, and comments and other keys stay",
			existing: "# the hub's agents\nskills:\n  pr-reviewer: [scribe:x]\n  darn: [scribe:y]  # darn's own\n" +
				"efforts:\n  default: medium\n  pr-reviewer: low\n  pr-reviewer/opus: high\n  darn/opus: max\n",
			wantRewrote: []string{
				skillsRemoved,
				"efforts.pr-reviewer is removed, since lym no longer starts the pr-reviewer role",
				"efforts.pr-reviewer/opus is removed, since lym no longer starts the pr-reviewer role",
			},
			wantKept: []string{"# the hub's agents", "# darn's own", "darn: ['scribe:y']", "default: medium", "darn/opus: max"},
		},
		{
			name:        "a section holding the role alone is emptied and keeps its comments",
			existing:    "# the hub's agents\nskills:\n  pr-reviewer: [scribe:x]\n",
			wantRewrote: []string{skillsRemoved},
			wantKept:    []string{"# the hub's agents"},
		},
		{
			name:     "a file naming the role nowhere comes back unchanged",
			existing: "skills:\n  darn:   [scribe:y]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := agentconfig.DropPRReviewerRole([]byte(tt.existing), nil)
			if err != nil {
				t.Fatalf("DropPRReviewerRole() error = %v", err)
			}
			if !slices.Equal(rewrote, tt.wantRewrote) || len(flagged) != 0 {
				t.Errorf("DropPRReviewerRole() rewrote = %q, flagged = %q; want %q and no flag", rewrote, flagged, tt.wantRewrote)
			}
			if len(tt.wantRewrote) == 0 && string(got) != tt.existing {
				t.Errorf("DropPRReviewerRole() = %q; want the input back byte for byte %q", got, tt.existing)
			}
			for _, kept := range tt.wantKept {
				if !strings.Contains(string(got), kept) {
					t.Errorf("output = %q; want it to keep %q", got, kept)
				}
			}
			if _, err := loadHub(t, string(got)); err != nil {
				t.Errorf("Load(%q) error = %v; want the output to load", got, err)
			}
		})
	}

	t.Run("a worktree's local file is flagged and left as it is", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		path := configengine.LocalFile(anchor, agentconfig.ModuleName)
		const local = "efforts:\n  pr-reviewer/opus: high\n"
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(local), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}

		_, rewrote, flagged, err := agentconfig.DropPRReviewerRole(nil, []configengine.HubWorktree{{Name: "task", Anchor: anchor}})

		want := []string{path + " sets efforts.pr-reviewer/opus, of the pr-reviewer role lym no longer starts, so agents.yaml stops loading there; " + layerWayForward}
		if err != nil || len(rewrote) != 0 || !slices.Equal(flagged, want) {
			t.Errorf("DropPRReviewerRole() rewrote = %q, flagged = %q, %v; want no rewrite and %q", rewrote, flagged, err, want)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != local {
			t.Errorf("local file = %q, %v; want it left as %q", got, err, local)
		}
	})
}
