// procgroup_linux_test.go covers the group-terminate branches that end without arming a SIGKILL timer.
// It spawns real children, which this package's Test Tier Purity allowlist entry admits: process control is the package's subject.

package proc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestConfigurePlatformGroupTerminate_ArmsNoTimerWithoutALiveGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// run starts the command and drives its cancel, returning the cancel's error.
		run func(t *testing.T, cmd *exec.Cmd) error
		// wantReports is the signals the report callback sees.
		wantReports []os.Signal
		wantErr     error
	}{
		{"a cancel that never ran", func(t *testing.T, cmd *exec.Cmd) error {
			if err := cmd.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			})
			return nil
		}, nil, nil},
		{"a cancel after the group is gone", func(t *testing.T, cmd *exec.Cmd) error {
			if err := cmd.Run(); err != nil {
				t.Fatalf("Run: %v", err)
			}
			return cmd.Cancel()
		}, []os.Signal{syscall.SIGTERM}, syscall.ESRCH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(context.Background(), "sleep", "30")
			if tt.wantErr != nil {
				cmd = exec.CommandContext(context.Background(), "true")
			}
			var reported []os.Signal
			awaitGone := configurePlatformGroupTerminate(cmd, time.Hour, func(_ int, sig os.Signal, _ error) {
				reported = append(reported, sig)
			})

			err := tt.run(t, cmd)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("cancel error = %v; want %v", err, tt.wantErr)
			}
			if len(reported) != len(tt.wantReports) || (len(reported) == 1 && reported[0] != tt.wantReports[0]) {
				t.Errorf("reported signals = %v; want %v", reported, tt.wantReports)
			}
			returned := make(chan struct{})
			go func() {
				awaitGone()
				close(returned)
			}()
			select {
			case <-returned:
			case <-time.After(5 * time.Second):
				t.Errorf("awaitGone did not return at once")
			}
		})
	}
}
