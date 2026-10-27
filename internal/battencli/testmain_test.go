//go:build !tier1

// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, so this package's tests never inherit the
// operator's global gitconfig (see PATTERN-test-isolation).
// The binary also runs under tmux isolation through tmuxkit.Main.
//
// The `!tier1` constraint is load-bearing, not decorative: testmain_integration_test.go
// declares its own TestMain for the same package, and without this negation both files would
// compile together under `go test -tags tier1` and collide as two definitions of TestMain in one package.
// None of this package's tier0 test files call the resolver, run git, or spawn a process.
// Only the TestMain wiring itself needs this one file excluded once the tier1 sibling supplies its own.

package battencli

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the hermetic git setup once before the whole suite, then runs it under tmuxkit.Main.
// A pair push is refused rather than detached, which would fork this test binary; no test of this suite has a pair to push.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	spawnPairPush = func(*lymcwd.Location) error { return errors.New("battencli: unit tests push no pair") }
	os.Exit(tmuxkit.Main(m))
}
