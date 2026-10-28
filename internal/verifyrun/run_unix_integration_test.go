//go:build tier1 && !windows

// run_unix_integration_test.go holds the verify-run tests that rest on POSIX process groups and signals.

package verifyrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRun_ExpiryKillsBackgroundedDescendant pins that an expired context terminates a descendant of the verify shell that holds the output pipe, not only the shell:
// a descendant that exits on SIGTERM is gone when run returns, having run its trap.
// One that ignores SIGTERM is killed once the wait delay has passed.
func TestRun_ExpiryKillsBackgroundedDescendant(t *testing.T) {
	t.Parallel()

	const (
		expiry = time.Second
		delay  = 200 * time.Millisecond
	)
	tests := []struct {
		name             string
		trap             string
		wantMarker       bool
		wantGoneOnReturn bool
	}{
		{"a descendant exiting on SIGTERM is gone when run returns", `trap "echo term > $MARKER; exit 0" TERM`, true, true},
		{"a descendant ignoring SIGTERM is killed after the delay", `trap "" TERM`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pidFile := filepath.Join(dir, "descendant.pid")
			marker := filepath.Join(dir, "term.marker")
			command := "sh -c '" + tt.trap + "; echo $$ > " + pidFile + "; sleep 60 & wait' & wait"
			ctx, cancel := context.WithTimeout(context.Background(), expiry)
			defer cancel()

			start := time.Now()
			code, err := run(ctx, command, dir, append(os.Environ(), "MARKER="+marker), &bytes.Buffer{}, delay)
			elapsed := time.Since(start)
			if !errors.Is(err, context.DeadlineExceeded) || code != -1 {
				t.Fatalf("run = (%d, %v); want (-1, context.DeadlineExceeded)", code, err)
			}
			if !tt.wantGoneOnReturn && elapsed < expiry+delay {
				t.Errorf("run took %s; want at least the expiry plus the wait delay %s", elapsed, expiry+delay)
			}

			raw, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				if tt.wantGoneOnReturn || time.Now().After(deadline) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
					t.Fatalf("descendant %d still alive after run returned", pid)
				}
				time.Sleep(20 * time.Millisecond)
			}
			_, markerErr := os.Stat(marker)
			if tt.wantMarker != (markerErr == nil) {
				t.Errorf("trap marker present = %v; want %v", markerErr == nil, tt.wantMarker)
			}
		})
	}
}
