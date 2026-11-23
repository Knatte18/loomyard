// processscope_linux_test.go covers the scope cap of a running process against a real systemd user manager.
// A capped process keeps running when the kernel kills a runaway child at the cap, and its scope counts the kill.

package proc_test

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCapProcessMemory_KillsARunawayChildInTheScopeAndTheProcessLivesOn(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	// The shell waits for a line, runs the hog as a plain child, says it lives on, and waits for its stdin to close.
	hog := memoryHog(t, hogOverCap)
	cmd := exec.Command("sh", "-c", `read -r line; "$0"; echo survived; read -r line`, hog.Path)
	cmd.Env = hog.Env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	pid := cmd.Process.Pid
	unit := "lym-proc-test-" + strconv.Itoa(pid) + ".scope"

	cgroup, err := proc.CapProcessMemory(pid, unit, testMemoryCap)
	if err != nil {
		t.Fatalf("CapProcessMemory(%d, %q, %d) = %v; want nil", pid, unit, testMemoryCap, err)
	}
	if path.Base(cgroup) != unit {
		t.Errorf("CapProcessMemory(%d, %q, %d) cgroup = %q; want one ending in the unit", pid, unit, testMemoryCap, cgroup)
	}
	held, err := os.ReadFile(filepath.Join(proc.CgroupMount, filepath.FromSlash(cgroup), "memory.max"))
	if got := strings.TrimSpace(string(held)); err != nil || got != strconv.Itoa(testMemoryCap) {
		t.Errorf("the scope's memory.max = %q, %v; want %d", got, err, testMemoryCap)
	}
	// A host that does not account swap has no memory.swap.max to read.
	swap, err := os.ReadFile(filepath.Join(proc.CgroupMount, filepath.FromSlash(cgroup), "memory.swap.max"))
	if got := strings.TrimSpace(string(swap)); !errors.Is(err, fs.ErrNotExist) && (err != nil || got != "0") {
		t.Errorf("the scope's memory.swap.max = %q, %v; want 0", got, err)
	}

	if _, err := io.WriteString(stdin, "run\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "survived\n" {
		t.Fatalf("the capped shell printed %q, %v; want survived", line, err)
	}
	gotCgroup, kills, err := proc.ProcessOOMKills(pid)
	if err != nil || gotCgroup != cgroup || kills != 1 {
		t.Errorf("ProcessOOMKills(%d) = %q, %d, %v; want %q, 1, nil", pid, gotCgroup, kills, err, cgroup)
	}

	// The journal may not hold the kernel's line yet, or may be closed to this user, so it may name no kill.
	// Any kill it names in the fresh scope is the hog's, under the kernel's command name of at most 15 bytes.
	wantName := filepath.Base(hog.Path)
	if len(wantName) > 15 {
		wantName = wantName[:15]
	}
	named, err := proc.OOMKillsInCgroup(cgroup)
	if err != nil {
		t.Fatalf("OOMKillsInCgroup(%q) = %v; want nil", cgroup, err)
	}
	for _, kill := range named {
		if kill.Name != wantName || kill.PID <= 0 {
			t.Errorf("OOMKillsInCgroup(%q) names %+v; want only kills of %q", cgroup, kill, wantName)
		}
	}
}

func TestCapProcessMemory_AnEndedProcessIsACapError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	ended := exec.Command("true")
	if err := ended.Run(); err != nil {
		t.Fatal(err)
	}
	pid := ended.Process.Pid
	unit := "lym-proc-test-" + strconv.Itoa(pid) + ".scope"
	// The user manager refuses a pid that no longer exists, so the error comes from the refused scope.
	if _, err := proc.CapProcessMemory(pid, unit, testMemoryCap); !errors.Is(err, proc.ErrMemoryCap) || !strings.Contains(err.Error(), "refused the scope") {
		t.Errorf("CapProcessMemory(%d, %q, %d) of an ended process = %v; want an error wrapping ErrMemoryCap that names the refused scope", pid, unit, testMemoryCap, err)
	}
}
