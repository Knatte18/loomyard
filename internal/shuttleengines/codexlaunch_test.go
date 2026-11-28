// codexlaunch_test.go pins that the registry wires the Claude user setup into a Codex launch.

package shuttleengines

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeFixtureFiles writes each path of files with its content, making the directories.
func writeFixtureFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// newCodexLaunchFixture returns the registry's codex engine over a fake worktree.
// It also returns that worktree and the Claude config directory.
// The config directory holds a global CLAUDE.md.
// It sets CLAUDE_CONFIG_DIR and HOME, so a test that calls it does not run in parallel.
func newCodexLaunchFixture(t *testing.T) (engine shuttleengine.Engine, anchor, configDir string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	anchor = filepath.Join(base, "worktree")
	lymHome, operatorHome, configDir := filepath.Join(base, "lym-home"), filepath.Join(base, "operator-home"), filepath.Join(base, "claude-config")
	writeFixtureFiles(t, map[string]string{
		filepath.Join(anchor, ".git", "HEAD"):                   "ref: refs/heads/main\n",
		filepath.Join(operatorHome, "auth.json"):                "{}",
		filepath.Join(configDir, "CLAUDE.md"):                   "the global rules of the operator",
		configengine.ConfigFile(anchor, "codex"):                "sandbox_mode: danger-full-access\nwritable_roots: []\n",
		configengine.LocalFile(anchor, "codex"):                 "home: " + lymHome + "\noperator_home: " + operatorHome + "\n",
		filepath.Join(operatorHome, "sessions", ".placeholder"): "",
	})
	for _, name := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(anchor, ".git", name), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", base)

	set, err := New(shuttleengine.Config{Engine: "claude"}, configengine.Dirs{Anchor: anchor})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	engine, err = set.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(codex) error: %v", err)
	}
	return engine, anchor, configDir
}

// TestNewWithSeams_CodexTakesTheGivenSeams pins that a set built with seams runs the given hook runner and sandbox probe.
// Codex's Prepare calls the probe once under codex.sandbox_mode auto,
// and the runner once for the installed plugin's hook.
// The runner's context rides the launch line.
// The nil-seam fallbacks run `sh` and `codex`, so no tier0 test reaches them.
// It sets CLAUDE_CONFIG_DIR and HOME, so it does not run in parallel.
func TestNewWithSeams_CodexTakesTheGivenSeams(t *testing.T) {
	tiergate.Require(t, 0)

	_, anchor, configDir := newCodexLaunchFixture(t)
	pluginRoot := filepath.Join(configDir, "plugins", "p")
	writeFixtureFiles(t, map[string]string{
		configengine.ConfigFile(anchor, "codex"):                      "sandbox_mode: auto\nwritable_roots: []\n",
		configengine.ConfigFile(anchor, "claude"):                     "session_scope:\n  enabled: false\n",
		filepath.Join(configDir, "plugins", "installed_plugins.json"): `{"plugins":{"p@m":[{"scope":"user","installPath":"` + pluginRoot + `"}]}}`,
		filepath.Join(pluginRoot, "hooks", "hooks.json"):              `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"emit"}]}]}}`,
	})
	var hookCommands, probeDirs []string
	seams := LaunchSeams{
		RunHook: func(run shuttleengine.HookRun) ([]byte, error) {
			hookCommands = append(hookCommands, run.Command)
			return []byte("seam-context-marker"), nil
		},
		SandboxProbe: func(_, _, dir string) error {
			probeDirs = append(probeDirs, dir)
			return nil
		},
	}
	set, err := NewWithSeams(shuttleengine.Config{Engine: "claude"}, configengine.Dirs{Anchor: anchor}, seams)
	if err != nil {
		t.Fatalf("NewWithSeams() error: %v", err)
	}
	engine, err := set.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(codex) error: %v", err)
	}

	launch, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p", PromptEngine: "codex"}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !reflect.DeepEqual(hookCommands, []string{"emit"}) {
		t.Errorf("hook runner commands = %v; want the plugin's one hook", hookCommands)
	}
	if !reflect.DeepEqual(probeDirs, []string{anchor}) {
		t.Errorf("sandbox probe dirs = %v; want one probe in %q", probeDirs, anchor)
	}
	if !strings.Contains(launch.Cmd, "seam-context-marker") {
		t.Errorf("Prepare() launch line = %q; want it to carry the runner's context", launch.Cmd)
	}
}

// TestNew_CodexLaunchCarriesTheClaudeUserSetup pins the wiring in the registry's codex entry.
// A Codex launch's command line carries the user's global CLAUDE.md, which only the Claude source reads.
// It sets CLAUDE_CONFIG_DIR and HOME, so it does not run in parallel.
func TestNew_CodexLaunchCarriesTheClaudeUserSetup(t *testing.T) {
	tiergate.Require(t, 0)

	engine, _, _ := newCodexLaunchFixture(t)

	launch, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p", PromptEngine: "codex"}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !strings.Contains(launch.Cmd, "the global rules of the operator") {
		t.Errorf("Prepare() launch line = %q; want it to carry the global CLAUDE.md's text", launch.Cmd)
	}
}
