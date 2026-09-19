// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so reedcli's git-spawning fixtures never
// inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).

package reedcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain arms the hermetic git test environment before any test runs.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
