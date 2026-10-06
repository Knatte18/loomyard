package proc

import (
	"os"
	"os/exec"
	"testing"
)

func TestStartTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pid  int
		want bool
	}{
		{"own process has a start time", os.Getpid(), true},
		// Pids above the kernel's pid_max (at most 2^22) never exist.
		{"missing process has none", 1 << 30, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := StartTime(tt.pid)
			if ok != tt.want || (ok && got == "") {
				t.Errorf("StartTime(%d) = %q, %v; want ok=%v with a non-empty value when ok", tt.pid, got, ok, tt.want)
			}
		})
	}
}

func TestHideWindowIsNoop(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	HideWindow(cmd)

	if cmd.SysProcAttr != nil {
		t.Errorf("HideWindow(cmd) set SysProcAttr; want nil")
	}
}

func TestDetachSetsSetsid(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	Detach(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("cmd.SysProcAttr is nil")
	}
	if cmd.SysProcAttr.Setsid != true {
		t.Errorf("Detach(cmd) Setsid = %v; want true", cmd.SysProcAttr.Setsid)
	}
}
