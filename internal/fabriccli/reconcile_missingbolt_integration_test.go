// reconcile_missingbolt_integration_test.go drives hubs whose Bolt checkout is gone.
// A clone killed after its records store and before its Bolt checkout leaves such a hub.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommand_HubWithoutItsBoltNamesTheResetClone asserts both ways out of such a hub name the re-clone with --reset.
// `lym fabric reconcile` refuses naming it, and a re-clone into the same directory refuses naming it too.
// A one-repository hub names its join and its seed, and a two-repository hub names both of its origin URLs.
// A hub whose records origin lacks names the clone that discards them.
func TestCommand_HubWithoutItsBoltNamesTheResetClone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// build returns the hub, the options of the clone into its container, and the reset clones the two refusals name.
		build func(t *testing.T) (h *hubforge.Hub, opts fabriclayout.CloneOptions, wantReconcileReplacement, wantCloneReplacement string)
	}{
		{"OneRepository", func(t *testing.T) (*hubforge.Hub, fabriclayout.CloneOptions, string, string) {
			h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
			url := filepath.ToSlash(h.CodeBare)
			join := "`lym fabric clone " + url + " --into " + h.Container + " --reset`"
			seed := "`lym fabric clone --single-repo --shortname <shortname> " + url + " --into " + h.Container + " --reset`"
			return h, fabriclayout.CloneOptions{WeftURL: url}, join + " when origin holds the hub's board, or " + seed + " when it holds none", join
		}},
		{"TwoRepository", func(t *testing.T) (*hubforge.Hub, fabriclayout.CloneOptions, string, string) {
			h := hubforge.NewHub(t, ".")
			// A clone refusal names the plain --reset only for a hub whose records origin holds.
			gitkit.Git(t, h.PrimeRecords(), "push", "-q", "origin", "--all")
			recordsURL, codeURL := filepath.ToSlash(h.RecordsBare), filepath.ToSlash(h.CodeBare)
			clone := "`lym fabric clone " + recordsURL + " " + codeURL + " --into " + h.Container + " --reset`"
			return h, fabriclayout.CloneOptions{WeftURL: recordsURL, WarpURL: codeURL}, clone, clone
		}},
		{"TwoRepositoryWithUnpushedRecords", func(t *testing.T) (*hubforge.Hub, fabriclayout.CloneOptions, string, string) {
			h := hubforge.NewHub(t, ".")
			recordsURL, codeURL := filepath.ToSlash(h.RecordsBare), filepath.ToSlash(h.CodeBare)
			clone := "`lym fabric clone " + recordsURL + " " + codeURL + " --into " + h.Container + " --reset --discard-work`"
			return h, fabriclayout.CloneOptions{WeftURL: recordsURL, WarpURL: codeURL},
				clone + "; that clone loses the work origin lacks: ", "replace the hub and lose that work with " + clone
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, opts, wantReconcileReplacement, wantCloneReplacement := tt.build(t)
			boltDir := fabriclayout.BoltDir(h.Path)
			if err := os.RemoveAll(boltDir); err != nil {
				t.Fatalf("remove the Bolt checkout: %v", err)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "reconcile")
			if code == 0 {
				t.Fatalf("Execute(reconcile) = 0; want a refusal\noutput: %s", output)
			}
			wantReconcile := "the hub at " + h.Path + " has no repository at its " + fabriclayout.BoltDirName + " checkout " + boltDir +
				", as a clone that stopped before making it leaves; way forward: replace the hub with " + wantReconcileReplacement
			// A refusal over unpushed work goes on to name each commit count, so only its opening is pinned.
			losesWork := strings.HasSuffix(wantReconcileReplacement, ": ")
			result := envelope.RequireErr(t, output, "")
			if losesWork {
				if !strings.HasPrefix(result.Error, wantReconcile) || !strings.Contains(result.Error, h.PrimeRecords()+" that origin lacks") {
					t.Errorf("Execute(reconcile) error = %q; want it to open %q and name the records store's commits", result.Error, wantReconcile)
				}
			} else if result.Error != wantReconcile {
				t.Errorf("Execute(reconcile) error = %q; want %q", result.Error, wantReconcile)
			}

			_, err := fabricengine.CloneHub(h.Container, opts)
			wantClone := "hub already exists at " + h.Path + "; way forward: replace it with " + wantCloneReplacement
			if losesWork {
				if err == nil || !strings.Contains(err.Error(), wantCloneReplacement) {
					t.Errorf("CloneHub(%+v) error = %v; want it to hold %q", opts, err, wantCloneReplacement)
				}
			} else if err == nil || err.Error() != wantClone {
				t.Errorf("CloneHub(%+v) error = %v; want %q", opts, err, wantClone)
			}
		})
	}
}
