// testmain_test.go wires cmd/lyx's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so cmd/lyx's e2e tests (which spawn the lyx
// binary, which itself spawns git) never inherit the operator's global gitconfig (see
// `PATTERN-test-isolation`).
// This is what makes the no-daemon guarantee reach through the launched binary: HermeticGitEnv
// mutates this test process's own environment, which launched child processes inherit by default.
// The binary also runs under tmux isolation: tmuxkit.Main points the tests at a private tmux socket directory.

package main

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs HermeticGitEnv() before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
