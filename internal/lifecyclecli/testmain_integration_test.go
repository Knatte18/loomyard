//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git
// test environment: gitkit.HermeticGitEnv() runs once before any test, since
// lifecycle_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant /
// Hermetic Git Test Environment Invariant), in the shape internal/landingshed's own equivalent
// already uses.

package lifecyclecli

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
