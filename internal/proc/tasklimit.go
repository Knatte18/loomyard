// tasklimit.go reads the cgroup v2 task limit the calling process sits under: the level of its cgroup ancestry with the fewest free tasks.

package proc

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// TaskLimit is one cgroup's task count and its limit, as `pids.current` and `pids.max` read.
type TaskLimit struct {
	// Current is the tasks the cgroup and its descendants hold.
	Current int
	// Max is the cgroup's limit on them.
	Max int
	// Cgroup is the cgroup's path below the cgroup v2 mount, such as `/user.slice/user-1000.slice`.
	Cgroup string
}

// Headroom is the tasks the cgroup can still start before its limit refuses a fork.
func (l TaskLimit) Headroom() int {
	return l.Max - l.Current
}

// ReadTaskLimit reads the task limit closest to being reached over the calling process's cgroup and each of its ancestors, so a fork-refusing limit is seen at whichever level it is set, such as a systemd user slice.
// found is false when the process's cgroup v2 path is unreadable, as on Windows or a cgroup v1 host, or when no level sets a numeric limit.
func ReadTaskLimit() (limit TaskLimit, found bool) {
	return readTaskLimitAt("/proc/self/cgroup", "/sys/fs/cgroup")
}

// readTaskLimitAt reads the task limit closest to being reached for the process whose cgroup file is selfCgroup, under the cgroup v2 mount at mount.
func readTaskLimitAt(selfCgroup, mount string) (TaskLimit, bool) {
	cgroup, ok := readCgroupV2Path(selfCgroup)
	if !ok {
		return TaskLimit{}, false
	}
	var tightest TaskLimit
	found := false
	for level := cgroup; ; level = path.Dir(level) {
		if limit, ok := readPidsLevel(mount, level); ok && (!found || limit.Headroom() < tightest.Headroom()) {
			tightest, found = limit, true
		}
		if level == "/" {
			break
		}
	}
	return tightest, found
}

// readCgroupV2Path returns the cgroup v2 path the `0::` line of the cgroup file at selfCgroup names.
func readCgroupV2Path(selfCgroup string) (string, bool) {
	file, err := os.Open(selfCgroup)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if cgroup, ok := strings.CutPrefix(scanner.Text(), "0::"); ok && strings.HasPrefix(cgroup, "/") {
			return path.Clean(cgroup), true
		}
	}
	return "", false
}

// readPidsLevel reads the task count and numeric limit of the cgroup at level under mount; ok is false when the level sets no limit or either file is unreadable or malformed.
func readPidsLevel(mount, level string) (TaskLimit, bool) {
	dir := filepath.Join(mount, filepath.FromSlash(level))
	limit, ok := readIntFile(filepath.Join(dir, "pids.max"))
	if !ok {
		return TaskLimit{}, false
	}
	current, ok := readIntFile(filepath.Join(dir, "pids.current"))
	if !ok {
		return TaskLimit{}, false
	}
	return TaskLimit{Current: current, Max: limit, Cgroup: level}, true
}

// readIntFile reads the one decimal integer the file name holds; a file holding `max` reads as not ok.
func readIntFile(name string) (int, bool) {
	content, err := os.ReadFile(name)
	if err != nil {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(content)))
	return value, err == nil
}
