// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the
// operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant), and so
// this package's integration-tagged sibling, which does spawn git, is already covered by one
// declaration.

package lifecyclecli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs the hermetic git setup once before the whole suite.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
