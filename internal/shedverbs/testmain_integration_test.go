//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, since loop_arming_integration_test.go builds a real hub through internal/hubforge and therefore spawns git.
// A test process the loop tests start from this same binary carries the helper-role environment variable and runs as that role instead of the tests.

package shedverbs

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs a helper role when the binary was started as one, else HermeticGitEnv before any test spawns git, then the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	if code, isHelper := runHelperRole(); isHelper {
		os.Exit(code)
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
