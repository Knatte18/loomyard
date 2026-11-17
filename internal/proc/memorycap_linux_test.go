// memorycap_linux_test.go covers what a memory-capped run does on Linux alone.
// A scope reports the kernel's kill, and a host with no user manager fails the child's own allocation.
// A kill at the cap, of the child or of one of its children, ends what is left in the scope.
// A scope that never starts is an error, and a cancel still ends a capped run through its process group.

package proc_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunMemoryCapped_EndsAChildOverTheCapByTheHostsMeans(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		run  func(cmd *exec.Cmd, limit int64) (bool, error)
		// needsScope is set for a row that runs only where the user manager can cap a scope's memory.
		needsScope bool
		wantKilled bool
	}{
		{"AScopeReportsTheKernelsKill", proc.RunMemoryCapped, true, true},
		{"WithNoUserManagerTheChildFailsItsOwnAllocation", proc.RunMemoryCappedWithoutScope, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.needsScope && !proc.MemoryCapCoversTree() {
				t.Skip("this host has no systemd user manager that caps a scope's memory")
			}
			killed, err := tt.run(memoryHog(t, hogOverCap), testMemoryCap)
			if _, isExit := err.(*exec.ExitError); !isExit || killed != tt.wantKilled {
				t.Errorf("capped run of a child keeping %d MB under %d = killed %v, %T %v; want killed %v and exactly an *exec.ExitError", hogOverCap, testMemoryCap, killed, err, err, tt.wantKilled)
			}
		})
	}
}

func TestRunMemoryCapped_AKillAtTheCapEndsWhatTheChildLeftInItsScope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	// The shell starts a sleep that outlasts the test, prints its pid and becomes the hog the cap kills.
	hog := memoryHog(t, hogOverCap)
	cmd := exec.Command("sh", "-c", `sleep 300 >/dev/null 2>&1 & echo $!; exec "$0"`, hog.Path)
	cmd.Env = hog.Env
	var out bytes.Buffer
	cmd.Stdout = &out
	killed, err := proc.RunMemoryCapped(cmd, testMemoryCap)
	if !killed || err == nil {
		t.Fatalf("capped run of a child keeping %d MB under %d = killed %v, %v; want killed and an error", hogOverCap, testMemoryCap, killed, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil {
		t.Fatalf("the capped shell printed %q; want the sleep's pid", out.String())
	}
	requireProcessGone(t, pid)
}

func TestRunMemoryCapped_AKillOfAChildLeavesTheCommandRunningAndEndsItsScope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	// The shell starts a sleep that outlasts the test, runs the hog the cap kills as a plain child, and exits 0.
	// Only a scope that continues after the kill lets the shell print its last line.
	hog := memoryHog(t, hogOverCap)
	cmd := exec.Command("sh", "-c", `sleep 300 >/dev/null 2>&1 & echo $!; "$0"; echo survived`, hog.Path)
	cmd.Env = hog.Env
	var out bytes.Buffer
	cmd.Stdout = &out
	killed, err := proc.RunMemoryCapped(cmd, testMemoryCap)
	if killed || err != nil {
		t.Fatalf("capped run of a shell whose child keeps %d MB under %d = killed %v, %v; want no kill and no error", hogOverCap, testMemoryCap, killed, err)
	}
	printedPid, rest, found := strings.Cut(out.String(), "\n")
	if !found || rest != "survived\n" {
		t.Fatalf("the capped shell printed %q; want the sleep's pid and then survived", out.String())
	}
	pid, err := strconv.Atoi(printedPid)
	if err != nil {
		t.Fatalf("the capped shell printed %q first; want the sleep's pid", printedPid)
	}
	requireProcessGone(t, pid)
}

// requireProcessGone fails the test when the process pid is still present a minute from now.
func requireProcessGone(t *testing.T, pid int) {
	t.Helper()
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); errors.Is(err, os.ErrNotExist) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the sleep %d the capped command started was still running a minute after the kill", pid)
		}
	}
}

func TestRunMemoryCapped_AScopeThatNeverStartsIsAnErrorAndNotTheCommandsExit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// /bin/false stands in for a systemd-run that cannot create the scope: it exits 1 before it runs the command.
	killed, err := proc.RunMemoryCappedWithStarter(exec.Command("echo", "never runs"), testMemoryCap, "/bin/false")
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) || killed {
		t.Errorf("RunMemoryCapped with a starter that fails at once = killed %v, %v; want no kill and an error that is no *exec.ExitError", killed, err)
	}
}

func TestRunMemoryCapped_ACancelEndsTheRunAndIsNoCapKill(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The shell becomes a sleep that outlasts the minute the test allows, so only the cancel ends the run.
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo started; exec sleep 300")
	proc.ConfigureGroupKill(cmd, func(int, error) {})
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	type outcome struct {
		killed bool
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		killed, err := proc.RunMemoryCapped(cmd, testMemoryCap)
		writer.Close()
		done <- outcome{killed, err}
	}()
	if line, err := bufio.NewReader(reader).ReadString('\n'); err != nil || line != "started\n" {
		t.Fatalf("the capped command printed %q, %v; want its started line", line, err)
	}

	cancel()
	select {
	case got := <-done:
		if got.killed || got.err == nil {
			t.Errorf("RunMemoryCapped after a cancel = killed %v, %v; want no cap kill and the cancelled command's error", got.killed, got.err)
		}
	case <-time.After(time.Minute):
		t.Fatal("the capped command was still running a minute after its cancel")
	}
}
