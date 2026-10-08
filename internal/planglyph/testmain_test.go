// testmain_test.go wires the package's test binary into the hermetic git test environment:
// gitkit.HermeticGitEnv() runs once before any test, ahead of the integration-tagged tests batch 7
// adds to this package that spawn git via the (*quarry.Repo) git-delta query, per
// PATTERN-test-isolation. This batch's own tests spawn no process — they
// build a fixture repository under t.TempDir() and call quarry.Open/Resolve, which read files only
// — but the wiring is in place ahead of the first test that needs it.

package planglyph

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"golang.org/x/tools/go/packages"
)

// failingTypesLoader is the untagged tier's typesLoader: it fails without spawning, so every plan gate answers caller-uncovered from its scan.
type failingTypesLoader struct{}

func (failingTypesLoader) load(string, []string) ([]*packages.Package, error) {
	return nil, errors.New("type load disabled in the untagged tier")
}

// TestMain runs hermetic git environment setup before tests, then runs them under tmuxkit.Main.
// It replaces defaultTypesLoader, process-global state, before any test runs, so an untagged test never spawns go list.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	defaultTypesLoader = failingTypesLoader{}
	os.Exit(tmuxkit.Main(m))
}
