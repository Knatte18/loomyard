// codexlaunch_test.go pins that the registry wires the Claude user setup into a Codex launch.

package shuttleengines

import (
	"os"
	"path/filepath"
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
