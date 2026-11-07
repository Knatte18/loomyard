// testmain_test.go runs the test binary under the hermetic git environment and tmux isolation.
// It lets the integration test re-execute the binary as a helper process.
// It carries no build tag, so it compiles into every tag set;
// the helper roles themselves register from the integration-tagged file, since they spawn.

package gatecli

import (
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// helperEnv, when set to a key of helperRoles, turns the test binary into that helper role.
const helperEnv = "GATECLI_TEST_HELPER"

// helperPollEnv and helperLeaseReadEnv are durations that replace two production periods:
// the one a front polls at and the one a runner reads its lease at.
// helperReleaseEnv is a path: the stand-in role waits at its start until a file exists there.
const (
	helperPollEnv      = "GATECLI_TEST_POLL"
	helperLeaseReadEnv = "GATECLI_TEST_LEASE_READ"
	helperReleaseEnv   = "GATECLI_TEST_RELEASE"
)

// releasePollPeriod is how often a held helper looks for its release file.
const releasePollPeriod = 10 * time.Millisecond

// awaitRelease returns once a file exists at path, and at once for an empty path.
func awaitRelease(path string) {
	if path == "" {
		return
	}
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(releasePollPeriod)
	}
}

// helperRoles maps each helper role to the function that runs it and returns the process's exit code.
// It is empty outside the integration tier.
var helperRoles = map[string]func() int{}

// TestMain runs the tests under tmuxkit.Main, or runs the helper role helperEnv names.
func TestMain(m *testing.M) {
	if role, ok := helperRoles[os.Getenv(helperEnv)]; ok {
		if os.Getenv(helperEnv) == helperRoleStandIn {
			awaitRelease(os.Getenv(helperReleaseEnv))
		}
		os.Exit(role())
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
