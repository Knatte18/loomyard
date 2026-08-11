// testmain_test.go wires the package's test binary into the hermetic git test environment:
// lyxtest.HermeticGitEnv() runs once before any test, so fabrictest's git-spawning fixtures never
// inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).
// This is what satisfies that invariant for a package whose every other file spawns git; the guard
// named in CONSTRAINTS.md and enforced by cmd/lyx/hermeticenv_test.go looks for exactly this call.
//
// It carries no build tag deliberately: it must be compiled into the test binary on both a plain
// `go test` and a `-tags integration` run, or the integration-tagged suites this package's other files
// carry would run without the hermetic environment ever having been installed.

package fabrictest

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain runs lyxtest.HermeticGitEnv() before any test in this package spawns git, then delegates
// to the normal test run.
func TestMain(m *testing.M) {
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
