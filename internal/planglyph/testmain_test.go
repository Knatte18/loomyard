// testmain_test.go sets up the planglyph test binary: the hermetic git environment and the untagged types loader.

package planglyph

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"golang.org/x/tools/go/packages"
)

// failingTypesLoader is the types loader of the untagged tier:
// it fails without spawning, so every plan gate answers caller-uncovered from its scan.
type failingTypesLoader struct{}

func (failingTypesLoader) load(string, []string) ([]*packages.Package, error) {
	return nil, errors.New("type load disabled in the untagged tier")
}

// TestMain sets up the hermetic git environment, then runs the tests under tmuxkit.Main.
// It replaces defaultTypesLoader, which is process-global, before any test runs,
// so an untagged test never spawns go list.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	defaultTypesLoader = failingTypesLoader{}
	os.Exit(tmuxkit.Main(m))
}
