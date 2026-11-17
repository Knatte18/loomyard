//go:build !windows

// codexhook_integration_test.go pins that the registry gives the Codex engine the real plugin hook runner.

package shuttleengines

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestNew_CodexLaunchCarriesAPluginSessionStartContext pins the hook runner in the registry's codex entry.
// An installed plugin's SessionStart command hook runs through `sh`, and its output rides the launch line.
// It sets CLAUDE_CONFIG_DIR and HOME, so it does not run in parallel.
func TestNew_CodexLaunchCarriesAPluginSessionStartContext(t *testing.T) {
	tiergate.Require(t, 1)

	engine, anchor, configDir := newCodexLaunchFixture(t)
	pluginRoot := filepath.Join(configDir, "plugins", "p")
	writeFixtureFiles(t, map[string]string{
		configengine.ConfigFile(anchor, "claude"):                     "session_scope:\n  enabled: false\n",
		filepath.Join(configDir, "plugins", "installed_plugins.json"): `{"plugins":{"p@m":[{"scope":"user","installPath":"` + pluginRoot + `"}]}}`,
		filepath.Join(pluginRoot, "hooks", "hooks.json"):              `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"printf plugin-context-marker"}]}]}}`,
	})

	launch, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p", PromptEngine: "codex"}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !strings.Contains(launch.Cmd, "plugin-context-marker") {
		t.Errorf("Prepare() launch line = %q; want it to carry the plugin hook's output", launch.Cmd)
	}
}
