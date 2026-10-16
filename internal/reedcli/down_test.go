// down_test.go pins the wait `reed down` makes for the hub's watchdog lock, through the fake engine seam and an injected probe and sleep so no test waits out the production bound.

package reedcli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestDownCmd_WaitsForTheWatchdogOnlyWhenItEndedTheServer drives downCmd over a fake engine and asserts the envelope, the number of lock probes and the single Warn a timed-out wait logs.
// It captures the process-global logger, so it is not parallel.
func TestDownCmd_WaitsForTheWatchdogOnlyWhenItEndedTheServer(t *testing.T) {
	const poll = 100 * time.Millisecond
	tests := []struct {
		name string
		down reedengine.DownResult
		// downErr is what Down returns.
		downErr error
		// scratchDir has the hub's scratch directory created, so the real probe can take the lock.
		scratchDir bool
		// free scripts the probe's answers by call number, true meaning the lock is free, and its last answer repeats; nil runs the real probe.
		free         []bool
		wantErr      string
		wantProbes   int
		wantSleeps   int
		wantWarns    int
		wantLockFree bool
	}{
		{
			name:         "ended the server, real probe over a free lock",
			down:         reedengine.DownResult{Session: "alpha", EndedServer: true},
			scratchDir:   true,
			wantProbes:   1,
			wantLockFree: true,
		},
		{
			name:       "ended the server, real probe with no scratch directory",
			down:       reedengine.DownResult{Session: "alpha", EndedServer: true},
			wantProbes: 1,
		},
		{
			name:       "ended the server, lock held twice then free",
			down:       reedengine.DownResult{Session: "alpha", EndedServer: true},
			free:       []bool{false, false, true},
			wantProbes: 3,
			wantSleeps: 2,
		},
		{
			name:       "ended the server, lock always held",
			down:       reedengine.DownResult{Session: "alpha", EndedServer: true},
			free:       []bool{false},
			wantProbes: 11,
			wantSleeps: 10,
			wantWarns:  1,
		},
		{
			name: "left a sibling",
			down: reedengine.DownResult{Session: "alpha"},
			free: []bool{false},
		},
		{
			name:    "Down error",
			downErr: errors.New("tmux unavailable"),
			free:    []bool{false},
			wantErr: "tmux unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			hub := t.TempDir()
			scratch := fabricengine.HubScratchDir(hub)
			if tt.scratchDir {
				if err := os.MkdirAll(scratch, 0o755); err != nil {
					t.Fatalf("mkdir scratch: %v", err)
				}
			}
			lockPath := filepath.Join(scratch, watchdogLockFileName)

			probes, sleeps := 0, 0
			c := &reedCLI{
				strands:      &fakeStrandOps{down: tt.down, downErr: tt.downErr},
				hubPath:      hub,
				downLockWait: time.Second,
				downLockPoll: poll,
				downLockProbe: func(path string) (bool, error) {
					probes++
					if tt.free == nil {
						return probeWatchdogLock(path)
					}
					return tt.free[min(probes, len(tt.free))-1], nil
				},
				downLockSleep: func(d time.Duration) {
					if d != poll {
						t.Errorf("sleep = %v, want %v", d, poll)
					}
					sleeps++
				},
			}
			cmd := c.downCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if tt.wantErr != "" {
				envelope.RequireErr(t, out.String(), tt.wantErr)
			} else if env := envelope.RequireOK(t, out.String()); env.Raw["session"] != tt.down.Session {
				t.Errorf("envelope session = %v, want %q", env.Raw["session"], tt.down.Session)
			}
			if probes != tt.wantProbes {
				t.Errorf("probes = %d, want %d", probes, tt.wantProbes)
			}
			if sleeps != tt.wantSleeps {
				t.Errorf("sleeps = %d, want %d", sleeps, tt.wantSleeps)
			}
			warns := strings.Count(logs.String(), "gave up waiting for the watchdog")
			if warns != tt.wantWarns {
				t.Errorf("Warn lines = %d, want %d; log = %q", warns, tt.wantWarns, logs.String())
			}
			if tt.wantWarns > 0 && (!strings.Contains(logs.String(), hub) || !strings.Contains(logs.String(), lockPath)) {
				t.Errorf("Warn = %q, want it to name hub %q and lock path %q", logs.String(), hub, lockPath)
			}
			if tt.wantLockFree {
				held, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err != nil || !acquired {
					t.Fatalf("lock after the probe: acquired=%v err=%v; want the probe to have released it", acquired, err)
				}
				held.Release()
			}
		})
	}
}
