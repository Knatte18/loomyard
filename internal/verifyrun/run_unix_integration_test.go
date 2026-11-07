//go:build !windows

// run_unix_integration_test.go holds the verify-run tests that rest on POSIX process groups and signals.

package verifyrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// expiringContext is a context whose deadline passes when expire is called, reporting context.DeadlineExceeded after it.
// It lets a test expire the context on an event instead of after a fixed time.
type expiringContext struct {
	context.Context
	done chan struct{}
	once sync.Once
}

func newExpiringContext() *expiringContext {
	return &expiringContext{Context: context.Background(), done: make(chan struct{})}
}

func (c *expiringContext) Done() <-chan struct{} {
	return c.done
}

func (c *expiringContext) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (c *expiringContext) expire() {
	c.once.Do(func() { close(c.done) })
}

// TestRun_ExpiryKillsBackgroundedDescendant pins that an expired context ends a descendant that holds the output pipe.
// A descendant that exits on SIGTERM is gone when run returns, having run its trap.
// One that ignores SIGTERM is killed once the wait delay has passed.
// The context expires once the descendant has written its pid, so the expiry is an event and not a fixed pause.
func TestRun_ExpiryKillsBackgroundedDescendant(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const delay = 200 * time.Millisecond
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
			ctx := newExpiringContext()
			defer ctx.expire()
			expired := make(chan time.Time, 1)
			go func() {
				// The expiry fires after the bound too, so a missing pid file fails the test below and does not hang it.
				for until := time.Now().Add(10 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
					if _, err := os.Stat(pidFile); err == nil {
						break
					}
				}
				expired <- time.Now()
				ctx.expire()
			}()

			code, err := run(ctx, command, dir, append(os.Environ(), "MARKER="+marker), &bytes.Buffer{}, delay)
			returned := time.Now()
			if !errors.Is(err, context.DeadlineExceeded) || code != -1 {
				t.Fatalf("run = (%d, %v); want (-1, context.DeadlineExceeded)", code, err)
			}
			if elapsed := returned.Sub(<-expired); !tt.wantGoneOnReturn && elapsed < delay {
				t.Errorf("run took %s after the expiry; want at least the wait delay %s", elapsed, delay)
			}

			pid := waitForPidFile(t, pidFile)
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
