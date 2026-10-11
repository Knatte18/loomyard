// proc_test.go checks the kit's stat and comm probes against the test's own process.

package tmuxkit

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// commMaxBytes is the kernel's bound on a command name read from /proc/<pid>/comm.
const commMaxBytes = 15

// TestProcProbes_OwnProcess checks the parent pid, state and comm probes against the running test process, and that each answers false for a pid above the kernel's range.
func TestProcProbes_OwnProcess(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("/proc probes exist on linux only")
	}
	const absentPID = 1 << 30
	pid := os.Getpid()

	if ppid, ok := ProcPPID(pid); !ok || ppid != os.Getppid() {
		t.Errorf("ProcPPID(%d) = %d, %v; want %d, true", pid, ppid, ok, os.Getppid())
	}
	if state, ok := ProcState(pid); !ok || !slices.Contains([]string{"R", "S", "D"}, state) {
		t.Errorf("ProcState(%d) = %q, %v; want R, S or D and true", pid, state, ok)
	}
	wantComm := filepath.Base(os.Args[0])
	if len(wantComm) > commMaxBytes {
		wantComm = wantComm[:commMaxBytes]
	}
	if comm, ok := ProcComm(pid); !ok || comm != wantComm {
		t.Errorf("ProcComm(%d) = %q, %v; want %q, true", pid, comm, ok, wantComm)
	}

	if ppid, ok := ProcPPID(absentPID); ok {
		t.Errorf("ProcPPID(%d) = %d, true; want false", absentPID, ppid)
	}
	if state, ok := ProcState(absentPID); ok {
		t.Errorf("ProcState(%d) = %q, true; want false", absentPID, state)
	}
	if comm, ok := ProcComm(absentPID); ok {
		t.Errorf("ProcComm(%d) = %q, true; want false", absentPID, comm)
	}
}
