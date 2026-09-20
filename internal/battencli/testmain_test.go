//go:build !integration

// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the
// operator's global gitconfig (see CONSTRAINTS.md's Hermetic Git Test Environment Invariant).
//
// The `!integration` constraint is load-bearing, not decorative: testmain_integration_test.go
// declares its own TestMain for the same package, and without this negation both files would
// compile together under `go test -tags integration` (an untagged file always compiles regardless
// of which tags are set) and collide as two definitions of TestMain in one package. This file's
// own suite stays untagged and Tier 1 in every other respect -- none of its sibling test files call
// the resolver, run git, or spawn a process -- only the TestMain wiring itself needs this one file
// excluded once the integration-tagged sibling supplies its own.

package battencli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMain runs the hermetic git setup once before the whole suite.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(m.Run())
}
