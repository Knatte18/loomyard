package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"
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

func TestProcessTreeReads(t *testing.T) {
	t.Parallel()

	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	childPID := child.Process.Pid

	// Burn about 100 ms of CPU, so the test process's own sample is above zero.
	for deadline := time.Now().Add(100 * time.Millisecond); time.Now().Before(deadline); {
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	find := func(tree []ProcessCPU, pid int) (ProcessCPU, bool) {
		for _, process := range tree {
			if process.PID == pid {
				return process, true
			}
		}
		return ProcessCPU{}, false
	}

	tree, err := TreeCPU(os.Getpid())
	if err != nil {
		t.Fatalf("TreeCPU(own pid): %v", err)
	}
	self, found := find(tree, os.Getpid())
	if !found || self.Path != executable || self.CPU <= 0 {
		t.Errorf("TreeCPU(own pid) test process = %+v, found %v; want Path %q and CPU above zero", self, found, executable)
	}
	if sleeper, found := find(tree, childPID); !found || sleeper.Path == "" {
		t.Errorf("TreeCPU(own pid) child = %+v, found %v; want the child with a non-empty Path", sleeper, found)
	}

	childTree, err := TreeCPU(childPID)
	if err != nil {
		t.Fatalf("TreeCPU(child pid): %v", err)
	}
	if _, found := find(childTree, childPID); !found {
		t.Errorf("TreeCPU(child pid) = %+v; want it to hold the child", childTree)
	}
	if _, found := find(childTree, os.Getpid()); found {
		t.Errorf("TreeCPU(child pid) = %+v; want it not to hold the test process", childTree)
	}

	tests := []struct {
		name string
		pid  int
		want bool
	}{
		{"parent is an ancestor", os.Getppid(), true},
		{"the process itself is not", os.Getpid(), false},
		{"a child is not", childPID, false},
		// Pids above the kernel's pid_max (at most 2^22) never exist.
		{"a pid past the kernel maximum is not", 1 << 30, false},
	}
	for _, tt := range tests {
		if got := IsAncestor(tt.pid); got != tt.want {
			t.Errorf("IsAncestor(%d) (%s) = %v; want %v", tt.pid, tt.name, got, tt.want)
		}
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
