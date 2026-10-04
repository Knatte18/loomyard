// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the
// operator's global gitconfig (see PATTERN-hermetic-git-tests), and so
// batch 7's later tagged tests -- which do spawn git -- are already covered by one declaration.

package loomcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the hermetic git setup once before the whole suite, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
