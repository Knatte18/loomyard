// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so burlercli's git-spawning fixtures never
// inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).
//
// This file is required rather than tidy: card 19's integration test drives RunCLIIn, which now
// reaches preflight.ResolveMode and through it lyxcwd.Resolve's real `git rev-parse` spawn, in a
// package that spawns git from no test today. The Hermetic Git Test Environment guard is
// token-keyed and will not catch a spawn reached indirectly through a CLI seam, so nothing else
// would flag the gap.
// Git-config isolation here and the per-test state-root redirect (t.Setenv) are two separate
// halves and this package needs both.

package burlercli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs gitkit.HermeticGitEnv() before any test in this package spawns git, then delegates
// to the normal test run.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
