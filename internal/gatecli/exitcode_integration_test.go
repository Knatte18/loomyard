//go:build !windows

// exitcode_integration_test.go covers the exit codes exitCodeOf and tierExitCodeOf read from real child processes ended by a signal or by an exit.

package gatecli

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestExitCodeOf(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name   string
		script string
		// wantTier is the code of a tier command, which reads a crash signal as 128 plus its number; wantPlain is the code of any other child, which reads every signal as 1.
		wantTier  int
		wantPlain int
	}{
		{"Exited", "exit 7", 7, 7},
		{"Passed", "exit 0", 0, 0},
		{"SegmentationFault", "kill -SEGV $$", 128 + int(syscall.SIGSEGV), 1},
		{"Aborted", "kill -ABRT $$", 128 + int(syscall.SIGABRT), 1},
		{"KilledIsNoCrash", "kill -KILL $$", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := exec.Command("sh", "-c", tt.script).Run()
			if got := tierExitCodeOf(err); got != tt.wantTier {
				t.Errorf("tierExitCodeOf(%q) = %d; want %d", tt.script, got, tt.wantTier)
			}
			if got := exitCodeOf(err); got != tt.wantPlain {
				t.Errorf("exitCodeOf(%q) = %d; want %d", tt.script, got, tt.wantPlain)
			}
		})
	}

	t.Run("NeverStarted", func(t *testing.T) {
		t.Parallel()
		err := exec.Command("/nonexistent/lym-gate-test-shell").Run()
		if got := tierExitCodeOf(err); got != 1 {
			t.Errorf("tierExitCodeOf(a command that cannot start) = %d; want 1", got)
		}
		if got := exitCodeOf(err); got != 1 {
			t.Errorf("exitCodeOf(a command that cannot start) = %d; want 1", got)
		}
	})
}
