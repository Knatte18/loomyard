//go:build integration

// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so
// gitrepo's git-spawning fixtures never inherit the operator's global
// gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
// This mirrors internal/gitexec/testmain_test.go exactly.

package gitrepo_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package spawns
// git, then delegates to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
