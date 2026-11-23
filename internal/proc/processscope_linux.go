// processscope_linux.go moves a running process into a memory-capped scope of the systemd user manager over D-Bus.
// It also reads a process's OOM kill count from its cgroup and the killed processes' names from the kernel journal.

package proc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// busctl names the command that calls the systemd user manager over D-Bus.
const busctl = "busctl"

// journalctl names the command that reads the systemd journal.
const journalctl = "journalctl"

// scopeJoinTimeout bounds the scope's start: the user manager's answer and the move of the process into the scope.
// scopeJoinPoll is the wait between two reads of the process's cgroup while it moves.
// scopeStatePollEvery is how many of those reads pass between two reads of the scope's state.
// scopeEndedReadsToFail is how many state reads in a row must show the scope ended before the wait gives up on it:
// a scope whose start job is still queued reads inactive too.
const (
	scopeJoinTimeout      = 5 * time.Second
	scopeJoinPoll         = 20 * time.Millisecond
	scopeStatePollEvery   = 10
	scopeEndedReadsToFail = 2
)

// journalReadTimeout bounds the read of the kernel journal.
const journalReadTimeout = 10 * time.Second

// oomKillCountKey is the key of the memory.events line that counts the processes the OOM killer ended in the cgroup.
const oomKillCountKey = "oom_kill"

// capProcessMemoryPlatform asks the user manager for a transient scope named unit holding pid, capped at limit bytes.
// The answer comes before the scope has started, so it waits until pid's cgroup is the scope's.
// It then checks that the scope's memory.max is limit rounded down to a whole page.
func capProcessMemoryPlatform(pid int, unit string, limit int64) (string, error) {
	starter, err := exec.LookPath(busctl)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not on PATH: %w", ErrMemoryCap, busctl, err)
	}
	bounded, cancel := context.WithTimeout(context.Background(), scopeJoinTimeout)
	defer cancel()
	if out, err := exec.CommandContext(bounded, starter, startScopeArgs(pid, unit, limit)...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: the user manager refused the scope %s: %w: %s", ErrMemoryCap, unit, err, strings.TrimSpace(string(out)))
	}
	cgroup, err := waitForScope(bounded, func() (string, bool) { return scopeEnded(bounded, starter, unit) }, pid, unit)
	if err != nil {
		return "", err
	}
	wantMax := strconv.FormatInt(limit&^int64(unix.Getpagesize()-1), 10)
	held, err := os.ReadFile(filepath.Join(cgroupMount, filepath.FromSlash(cgroup), cgroupMemoryMaxFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: the user manager does not hand the memory controller to the scope %s", ErrMemoryCap, unit)
	}
	if err != nil {
		return "", fmt.Errorf("%w: read the %s of the scope %s: %w", ErrMemoryCap, cgroupMemoryMaxFile, unit, err)
	}
	if got := strings.TrimSpace(string(held)); got != wantMax {
		return "", fmt.Errorf("%w: the scope %s holds %s %s, not %s", ErrMemoryCap, unit, cgroupMemoryMaxFile, got, wantMax)
	}
	return cgroup, nil
}

// startScopeArgs returns busctl's arguments for starting the transient scope unit that holds pid.
// The scope is capped at limit bytes with no swap.
// Its OOM policy is continue, so a kill at the cap never stops the scope and with it every process left in it.
// It is collected once it empties, whatever ended it.
func startScopeArgs(pid int, unit string, limit int64) []string {
	return []string{
		"--user", "call", "org.freedesktop.systemd1", "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager",
		"StartTransientUnit", "ssa(sv)a(sa(sv))", unit, "fail", "5",
		"PIDs", "au", "1", strconv.Itoa(pid),
		"MemoryMax", "t", strconv.FormatInt(limit, 10),
		"MemorySwapMax", "t", "0",
		"OOMPolicy", "s", "continue",
		"CollectMode", "s", "inactive-or-failed",
		"0",
	}
}

// waitForScope waits until process pid's cgroup is the scope unit's and returns that cgroup.
// Every scopeStatePollEvery reads it calls readEnded, which answers the scope's state and whether that state is ended.
// scopeEndedReadsToFail answers of ended in a row end the wait,
// so a start job that failed after the user manager accepted it does not hold the wait to its timeout.
// A process that ends, a scope that ended or a ctx that is done first returns an error wrapping ErrMemoryCap.
func waitForScope(ctx context.Context, readEnded func() (state string, ended bool), pid int, unit string) (string, error) {
	cgroupFile := processCgroupFile(pid)
	endedReads := 0
	for polls := 1; ; polls++ {
		cgroup, ok := readCgroupV2Path(cgroupFile)
		if !ok {
			return "", fmt.Errorf("%w: process %d ended before it joined the scope %s", ErrMemoryCap, pid, unit)
		}
		if path.Base(cgroup) == unit {
			return cgroup, nil
		}
		if polls%scopeStatePollEvery == 0 {
			state, ended := readEnded()
			if !ended {
				endedReads = 0
			} else if endedReads++; endedReads >= scopeEndedReadsToFail {
				return "", fmt.Errorf("%w: the scope %s is %s, so process %d never joined it", ErrMemoryCap, unit, state, pid)
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w: process %d was still in %s when the wait for the scope %s ended", ErrMemoryCap, pid, cgroup, unit)
		case <-time.After(scopeJoinPoll):
		}
	}
}

// scopeEnded reports whether the user manager at busctlPath shows the scope unit as failed or inactive, with that state.
// The scope's collect mode drops a failed unit at once, and the user manager reads a dropped unit as inactive.
// A state that cannot be read reports false, so the wait goes on to its timeout.
func scopeEnded(ctx context.Context, busctlPath, unit string) (state string, ended bool) {
	args := []string{"--user", "get-property", "org.freedesktop.systemd1", "/org/freedesktop/systemd1/unit/" + unitObjectPathLabel(unit),
		"org.freedesktop.systemd1.Unit", "ActiveState"}
	out, err := exec.CommandContext(ctx, busctlPath, args...).Output()
	if err != nil {
		return "", false
	}
	return scopeStateEnded(string(out))
}

// unitObjectPathLabel escapes the unit name into the last element of the unit's D-Bus object path.
// Every byte but a letter, or a digit past the first byte, becomes an underscore and its two lower-case hex digits.
func unitObjectPathLabel(unit string) string {
	var label strings.Builder
	for i := 0; i < len(unit); i++ {
		c := unit[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') {
			label.WriteByte(c)
			continue
		}
		fmt.Fprintf(&label, "_%02x", c)
	}
	return label.String()
}

// scopeStateEnded reads busctl's answer to a get-property of a unit's ActiveState, such as `s "failed"`.
// It returns the state and whether it is failed or inactive.
func scopeStateEnded(answer string) (state string, ended bool) {
	state = strings.Trim(strings.TrimPrefix(strings.TrimSpace(answer), "s "), `"`)
	return state, state == "failed" || state == "inactive"
}

// processCgroupFile names the file that lists the cgroup of process pid.
func processCgroupFile(pid int) string {
	return "/proc/" + strconv.Itoa(pid) + "/cgroup"
}

// processOOMKillsPlatform reads pid's cgroup and the oom_kill count of that cgroup's memory.events file.
func processOOMKillsPlatform(pid int) (string, int, error) {
	cgroup, ok := readCgroupV2Path(processCgroupFile(pid))
	if !ok {
		return "", 0, fmt.Errorf("read the cgroup of process %d: no cgroup v2 path in %s", pid, processCgroupFile(pid))
	}
	events, err := os.ReadFile(filepath.Join(cgroupMount, filepath.FromSlash(cgroup), cgroupMemoryEventsFile))
	if err != nil {
		return cgroup, 0, fmt.Errorf("read the OOM kills of process %d's cgroup: %w", pid, err)
	}
	return cgroup, oomKillCount(string(events)), nil
}

// oomKillCount returns the oom_kill count of the content of a cgroup's memory.events file, 0 when it holds none.
func oomKillCount(events string) int {
	for _, line := range strings.Split(events, "\n") {
		name, value, found := strings.Cut(line, " ")
		if !found || name != oomKillCountKey {
			continue
		}
		if count, err := strconv.Atoi(value); err == nil {
			return count
		}
	}
	return 0
}

// oomKillsInCgroupPlatform reads the current boot's kernel messages that name cgroup as a killed task's cgroup.
// journalctl exits 1 when no message matches, which reads as no kill.
func oomKillsInCgroupPlatform(cgroup string) ([]OOMKill, error) {
	bounded, cancel := context.WithTimeout(context.Background(), journalReadTimeout)
	defer cancel()
	read := exec.CommandContext(bounded, journalctl, "--dmesg", "--boot", "--output=cat", "--no-pager",
		"--grep", "task_memcg="+regexp.QuoteMeta(cgroup)+",")
	var stderr strings.Builder
	read.Stderr = &stderr
	out, err := read.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the kernel journal: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseOOMKills(string(out), cgroup), nil
}

// oomKillLine matches the kernel's oom-kill message, capturing the killed task's cgroup, name and pid.
var oomKillLine = regexp.MustCompile(`oom-kill:.*,task_memcg=([^,]*),task=(.*),pid=(\d+),uid=\d+`)

// parseOOMKills returns the kills that the kernel messages in journal record for tasks of cgroup, oldest first.
func parseOOMKills(journal, cgroup string) []OOMKill {
	var kills []OOMKill
	for _, line := range strings.Split(journal, "\n") {
		match := oomKillLine.FindStringSubmatch(line)
		if match == nil || match[1] != cgroup {
			continue
		}
		pid, err := strconv.Atoi(match[3])
		if err != nil {
			continue
		}
		kills = append(kills, OOMKill{PID: pid, Name: match[2]})
	}
	return kills
}
