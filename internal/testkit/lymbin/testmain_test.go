// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main.
// It has no build tag, so it compiles into every tag set; the integration test re-executes it as a helper process.
// The helper roles register from the integration-tagged file, since they spawn.

package lymbin

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// helperEnv, when set to a key of helperRoles, turns the test binary into that helper role.
const helperEnv = "LYMBIN_TEST_HELPER"

// helperRoles maps each helper role to the function that runs it and returns the process's exit code.
// It is empty outside the integration tier.
var helperRoles = map[string]func() int{}

// TestMain runs the tests under tmuxkit.Main, or runs the helper role helperEnv names.
func TestMain(m *testing.M) {
	if role, ok := helperRoles[os.Getenv(helperEnv)]; ok {
		os.Exit(role())
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
