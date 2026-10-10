// tied_test.go covers the platform-independent behavior of StartTied and IsBreakawayRefused.
// It spawns a real child, which this package's Test Tier Purity allowlist entry admits: process control is the package's subject.

package proc

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestStartTiedRecordsChildInItsOwnGroup(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(os.Args[0], "-test.run=^$")
	tied, err := StartTied(cmd)
	if err != nil {
		t.Fatalf("StartTied: %v", err)
	}
	record := tied.Record()
	if err := tied.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if record.PID != cmd.Process.Pid || record.PGID != record.PID {
		t.Errorf("Record() = %+v; want pid %d and a group id equal to it", record, cmd.Process.Pid)
	}
}

func TestIsBreakawayRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"ordinary error", errors.New("boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if IsBreakawayRefused(tt.err) {
				t.Errorf("IsBreakawayRefused(%v) = true; want false", tt.err)
			}
		})
	}
}
