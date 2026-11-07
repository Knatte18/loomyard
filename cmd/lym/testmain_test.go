// testmain_test.go wires cmd/lym's test binary into the hermetic git test environment.
// gitkit.HermeticGitEnv() runs once before any test, so the e2e tests never inherit the operator's global gitconfig.
// It mutates this process's own environment, which launched child processes inherit by default,
// so the no-daemon guarantee reaches through the launched binary.

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
