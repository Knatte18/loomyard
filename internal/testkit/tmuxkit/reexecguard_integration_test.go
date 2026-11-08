// reexecguard_integration_test.go pins that a test binary started with a lym verb exits at Main without running a test.
// Every package's TestMain calls Main, so this one binary proves the guard for them all.

package tmuxkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// guardChildEnv marks the test binary this test starts, which skips the test so that a missing guard cannot recurse.
const guardChildEnv = "TMUXKIT_REEXEC_GUARD_CHILD"

func TestMain_ExitsTwoWithoutRunningTheSuiteOnAPositionalArgument(t *testing.T) {
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
	cmd := exec.Command(executable, "slack", "bar")
	childTemp := t.TempDir()
	cmd.Env = append(os.Environ(), guardChildEnv+"=1", "TMPDIR="+childTemp)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != refusedReexecExit {
		t.Fatalf("test binary started with `slack bar`: error = %v; want exit status %d", err, refusedReexecExit)
	}
	if stdout.Len() != 0 {
		t.Errorf("test binary started with `slack bar` wrote stdout %q; want none, as no test ran", stdout.String())
	}
	if !strings.Contains(stderr.String(), `refusing the non-test arguments ["slack" "bar"]`) {
		t.Errorf("test binary started with `slack bar` wrote stderr %q; want the refusal naming both words", stderr.String())
	}
	roots, err := os.ReadDir(childTemp)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", childTemp, err)
	}
	for _, root := range roots {
		left, err := os.ReadDir(filepath.Join(childTemp, root.Name()))
		if err != nil || len(left) != 0 {
			t.Errorf("refused test binary left %v in %s (read error %v); want its run directory removed", left, root.Name(), err)
		}
	}
}
