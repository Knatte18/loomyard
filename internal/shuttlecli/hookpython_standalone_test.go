// hookpython_standalone_test.go pins the hidden hook-python-allowed verb in a standalone run.
// No hub holds the target, so its exit code follows the deny_python of the config in the target's state directory.

package shuttlecli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHookPythonAllowedVerb_FollowsAStandaloneRunsStateDirConfig runs the verb from below a git repository.
// No hub holds that repository.
// The verb answers the deny_python of the config in the state directory derived for the repository root.
// A standalone launch loads its shuttle config from that directory.
// A sibling Bolt directory with no `_lym` is a damaged hub, which a launch also runs as standalone.
// With no shuttle file in the state directory, the verb answers the template default, which denies.
// It does not run in parallel: it points the state root at a temp directory through the environment.
func TestHookPythonAllowedVerb_FollowsAStandaloneRunsStateDirConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	tests := []struct {
		name string
		// denyPython is the value written into the state directory's shuttle file; empty writes no shuttle file.
		denyPython string
		// damagedHub adds a Bolt directory beside the target that holds no `_lym`.
		damagedHub bool
		wantCode   int
	}{
		{name: "PythonAllowed", denyPython: "false", wantCode: 0},
		{name: "PythonDenied", denyPython: "true", wantCode: 1},
		{name: "PythonAllowedInADamagedHub", denyPython: "false", damagedHub: true, wantCode: 0},
		{name: "NoShuttleFileKeepsTheTemplateDefault", wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			// The target holds the entries dotgit reads as a usable repository, so no process is spawned.
			gitDir := filepath.Join(target, ".git")
			dirs := []string{filepath.Join(gitDir, "objects"), filepath.Join(gitDir, "refs"), filepath.Join(target, "below", "the", "root")}
			if tt.damagedHub {
				dirs = append(dirs, fabriclayout.BoltDir(root))
			}
			for _, dir := range dirs {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
				t.Fatalf("write HEAD: %v", err)
			}
			if tt.denyPython != "" {
				stateDir, err := cliwire.StandaloneStateDir(target)
				if err != nil {
					t.Fatalf("StandaloneStateDir(%s) error: %v", target, err)
				}
				shuttleFile := configengine.ConfigFile(stateDir, "shuttle")
				if err := os.MkdirAll(filepath.Dir(shuttleFile), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", filepath.Dir(shuttleFile), err)
				}
				if err := os.WriteFile(shuttleFile, withDenyPython(tt.denyPython), 0o644); err != nil {
					t.Fatalf("write %s: %v", shuttleFile, err)
				}
			}
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), filepath.Join(target, "below", "the", "root"), &out, []string{shuttleengine.PythonAllowedHookVerb})
			if code != tt.wantCode || out.Len() != 0 {
				t.Errorf("ExecuteIn(%s) = %d, output %q; want %d and no output", shuttleengine.PythonAllowedHookVerb, code, out.String(), tt.wantCode)
			}
		})
	}
}
