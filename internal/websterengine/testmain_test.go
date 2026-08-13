// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so websterengine's git-spawning fixtures
// (begin-batch's HeadSHA capture, record-batch's ChangedFiles/Dirty, chain rollback's ResetHard)
// never inherit the operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment
// Invariant).
// Every untagged test in this package still spawns nothing;
// only the //go:build integration-tagged files ever reach git,
// but TestMain must run for the whole test binary regardless of which tests are compiled in.

package websterengine

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
