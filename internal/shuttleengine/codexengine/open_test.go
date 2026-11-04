// open_test.go covers OpenLaunch: the launch of a session opened with no prompt, the lym home it readies, and the refusals that write nothing.

package codexengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOpenLaunch pins the launch an opened session gets and what it refuses.
// A valid spec gets the Codex command with the told records as its writable root, the model and the effort, no hooks, no first turn, an empty resume line, no creel hooks and no session id,
// and the lym home trusts the checkout's roots while the session directory stays unmade.
// A permission mode other than bypass, an effort codex.yaml does not list and Codex rules in the worktree are refused before the lym home is made.
func TestOpenLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		spec shuttleengine.OpenSpec
		// setup changes the worktree before OpenLaunch.
		setup func(t *testing.T, anchor string)
		// wantErr holds the texts a refusal must carry; empty means the launch goes ahead.
		wantErr []string
	}{
		{name: "BypassDefault", spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-luna", Effort: "high"}}},
		{name: "PromptPermissionMode", spec: shuttleengine.OpenSpec{Role: "explore", PermissionMode: "prompt"}, wantErr: []string{`permission mode "prompt"`, "way forward"}},
		{name: "EffortOutsideEfforts", spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Effort: "xhigh"}}, wantErr: []string{`effort "xhigh"`, "way forward"}},
		{
			name: "RulesInWorktree",
			spec: shuttleengine.OpenSpec{Role: "explore"},
			setup: func(t *testing.T, anchor string) {
				mkdirAll(t, filepath.Join(anchor, codexFolderName, rulesDirName))
			},
			wantErr: []string{filepath.Join(codexFolderName, rulesDirName), "way forward"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, records := repo.addWorktree(t, "wt")
			if tt.setup != nil {
				tt.setup(t, anchor)
			}
			engine, _ := newEngine(anchor)
			engine.dirs.Records = records
			sessionDir := filepath.Join(t.TempDir(), "session")

			launch, err := engine.OpenLaunch(sessionDir, tt.spec, shuttleengine.Config{})
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("OpenLaunch() error = nil; want a refusal")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("OpenLaunch() error %q does not contain %q", err, want)
					}
				}
				if _, statErr := os.Lstat(repo.lymHome); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("lym home stat error = %v; want the home unmade", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenLaunch() error: %v", err)
			}

			want := shuttleengine.Launch{Cmd: codexCommand(shell.ForGOOS(), launchLine{
				bin:           "codex",
				home:          repo.lymHome,
				writableRoots: []string{records},
				model:         tt.spec.Model,
				effort:        tt.spec.Effort,
			})}
			if !reflect.DeepEqual(launch, want) {
				t.Errorf("OpenLaunch() launch = %+v; want %+v", launch, want)
			}
			if got, want := readRecord(t, repo.lymHome).Roots, []string{repo.mainRoot, anchor}; !reflect.DeepEqual(got, want) {
				t.Errorf("trust record roots = %v; want %v", got, want)
			}
			if _, statErr := os.Lstat(sessionDir); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("session dir stat error = %v; want the directory unmade", statErr)
			}
		})
	}
}
