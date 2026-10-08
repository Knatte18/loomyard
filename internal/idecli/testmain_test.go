// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so idecli's git-spawning fixtures never
// inherit the operator's global gitconfig (see
// PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main, and never writes the operator's VS Code keybindings.json.

package idecli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv before spawning git tests, points KeybindingsPath at a temporary directory for the whole package run, then runs the tests under tmuxkit.Main.
// KeybindingsPath is process-global state, so no test writes the operator's real keybindings.json.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	dir, err := os.MkdirTemp("", "keybindings-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create keybindings dir:", err)
		os.Exit(1)
	}
	ideengine.KeybindingsPath = func() (string, error) { return filepath.Join(dir, "keybindings.json"), nil }
	code := tmuxkit.Main(m)
	os.RemoveAll(dir)
	os.Exit(code)
}
