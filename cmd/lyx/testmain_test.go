// testmain_test.go wires cmd/lyx's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so cmd/lyx's e2e tests (which spawn the lyx
// binary, which itself spawns git) never inherit the operator's global gitconfig (see
// CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
// This is what makes the no-daemon guarantee reach through the launched binary: HermeticGitEnv
// mutates this test process's own environment, which launched child processes inherit by default.

package main

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs HermeticGitEnv() before any test spawns git.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
