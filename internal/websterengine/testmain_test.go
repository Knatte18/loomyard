// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so websterengine's git-spawning fixtures
// (begin-batch's HeadSHA capture, record-batch's ChangedFiles/Dirty, chain rollback's ResetHard)
// never inherit the operator's global gitconfig (see PATTERN-test-isolation).
// Every tier0 test in this package still spawns nothing;
// only the tier1 tests ever reach git,
// but TestMain runs for the whole test binary regardless of which tier a run selects.

package websterengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
