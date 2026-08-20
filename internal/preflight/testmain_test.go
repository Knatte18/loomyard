// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so preflight's fixture-driven integration
// tests never inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test
// Environment Invariant), mirroring internal/loomengine/testmain_test.go.
// It stays untagged so it is compiled into both the tagged and the untagged test binary.

package preflight

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs HermeticGitEnv before any test spawns git.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
