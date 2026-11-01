// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main, or as the helper process a test re-executes it as.
// It carries no build tag, so it compiles into every tag set.

package verifyrun

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// helperRoleEnv names the role this binary plays instead of running the tests;
// helperPidFileEnv names the file that role writes the tied shell's pid to.
const (
	helperRoleEnv    = "VERIFYRUN_TEST_ROLE"
	helperPidFileEnv = "VERIFYRUN_TEST_PIDFILE"
	roleRunTied      = "run-tied"
)

// TestMain runs the tests under the hermetic git environment and tmuxkit.Main, or the helper role when helperRoleEnv names one.
func TestMain(m *testing.M) {
	if os.Getenv(helperRoleEnv) == roleRunTied {
		os.Exit(runTiedHelper())
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}

// runTiedHelper runs `sleep 30` through RunTied and records the shell's pid in the file helperPidFileEnv names.
func runTiedHelper() int {
	pidFile := os.Getenv(helperPidFileEnv)
	_, err := RunTied(context.Background(), "sleep 30", ".", nil, nil, func(record proc.TreeRecord) {
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(record.PID)), 0o600); err != nil {
			os.Exit(2)
		}
	})
	if err != nil {
		return 3
	}
	return 0
}
