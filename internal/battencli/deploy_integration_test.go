//go:build integration

// deploy_integration_test.go covers runDeploy against a real shell, which is why it sits in the integration tier.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRunDeploy_WritesTheLogAndReportsExitCodeAndTimeout asserts runDeploy writes the command's output to the log, reports a non-zero exit code, and reports TimedOut for a command that outlives a short timeout.
func TestRunDeploy_WritesTheLogAndReportsExitCodeAndTimeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the commands below are written for sh")
	}
	tests := []struct {
		name         string
		command      string
		timeout      time.Duration
		wantExitCode int
		wantTimedOut bool
		wantLog      string
	}{
		{name: "OutputGoesToTheLog", command: "echo deployed", timeout: time.Minute, wantLog: "deployed"},
		{name: "NonZeroExitIsReported", command: "echo failing; exit 3", timeout: time.Minute, wantExitCode: 3, wantLog: "failing"},
		{name: "CommandOutlivingTheTimeoutIsTimedOut", command: "sleep 30", timeout: 200 * time.Millisecond, wantTimedOut: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			logPath := filepath.Join(dir, "deploy.log")

			result, err := runDeploy(context.Background(), dir, tt.command, logPath, tt.timeout)
			if err != nil {
				t.Fatalf("runDeploy() error = %v; want nil", err)
			}
			if result.ExitCode != tt.wantExitCode || result.TimedOut != tt.wantTimedOut {
				t.Errorf("runDeploy() = %+v; want exit code %d, timed out %v", result, tt.wantExitCode, tt.wantTimedOut)
			}
			logged, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read the deploy log: %v", err)
			}
			if !strings.Contains(string(logged), tt.wantLog) {
				t.Errorf("deploy log = %q; want it to contain %q", logged, tt.wantLog)
			}
		})
	}
}
