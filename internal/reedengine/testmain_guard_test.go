// testmain_guard_test.go pins the TestMain guard that refuses a positional argument.
// A tmux job that names the test binary would otherwise run the whole suite again and pin the same job in each session.

package reedengine

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// guardChildEnv marks the test binary this test starts, which skips the test so that a missing guard cannot recurse.
const guardChildEnv = "REEDENGINE_TESTMAIN_GUARD_CHILD"

func TestTestMain_ExitsTwoWithoutRunningTheSuiteOnAPositionalArgument(t *testing.T) {
	tiergate.Require(t, 1)
	if os.Getenv(guardChildEnv) != "" {
		t.Skip("the child of the guard test never starts another")
	}
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(executable, "slack-bar-state")
	cmd.Env = append(os.Environ(), guardChildEnv+"=1")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("test binary with a positional argument: error = %v; want exit status 2", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("test binary with a positional argument wrote stdout %q; want none, as no test ran", stdout.String())
	}
	if !strings.Contains(stderr.String(), "refusing the non-test arguments") {
		t.Errorf("test binary with a positional argument wrote stderr %q; want the refusal", stderr.String())
	}
}
