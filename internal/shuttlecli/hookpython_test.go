// hookpython_test.go pins the hidden hook-python-allowed verb over a real hub:
// its exit code follows the hub's deny_python at the default layer,
// and a cwd outside any worktree exits 1 with a JSON error.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// withDenyPython returns the shuttle config template with deny_python set to value.
func withDenyPython(value string) []byte {
	return []byte(strings.Replace(shuttleengine.ConfigTemplate(), "deny_python: true", "deny_python: "+value, 1))
}

// assertJSONError fails the test unless output is one JSON error object whose message opens with "shuttle: ".
func assertJSONError(t *testing.T, output string) {
	t.Helper()
	var got struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &got); err != nil {
		t.Errorf("output = %q; want a JSON error object: %v", output, err)
		return
	}
	if got.OK || !strings.HasPrefix(got.Error, "shuttle: ") {
		t.Errorf("output = %q; want ok=false and an error opening with %q", output, "shuttle: ")
	}
}

// TestHookPythonAllowedVerb_FollowsTheHubsDenyPython runs the verb through the shuttle command tree.
// It runs from a hub's prime worktree, or from a directory below its anchor.
// It sets deny_python at the hub's default layer before each row, and the verb answers the value then set:
// exit 0 with nothing printed for false, exit 1 with nothing printed for true.
// A config that does not parse exits 1 with a JSON error, so a broken config never allows Python.
// From a directory outside any worktree it exits 1 and reports why as a JSON error.
// The rows share one hub and rewrite its config in turn, so they do not run in parallel.
func TestHookPythonAllowedVerb_FollowsTheHubsDenyPython(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.NewHub(t, ".")
	shuttleFile := configengine.ConfigFile(hub.BoardDir(), "shuttle")
	if err := os.MkdirAll(filepath.Dir(shuttleFile), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(shuttleFile), err)
	}
	tests := []struct {
		name string
		// denyPython is the value written at the default layer; empty with no rawConfig writes nothing and runs outside the hub.
		denyPython string
		// rawConfig is written at the default layer as it stands, in place of the template with denyPython.
		rawConfig string
		// belowAnchor runs the verb from a new directory under the prime worktree.
		belowAnchor bool
		wantCode    int
		// wantError wants a JSON error object as the output; otherwise the output is empty.
		wantError bool
	}{
		{name: "PythonAllowed", denyPython: "false", wantCode: 0},
		{name: "PythonDenied", denyPython: "true", wantCode: 1},
		{name: "AllowedAgainAfterAChange", denyPython: "false", wantCode: 0},
		{name: "AllowedFromBelowTheAnchor", denyPython: "false", belowAnchor: true, wantCode: 0},
		{name: "ConfigThatDoesNotLoad", rawConfig: "deny_python: [unclosed\n", wantCode: 1, wantError: true},
		{name: "OutsideAWorktree", wantCode: 1, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.denyPython != "" || tt.rawConfig != "" {
				dir = hub.PrimeWorktree()
				if tt.belowAnchor {
					dir = filepath.Join(dir, "below", "the", "anchor")
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir %s: %v", dir, err)
					}
				}
				content := withDenyPython(tt.denyPython)
				if tt.rawConfig != "" {
					content = []byte(tt.rawConfig)
				}
				if err := os.WriteFile(shuttleFile, content, 0o644); err != nil {
					t.Fatalf("write %s: %v", shuttleFile, err)
				}
			}
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), dir, &out, []string{shuttleengine.PythonAllowedHookVerb})
			if code != tt.wantCode {
				t.Errorf("ExecuteIn(%s) = %d; want %d; output: %s", shuttleengine.PythonAllowedHookVerb, code, tt.wantCode, out.String())
			}
			if tt.wantError {
				assertJSONError(t, out.String())
			} else if out.Len() != 0 {
				t.Errorf("output = %q; want none", out.String())
			}
		})
	}
}
