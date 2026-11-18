// denypython_integration_test.go proves the orch launch's config wiring carries the hub's deny_python into the launch.
// The python hook row stays installed whatever the value.
// The notice names Python only while the value is true.

package orchcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOrchIntegration_WireCLILaunchFollowsDenyPythonAtTheDefaultLayer sets deny_python at a hub's default layer.
// It wires the orch CLI over that hub, and prepares a launch on the wired engine set with the wired shuttle config.
// The launch's system-prompt notice holds the python sentence for true and not for false.
// The launch's settings carry the python hook's live check for both.
func TestOrchIntegration_WireCLILaunchFollowsDenyPythonAtTheDefaultLayer(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const engine = "claude"
	var pythonNotices []string
	for _, row := range shellguard.Rows("", nil) {
		if row.Switch == shellguard.SwitchDenyPython {
			pythonNotices = append(pythonNotices, row.NoticeFor(engine))
		}
	}
	if len(pythonNotices) == 0 {
		t.Fatalf("shellguard.Rows has no row on the python switch")
	}

	tests := []struct {
		name       string
		denyPython string
		wantNotice bool
	}{
		{name: "DenyPythonFalse", denyPython: "false", wantNotice: false},
		{name: "DenyPythonTrue", denyPython: "true", wantNotice: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			shuttleFile := configengine.ConfigFile(fabricengine.BoltDir(h.Path), "shuttle")
			if err := os.MkdirAll(filepath.Dir(shuttleFile), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", filepath.Dir(shuttleFile), err)
			}
			config := strings.Replace(shuttleengine.ConfigTemplate(), "deny_python: true", "deny_python: "+tt.denyPython, 1)
			if err := os.WriteFile(shuttleFile, []byte(config), 0o644); err != nil {
				t.Fatalf("write %s: %v", shuttleFile, err)
			}

			c := &orchCLI{}
			if err := wireCLI(c, h.Location); err != nil {
				t.Fatalf("wireCLI() error = %v", err)
			}
			launcher, err := c.engines.Lookup(engine)
			if err != nil {
				t.Fatalf("Lookup(%q) error: %v", engine, err)
			}
			runDir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "do the thing", PromptEngine: engine}
			if _, err := launcher.Prepare(runDir, spec, c.shuttleCfg); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}

			notice, err := os.ReadFile(filepath.Join(runDir, "append-system-prompt.txt"))
			if err != nil {
				t.Fatalf("read the system-prompt file: %v", err)
			}
			for _, pythonNotice := range pythonNotices {
				if got := strings.Contains(string(notice), pythonNotice); got != tt.wantNotice {
					t.Errorf("deny_python=%s: notice holds %q = %v; want %v (notice: %s)", tt.denyPython, pythonNotice, got, tt.wantNotice, notice)
				}
			}
			settings, err := os.ReadFile(filepath.Join(runDir, "settings.json"))
			if err != nil {
				t.Fatalf("read the settings file: %v", err)
			}
			if !strings.Contains(string(settings), shuttleengine.PythonAllowedHookVerb) {
				t.Errorf("deny_python=%s: settings hold no python hook live check %q; want the row installed", tt.denyPython, shuttleengine.PythonAllowedHookVerb)
			}
		})
	}
}
