// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so fabriccli's git-spawning fixtures never
// inherit the operator's global gitconfig (see
// PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.

package fabriccli_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test spawns git, then runs the tests under tmuxkit.Main.
// The add verb's pair push runs in the foreground: a detached one would fork this test binary, and the tests read the pushed branches on the remote.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	fabriccli.SetPairPushForTest(func(codeWorktree, recordsWorktree string) error {
		_, err := fabricengine.CoalescePushBothAt(codeWorktree, recordsWorktree, fabricengine.EnvSyncOptions())
		return err
	})
	os.Exit(tmuxkit.Main(m))
}
