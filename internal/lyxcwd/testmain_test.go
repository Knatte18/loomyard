// testmain_test.go wires the package's test binary into the hermetic git test environment: lyxtest.HermeticGitEnv() runs once before any test, so lyxcwd's git-spawning fixtures never inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
// This file lives in the external package lyxcwd_test, not the internal lyxcwd package: lyxtest imports lyxcwd (the lyxtest Leaf Invariant's direction), so an internal test file importing lyxtest would close a test-build cycle.

package lyxcwd_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package spawns git, then delegates to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
