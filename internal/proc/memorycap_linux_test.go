// memorycap_linux_test.go covers what a memory-capped run does on Linux alone.
// A scope reports the kernel's kill, and a host with no user manager fails the child's own allocation.
// A kill at the cap, of the child or of one of its children, ends what is left in the scope.
// The holder that keeps the scope ends with the run and is never a child of the command.
// A scope that never starts, ignores the cap or lacks setsid is an error,
// and a cancel still ends a capped run through its process group.

package proc_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

func TestRunMemoryCappedVia_StartsThroughItsStartAndReportsAChildKilledAtTheCap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	tests := []struct {
		name  string
		start proc.StartFunc
	}{
		{"AStartThatReturnsAtOnce", func(cmd *exec.Cmd) (func() error, error) {
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			return cmd.Wait, nil
		}},
		// A start that returns only once the command has ended stands in for a caller slow to read the scope.
		// The kill must still be seen after the command's own processes are gone.
		{"AStartThatReturnsOnceTheCommandHasEnded", func(cmd *exec.Cmd) (func() error, error) {
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			err := cmd.Wait()
			return func() error { return err }, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The shell runs the hog the cap kills as a plain child and exits 0, so only the tree saw a kill.
			hog := memoryHog(t, hogOverCap)
			cmd := exec.Command("sh", "-c", `"$0"; echo survived`, hog.Path)
			cmd.Env = hog.Env
			var out bytes.Buffer
			cmd.Stdout = &out
			starts := 0
			treeKilled, err := proc.RunMemoryCappedVia(cmd, testMemoryCap, func(cmd *exec.Cmd) (func() error, error) {
				starts++
				return tt.start(cmd)
			})
			if !treeKilled || err != nil || starts != 1 || out.String() != "survived\n" {
				t.Errorf("RunMemoryCappedVia of a shell whose child keeps %d MB under %d = tree killed %v, %v, %d starts, output %q; want a tree kill, no error, one start and survived", hogOverCap, testMemoryCap, treeKilled, err, starts, out.String())
			}
		})
	}
}

func TestRunMemoryCappedVia_ASigkillFromOutsideIsNoKillAtTheCap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	// The shell kills itself with SIGKILL, as a process cap or an operator does, so the kernel's cap kills nothing.
	cmd := exec.Command("sh", "-c", "kill -9 $$")
	treeKilled, err := proc.RunMemoryCappedVia(cmd, testMemoryCap, func(cmd *exec.Cmd) (func() error, error) {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd.Wait, nil
	})
	var exitErr *exec.ExitError
	if treeKilled || !errors.As(err, &exitErr) {
		t.Errorf("RunMemoryCappedVia of a shell that kills itself = tree killed %v, %v; want no tree kill and the shell's *exec.ExitError", treeKilled, err)
	}
}

//testtiming:keep pins that the scope is removed once a capped run with no kill returns, which no covering test checks
func TestRunMemoryCapped_TheHolderEndsOnceACappedRunWithNoKillReturns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	cmd := exec.Command("cat", proc.SelfCgroupFile)
	var out bytes.Buffer
	cmd.Stdout = &out
	if killed, err := proc.RunMemoryCapped(cmd, testMemoryCap); killed || err != nil {
		t.Fatalf("capped run of cat = killed %v, %v; want no kill and no error", killed, err)
	}
	var scopeDir string
	for _, line := range strings.Split(out.String(), "\n") {
		if cgroup, found := strings.CutPrefix(line, "0::"); found {
			scopeDir = filepath.Join(proc.CgroupMount, cgroup)
		}
	}
	if scopeDir == "" {
		t.Fatalf("the capped cat printed %q; want a line for its cgroup v2 path", out.String())
	}
	// The scope is removed once its last process, the holder, has ended.
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(scopeDir); errors.Is(err, os.ErrNotExist) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the scope %s still existed a minute after the capped run returned", scopeDir)
		}
	}
}

func TestRunMemoryCapped_TheHolderIsNeverAChildOfTheCommand(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	if children, _ := filepath.Glob("/proc/self/task/*/children"); len(children) == 0 {
		t.Skip("this kernel does not list a process's children in /proc")
	}

	// The shell becomes cat, which lists the children of its own pid.
	cmd := exec.Command("sh", "-c", "exec cat /proc/self/task/*/children")
	var out bytes.Buffer
	cmd.Stdout = &out
	killed, err := proc.RunMemoryCapped(cmd, testMemoryCap)
	if killed || err != nil || out.Len() != 0 {
		t.Errorf("capped run of a command listing its children = killed %v, %v, output %q; want no kill, no error and no children", killed, err, out.String())
	}
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

func TestRunMemoryCapped_AScopeNotHeldToTheCapIsAnErrorAndNeverRunsTheCommand(t *testing.T) {
	tiergate.Require(t, 1)
	// The stand-in that ignores the cap drops systemd-run's options and runs the rest in the test's own cgroup.
	// That cgroup's memory.max is not the cap, as with a user manager that ignores MemoryMax.
	// It is written before the test goes parallel:
	// a parallel test's fork could otherwise inherit the open write and fail its exec with ETXTBSY.
	ignoringStarter := filepath.Join(t.TempDir(), "systemd-run")
	script := "#!/bin/sh\nwhile [ \"$1\" != -- ]; do shift; done\nshift\nexec \"$@\"\n"
	if err := os.WriteFile(ignoringStarter, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Parallel()

	tests := []struct {
		name string
		// starter is the path of the program that stands in for systemd-run.
		starter string
	}{
		// /bin/false stands in for a systemd-run that cannot create the scope: it exits 1 before it runs the command.
		{"AScopeThatNeverStarts", "/bin/false"},
		{"AScopeThatIgnoresTheCap", ignoringStarter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("echo", "ran")
			var out bytes.Buffer
			cmd.Stdout = &out
			killed, err := proc.RunMemoryCappedWithStarter(cmd, testMemoryCap, tt.starter)
			var exitErr *exec.ExitError
			if !errors.Is(err, proc.ErrMemoryCap) || errors.As(err, &exitErr) || killed || out.Len() != 0 {
				t.Errorf("RunMemoryCapped in a scope not held to the cap = killed %v, %v, output %q; want no kill, no output and an error wrapping proc.ErrMemoryCap, never an *exec.ExitError", killed, err, out.String())
			}
		})
	}
}

//testtiming:keep pins the ErrMemoryCap refusal of a scope held to the cap whose PATH has no setsid, a shell branch coverage cannot see
func TestRunMemoryCapped_AScopeWithoutSetsidIsAnErrorAndNeverRunsTheCommand(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	cmd := exec.Command("echo", "ran")
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	var out bytes.Buffer
	cmd.Stdout = &out
	killed, err := proc.RunMemoryCapped(cmd, testMemoryCap)
	var exitErr *exec.ExitError
	if !errors.Is(err, proc.ErrMemoryCap) || errors.As(err, &exitErr) || killed || out.Len() != 0 {
		t.Errorf("RunMemoryCapped in a scope with no setsid = killed %v, %v, output %q; want no kill, no output and an error wrapping proc.ErrMemoryCap, never an *exec.ExitError", killed, err, out.String())
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
