// testmain_test.go wires the package's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test.
// Fixture-driven integration tests never inherit the operator's global gitconfig (see PATTERN-test-isolation).
// This file stays untagged, so it is compiled into both the tagged and the untagged test binary.

package preflight

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
