// open_test.go covers OpenLaunch for a session opened with no prompt.
// It covers the lym home the launch readies and the refusals that write nothing.

package codexengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOpenLaunch pins the launch an opened session gets and what it refuses.
// A valid spec gets the Codex command with the told records and the worktree's git dirs as its writable roots.
// The command carries the model and the effort.
// It runs workspace-write when the sandbox probe reports a sandbox that starts.
// It runs danger-full-access when a layer sets it, or when auto's probe reports one that cannot start.
// It has no hooks, no first turn, an empty resume line, no creel hooks and no session id.
// The lym home trusts the checkout's roots while the session directory stays unmade.
// The launch refuses a permission mode other than bypass and an effort codex.yaml does not list.
// It also refuses Codex rules in the worktree, and each of these refusals comes before the lym home is made.
// It refuses a sandbox that cannot start under workspace-write, naming the cause and the way forward;
// the probe runs under the lym home, so that refusal comes after the home is made.
func TestOpenLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		spec shuttleengine.OpenSpec
		// setup changes the worktree before OpenLaunch.
		setup func(t *testing.T, anchor string)
		// probeErr is the sandbox probe's answer; nil means the sandbox starts.
		probeErr error
		// wantSandboxMode is the sandbox the launch line carries; empty means sandboxWorkspaceWrite.
		wantSandboxMode string
		// wantErr holds the texts a refusal must carry; empty means the launch goes ahead.
		wantErr []string
		// wantProbe reports whether the launch probes the sandbox, with the Codex executable, under the lym home, in the anchor.
		// The probe runs under the home, so a refusal from it leaves the home made and skips the unmade-home check.
		wantProbe bool
	}{
		{name: "BypassDefault", spec: shuttleengine.OpenSpec{Role: "explore", ModelChoice: shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-luna", Effort: "high"}}},
		{
			name: "FullAccessSandboxMode",
			spec: shuttleengine.OpenSpec{Role: "explore"},
			setup: func(t *testing.T, anchor string) {
				writeFile(t, configengine.ConfigFile(anchor, "codex"), "writable_roots: []\nsandbox_mode: "+sandboxFullAccess+"\n")
			},
			wantSandboxMode: sandboxFullAccess,
		},
		{
			name:            "AutoSandboxCannotStart",
			spec:            shuttleengine.OpenSpec{Role: "explore"},
			probeErr:        errors.New("bwrap: setting up uid map: Permission denied"),
			wantSandboxMode: sandboxFullAccess,
			wantProbe:       true,
		},
		{
			name: "WorkspaceWriteSandboxCannotStart",
			spec: shuttleengine.OpenSpec{Role: "explore"},
			setup: func(t *testing.T, anchor string) {
				writeFile(t, configengine.ConfigFile(anchor, "codex"), "writable_roots: []\nsandbox_mode: "+sandboxWorkspaceWrite+"\n")
			},
			probeErr:  errors.New("bwrap: setting up uid map: Permission denied"),
			wantErr:   []string{"bwrap: setting up uid map: Permission denied", "way forward"},
			wantProbe: true,
		},
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
			var probed []string
			engine.sandboxProbe = func(bin, home, dir string) error {
				probed = append(probed, bin, home, dir)
				return tt.probeErr
			}
			engine.dirs.Records = records
			sessionDir := filepath.Join(t.TempDir(), "session")

			launch, err := engine.OpenLaunch(sessionDir, tt.spec, shuttleengine.Config{})

			if tt.wantProbe {
				if wantProbed := []string{"codex", repo.lymHome, anchor}; !reflect.DeepEqual(probed, wantProbed) {
					t.Errorf("sandbox probe called with %q; want %q", probed, wantProbed)
				}
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("OpenLaunch() error = nil; want a refusal")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("OpenLaunch() error %q does not contain %q", err, want)
					}
				}
				if _, statErr := os.Lstat(repo.lymHome); !tt.wantProbe && !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("lym home stat error = %v; want the home unmade", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenLaunch() error: %v", err)
			}

			wantMode := tt.wantSandboxMode
			if wantMode == "" {
				wantMode = sandboxWorkspaceWrite
			}
			want := shuttleengine.Launch{Cmd: codexCommand(shell.ForGOOS(), launchLine{
				bin:           "codex",
				home:          repo.lymHome,
				sandboxMode:   wantMode,
				writableRoots: append([]string{records}, repo.wantGitDirs(anchor)...),
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
