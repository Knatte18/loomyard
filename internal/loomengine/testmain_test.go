// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so
// loomengine's fixture-driven integration tests never inherit the operator's
// global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant), mirroring internal/warpengine/testmain_test.go.

package loomengine

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package
// spawns git, then delegates to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
