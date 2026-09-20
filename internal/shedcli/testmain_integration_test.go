//go:build integration

// testmain_integration_test.go wires this package's integration test binary into the hermetic git
// test environment: gitkit.HermeticGitEnv() runs once before any test, since parity_test.go builds a
// real hub through internal/hubforge's fabric-fixture entry point and therefore spawns git (Test
// Tier Purity Invariant / Hermetic Git Test Environment Invariant), in the shape
// internal/battencli's own equivalent already uses.

package shedcli

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
