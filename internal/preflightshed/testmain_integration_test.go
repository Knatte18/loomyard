//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git
// test environment: gitkit.HermeticGitEnv() runs once before any test, since this file's sibling
// (preflight_integration_test.go) spawns git via hubforge fixtures (Test Tier Purity Invariant /
// Hermetic Git Test Environment Invariant).

package preflightshed

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
