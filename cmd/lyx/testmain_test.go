// testmain_test.go wires cmd/lyx's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so cmd/lyx's
// e2e tests (which spawn the lyx binary, which itself spawns git) never inherit
// the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test
// Environment Invariant). This is what makes the no-daemon guarantee reach
// through the launched binary: HermeticGitEnv mutates this test process's own
// environment, which launched child processes inherit by default.

package main

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
