// memorycap_linux.go caps a command's memory with a transient scope of the systemd user manager.
// It ends what is left in the scope once the kernel kills a process in it at the cap.
// On a host whose user manager cannot cap a scope's memory, it limits the data segment of each process instead.

package proc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// systemdRun names the command that starts a transient scope.
const systemdRun = "systemd-run"

// memoryController is the cgroup controller a scope's memory limit needs.
const memoryController = "memory"

// probeMemoryCap is the memory limit of the scope the probe starts.
const probeMemoryCap = 64 << 20

// probeTimeout bounds the probe's scope, so a user manager that does not answer reads as a host that cannot cap one.
// probeWaitDelay bounds the wait for the probe's output once it is killed.
const (
	probeTimeout   = 5 * time.Second
	probeWaitDelay = time.Second
)

// scopeStartScript is what a scope runs ahead of its command.
// Its first argument is the memory.max the scope must hold, and the rest are the command and its arguments.
// A scope whose memory.max differs never runs the command: the script names both values on stderr and exits 125.
// So does a scope whose PATH holds no setsid, which the holder needs to leave the command's process group.
// Otherwise it writes the scope's cgroup to file descriptor 3, which tells the caller the scope started under the cap.
// It then starts a holder that keeps the scope until file descriptor 4 reaches end of file,
// so the scope and its memory events outlast the command until the caller has read them.
// The holder leaves the shell's tree and process group at once, in a session of its own,
// so the command never sees it as a child of its own and a signal to the command's group never reaches it.
// Its subshell points stdin, stdout and stderr at /dev/null before it starts the holder,
// so the holder never keeps a pipe of the command's open,
// and a caller's wait for the command's output ends with the command.
// The shell then becomes the command, so the command's process is the shell's.
const scopeStartScript = `while IFS= read -r line; do case $line in 0::*) scope=${line#0::};; esac; done < ` + selfCgroupFile + `; ` +
	`IFS= read -r max < "` + cgroupMount + `$scope/` + cgroupMemoryMaxFile + `" && [ "$max" = "$1" ] || ` +
	`{ echo "memory cap: the scope $scope holds ` + cgroupMemoryMaxFile + ` ${max:-unread}, not $1" >&2; exit 125; }; ` +
	`command -v setsid >/dev/null || { echo "memory cap: setsid is not on the scope's PATH" >&2; exit 125; }; ` +
	`printf '0::%s\n' "$scope" >&3; exec 3>&-; ( exec </dev/null >/dev/null 2>&1; setsid sh -c 'read -r line' <&4 & ); exec 4<&-; shift; exec "$@"`

// markerReadBound bounds how long the read of the scope's cgroup listing may go on once the command has ended.
// Every writer of the start pipe has closed by then, so the read ends at once unless a stray process holds the pipe.
const markerReadBound = time.Second

// cgroupKillFile names the cgroup v2 file whose write of 1 kills every process in the cgroup.
const cgroupKillFile = "cgroup.kill"

// cgroupMemoryEventsFile names the cgroup v2 file that counts the memory events of the cgroup, such as its OOM kills.
const cgroupMemoryEventsFile = "memory.events"

// memoryScope is how the host runs a command in a memory-capped scope; its zero value is a host that cannot.
type memoryScope struct {
	// starter is the path of systemd-run, empty on a host that cannot cap a scope's memory.
	starter string
}

// hostMemoryScope probes the host once per process.
var hostMemoryScope = sync.OnceValue(probeMemoryScope)

// probeMemoryScope starts one capped scope and returns the host's memoryScope.
// The scope prints its own cgroup,
// and the host can cap one when that cgroup's parent hands the memory controller down.
// A user manager without that controller starts the scope all the same and caps nothing.
// A scope that outlasts probeTimeout counts as one the host cannot start.
func probeMemoryScope() memoryScope {
	starter, err := exec.LookPath(systemdRun)
	if err != nil {
		return memoryScope{}
	}
	bounded, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	probe := exec.CommandContext(bounded, starter, memoryScopeArgs(probeMemoryCap, "cat", selfCgroupFile)...)
	probe.WaitDelay = probeWaitDelay
	listing, err := probe.Output()
	if err != nil {
		return memoryScope{}
	}
	cgroup, ok := cgroupV2Path(string(listing))
	if !ok {
		return memoryScope{}
	}
	controllers, err := os.ReadFile(filepath.Join(cgroupMount, filepath.FromSlash(path.Dir(cgroup)), "cgroup.subtree_control"))
	if err != nil || !slices.Contains(strings.Fields(string(controllers)), memoryController) {
		return memoryScope{}
	}
	return memoryScope{starter: starter}
}

// memoryScopeArgs returns systemd-run's arguments for running command in a scope capped at limit bytes with no swap.
// The scope is collected once it ends, whatever ended it.
// Its OOM policy is continue, so a kill at the cap never fails the scope with the result oom-kill.
// A desktop's out-of-memory notifier reports each unit that ends with that result as a host low on memory,
// and a kill at a test's own cap says nothing about the host.
func memoryScopeArgs(limit int64, command ...string) []string {
	return append([]string{
		"--user", "--scope", "--quiet", "--collect",
		"-p", "MemoryMax=" + strconv.FormatInt(limit, 10), "-p", "MemorySwapMax=0", "-p", "OOMPolicy=continue", "--",
	}, command...)
}

// runMemoryCappedPlatform runs cmd capped at limit bytes by the means the probe found, starting it through start.
func runMemoryCappedPlatform(cmd *exec.Cmd, limit int64, start StartFunc) (killed, treeKilled bool, err error) {
	return runMemoryCapped(cmd, limit, hostMemoryScope(), start)
}

// memoryCapCoversTreePlatform reports whether the host caps a command in a scope, which holds its whole tree.
func memoryCapCoversTreePlatform() bool {
	return hostMemoryScope().starter != ""
}

// runMemoryCapped runs cmd capped at limit bytes: in a scope when scope can start one, else under a data segment limit.
// It starts cmd through start and waits through the wait start returns.
// In a scope, killed reports a command the kernel ended with SIGKILL when no cancel of cmd sent one.
// treeKilled reports that the kernel killed a process in the scope at the scope's own limit.
// The scope's count of kills at its own limit tells, read once the command has ended.
// A holder process keeps the scope, and so the count, until that read is done.
// A SIGKILL sent from outside, to the command or to any other process, is no such kill.
// Neither is a kill at the limit of a cgroup that encloses the scope.
// After any SIGKILL of the command or kill at the cap, every process left in the scope is killed too.
// A cap that could not be set returns an error wrapping ErrMemoryCap.
// So does a scope that never ran the command, or whose memory.max is not limit rounded down to a whole page.
// That error is never an *exec.ExitError, so it never reads as a failing command.
// The scope path uses cmd.ExtraFiles, so cmd must carry none.
func runMemoryCapped(cmd *exec.Cmd, limit int64, scope memoryScope, start StartFunc) (killed, treeKilled bool, err error) {
	if scope.starter == "" {
		return false, false, runDataLimited(cmd, limit, start)
	}
	started, marker, err := os.Pipe()
	if err != nil {
		return false, false, fmt.Errorf("%w: create the pipe that marks the scope's start: %w", ErrMemoryCap, err)
	}
	defer started.Close()
	holderEnd, release, err := os.Pipe()
	if err != nil {
		_ = marker.Close()
		return false, false, fmt.Errorf("%w: create the pipe that holds the scope: %w", ErrMemoryCap, err)
	}
	// Closing release ends the holder, after the scope's events are read and on every return.
	defer release.Close()
	// The kernel rounds a cgroup's memory.max down to a whole page.
	wantMax := limit &^ int64(unix.Getpagesize()-1)
	// systemd-run moves itself into the scope and then becomes the shell, which becomes the command,
	// so the pid and process group stay cmd's own.
	shell := []string{"/bin/sh", "-c", scopeStartScript, "sh", strconv.FormatInt(wantMax, 10), cmd.Path}
	cmd.Args = append([]string{systemdRun}, memoryScopeArgs(limit, append(shell, cmd.Args[1:]...)...)...)
	cmd.Path = scope.starter
	cmd.ExtraFiles = []*os.File{marker, holderEnd}
	var cancelled atomic.Bool
	if cancel := cmd.Cancel; cancel != nil {
		cmd.Cancel = func() error {
			cancelled.Store(true)
			return cancel()
		}
	}
	wait, err := start(cmd)
	marker.Close()
	holderEnd.Close()
	if err != nil {
		return false, false, err
	}
	err = wait()
	_ = started.SetReadDeadline(time.Now().Add(markerReadBound))
	cgroup, ranUnderCap := scopeCgroup(started)
	if cmd.ProcessState == nil || cancelled.Load() {
		return false, false, err
	}
	if !ranUnderCap {
		return false, false, fmt.Errorf("%w: the scope did not run the command under a %s of %d bytes, and %s ended with: %v", ErrMemoryCap, cgroupMemoryMaxFile, wantMax, systemdRun, err)
	}
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	killed = ok && status.Signaled() && status.Signal() == syscall.SIGKILL
	oomKilled := scopeHadOOMKill(cgroup)
	if !killed && !oomKilled {
		return false, false, err
	}
	if killErr := killScope(cgroup); killErr != nil {
		err = errors.Join(err, killErr)
	}
	return killed, oomKilled, err
}

// scopeHadOOMKill reports whether the kernel killed a process of the scope at the scope's own limit.
// The scope is the one whose cgroup is cgroup.
// A scope already gone had nothing left to kill, so it reports false.
func scopeHadOOMKill(cgroup string) bool {
	events, err := os.ReadFile(filepath.Join(cgroupMount, filepath.FromSlash(cgroup), cgroupMemoryEventsFile))
	return err == nil && killedAtOwnLimit(string(events))
}

// killedAtOwnLimit reports whether the content of a cgroup's memory.events file shows a kill at the cgroup's own limit.
// The oom_kill count also rises for a kill at the limit of an enclosing cgroup or of the host.
// The oom count rises on the cgroup whose own limit ran out and on its ancestors, never on a cgroup below it,
// so a kill counts only when both are above 0.
func killedAtOwnLimit(events string) bool {
	var oomKills, ooms int
	for _, line := range strings.Split(events, "\n") {
		name, value, found := strings.Cut(line, " ")
		count, err := strconv.Atoi(value)
		if !found || err != nil {
			continue
		}
		switch name {
		case "oom":
			ooms = count
		case "oom_kill":
			oomKills = count
		}
	}
	return oomKills > 0 && ooms > 0
}

// scopeCgroup returns the cgroup the scope's shell wrote to started, the read end of the scope's start pipe.
// It reads until every writer of the pipe has closed, so a caller bounds it with a read deadline when that must end.
// ok is false when the shell wrote none, which means the scope never ran the command under its cap.
func scopeCgroup(started *os.File) (cgroup string, ok bool) {
	listing, err := io.ReadAll(started)
	if err != nil {
		return "", false
	}
	return cgroupV2Path(string(listing))
}

// killScope kills every process left in the scope whose cgroup is cgroup.
// The scope's OOM policy keeps it running after the kernel kills its command,
// so the processes the command started would otherwise outlive it.
// A scope already emptied and removed is no error.
func killScope(cgroup string) error {
	killFile := filepath.Join(cgroupMount, filepath.FromSlash(cgroup), cgroupKillFile)
	file, err := os.OpenFile(killFile, os.O_WRONLY, 0)
	if err == nil {
		_, err = file.Write([]byte("1"))
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, unix.ENODEV) {
		return fmt.Errorf("kill what the capped command left in its scope: %w", err)
	}
	return nil
}

// runDataLimited runs cmd with the data segment of its process limited to limit bytes.
// Every process cmd starts from then on inherits the limit as its own.
// The limit counts the memory a process maps to write to, not the address space it only reserves.
// It is set once start has started cmd, since a command cannot be started with one.
func runDataLimited(cmd *exec.Cmd, limit int64, start StartFunc) error {
	wait, err := start(cmd)
	if err != nil {
		return err
	}
	bound := unix.Rlimit{Cur: uint64(limit), Max: uint64(limit)}
	if err := unix.Prlimit(cmd.Process.Pid, unix.RLIMIT_DATA, &bound, nil); err != nil && !errors.Is(err, unix.ESRCH) {
		_ = cmd.Process.Kill()
		_ = wait()
		return fmt.Errorf("%w: limit the data segment of process %d: %w", ErrMemoryCap, cmd.Process.Pid, err)
	}
	return wait()
}
