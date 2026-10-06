// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so lyxcwd's git-spawning fixtures never
// inherit the operator's global gitconfig (see
// PATTERN-test-isolation).
// This file lives in the external package lyxcwd_test, not the internal lyxcwd package: gitkit
// imports lyxcwd (the gitkit Leaf Invariant's direction), so an internal test file importing
// gitkit would close a test-build cycle.

package lyxcwd_test

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
