//go:build !windows

// lymbin_unix_integration_test.go holds the lymbin timeout test whose sleeper's child is the Unix sleep command.

package lymbin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRun_TimesOutNamingTheTimeout passes whether or not the sleeper starts within the timeout, since either way it outlives it.
func TestRun_TimesOutNamingTheTimeout(t *testing.T) {
	tiergate.Require(t, 1)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperEnv, helperRoleSleeper)
	t.Setenv(helperPidFileEnv, filepath.Join(t.TempDir(), "pids"))

	out, code, err := Run(self, t.TempDir(), 200*time.Millisecond)

	if !errors.Is(err, ErrTimeout) || code != -1 || !strings.Contains(err.Error(), "after 200ms") {
		t.Errorf("Run = (%q, %d, %v); want ErrTimeout naming 200ms and exit code -1", out, code, err)
	}
}
