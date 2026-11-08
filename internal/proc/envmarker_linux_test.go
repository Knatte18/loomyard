// envmarker_linux_test.go pins that PIDsWithEnv finds a process by a whole entry of its environment.

package proc

import (
	"os"
	"os/exec"
	"slices"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPIDsWithEnv_FindsTheProcessCarryingTheEntry(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	entry := "PROC_ENVMARKER_TEST=" + strconv.Itoa(os.Getpid())
	marked := exec.Command("sleep", "60")
	marked.Env = append(os.Environ(), entry)
	unmarked := exec.Command("sleep", "60")
	unmarked.Env = append(os.Environ(), entry+"0")
	for _, cmd := range []*exec.Cmd{marked, unmarked} {
		if err := cmd.Start(); err != nil {
			t.Fatalf("start sleep: %v", err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}

	pids, err := PIDsWithEnv(entry)

	if err != nil {
		t.Fatalf("PIDsWithEnv(%q) error = %v", entry, err)
	}
	if want := []int{marked.Process.Pid}; !slices.Equal(pids, want) {
		t.Errorf("PIDsWithEnv(%q) = %v; want %v, the one process whose entry matches whole", entry, pids, want)
	}
}
