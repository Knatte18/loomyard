// testmain_test.go holds the package's TestMain, which sets up the test binary's git, keybindings and tmux isolation.

package idecli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain calls `gitkit.HermeticGitEnv` before any test, then runs the tests under `tmuxkit.Main`.
// It points `KeybindingsPath` at a temporary directory for the whole package run.
// `KeybindingsPath` is process-global state, so no test writes the operator's real keybindings.json.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	dir, err := lymtemp.MkdirTemp("keybindings-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create keybindings dir:", err)
		os.Exit(1)
	}
	ideengine.KeybindingsPath = func() (string, error) { return filepath.Join(dir, "keybindings.json"), nil }
	code := tmuxkit.Main(m)
	os.RemoveAll(dir)
	os.Exit(code)
}
