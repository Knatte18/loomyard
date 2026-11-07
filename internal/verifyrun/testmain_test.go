// testmain_test.go runs the test binary under tmuxkit.Main, or as the helper process a test re-executes it as.
// It serves every tier's tests, since the package's one test binary holds them all.

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

func TestMain(m *testing.M) {
	if os.Getenv(helperRoleEnv) == roleRunTied {
		os.Exit(runTiedHelper())
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}

// runTiedHelper is the helper role a test re-executes this binary as.
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
