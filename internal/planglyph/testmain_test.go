// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, ahead of the integration-tagged tests batch 7
// adds to this package that spawn git via (*quarry.Repo).DeltaGit, per the Hermetic Git Test
// Environment Invariant (CONSTRAINTS.md). This batch's own tests spawn no process — they build a
// fixture repository under t.TempDir() and call quarry.Open/Resolve, which read files only — but
// the wiring is in place ahead of the first test that needs it.

package planglyph

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs hermetic git environment setup before tests.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
