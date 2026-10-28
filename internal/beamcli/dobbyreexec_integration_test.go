//go:build tier1

// dobbyreexec_integration_test.go runs a real `lym beam dobby` through a re-exec onto a replaced binary and its stop, over a hubforge hub: the acceptance test of the dobby's lock, log and output file.

package beamcli_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// reexecDeadline bounds the wait for a re-exec: where the file watch does not open the loop ticks at most once a minute, and the re-exec needs two sightings of the replaced binary.
const reexecDeadline = 3 * time.Minute

// waitUntil polls cond every 50 ms until it holds or the deadline passes, and reports whether it held.
func waitUntil(deadline time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// copyExecutable copies the file at src to dst as an executable.
func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v; want nil", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", dst, err)
	}
}

// startRetryingBusy starts the command newCmd builds, and builds and starts a fresh one while the start fails with ETXTBSY, for up to ten seconds.
// A process a parallel test forks can hold the just-written binary open for writing until it execs, which fails this exec with "text file busy".
func startRetryingBusy(t *testing.T, newCmd func() *exec.Cmd) *exec.Cmd {
	t.Helper()
	var cmd *exec.Cmd
	var err error
	waitUntil(10*time.Second, func() bool {
		cmd = newCmd()
		err = cmd.Start()
		return !errors.Is(err, syscall.ETXTBSY)
	})
	if err != nil {
		t.Fatalf("start %s = %v; want nil", cmd.Path, err)
	}
	return cmd
}

// fileText returns the text of the file at path, empty when it does not exist.
func fileText(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
