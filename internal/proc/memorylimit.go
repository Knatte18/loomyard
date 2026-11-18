// memorylimit.go reads the cgroup v2 memory limit that the calling process sits under.
// It is the level of the cgroup ancestry with the least memory left.

package proc

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupMemoryMaxFile names the cgroup v2 file that holds the cgroup's memory limit, in bytes or max for none.
const cgroupMemoryMaxFile = "memory.max"

// MemoryLimit is one cgroup's memory use and its limit.
type MemoryLimit struct {
	// Used is the bytes the cgroup and its descendants hold outside the file cache, which the kernel drops under pressure.
	Used int64
	// Max is the lower of the cgroup's `memory.high` and `memory.max`.
	Max int64
	// Cgroup is the cgroup's path below the cgroup v2 mount, such as `/user.slice/user-1000.slice`.
	Cgroup string
}

// Headroom is the bytes the cgroup can still take before its limit throttles or kills.
func (l MemoryLimit) Headroom() int64 {
	return l.Max - l.Used
}

// ReadMemoryLimit reads the memory limit closest to being reached over the calling process's cgroup and its ancestors.
// A level's limit is the lower of its `memory.high`, where the kernel throttles the level, and its `memory.max`.
// Reading every level sees a limit wherever it is set, such as a systemd user slice.
// found is false when the process's cgroup v2 path is unreadable, as on Windows or a cgroup v1 host.
// It is also false when no level sets a numeric limit.
func ReadMemoryLimit() (limit MemoryLimit, found bool) {
	return readMemoryLimitAt(selfCgroupFile, cgroupMount)
}

// readMemoryLimitAt reads the memory limit closest to being reached for the process whose cgroup file is selfCgroup.
// mount is the cgroup v2 mount the levels are read under.
func readMemoryLimitAt(selfCgroup, mount string) (MemoryLimit, bool) {
	cgroup, ok := readCgroupV2Path(selfCgroup)
	if !ok {
		return MemoryLimit{}, false
	}
	var tightest MemoryLimit
	found := false
	for level := cgroup; ; level = path.Dir(level) {
		if limit, ok := readMemoryLevel(mount, level); ok && (!found || limit.Headroom() < tightest.Headroom()) {
			tightest, found = limit, true
		}
		if level == "/" {
			break
		}
	}
	return tightest, found
}

// readMemoryLevel reads the memory use and numeric limit of the cgroup at level under mount;
// ok is false when the level sets no limit or its `memory.current` is unreadable or malformed.
func readMemoryLevel(mount, level string) (MemoryLimit, bool) {
	dir := filepath.Join(mount, filepath.FromSlash(level))
	throttle, throttled := readIntFile(filepath.Join(dir, "memory.high"))
	hard, capped := readIntFile(filepath.Join(dir, cgroupMemoryMaxFile))
	limit := hard
	switch {
	case !throttled && !capped:
		return MemoryLimit{}, false
	case throttled && (!capped || throttle < hard):
		limit = throttle
	}
	current, ok := readIntFile(filepath.Join(dir, "memory.current"))
	if !ok {
		return MemoryLimit{}, false
	}
	return MemoryLimit{Used: int64(current) - fileCacheBytes(dir), Max: int64(limit), Cgroup: level}, true
}

// fileCacheBytes returns the file cache the cgroup at dir holds:
// the `inactive_file` and `active_file` lines of its `memory.stat`.
// An unreadable file reads as none.
func fileCacheBytes(dir string) int64 {
	content, err := os.ReadFile(filepath.Join(dir, "memory.stat"))
	if err != nil {
		return 0
	}
	var cache int64
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(line, " ")
		if !found || (key != "inactive_file" && key != "active_file") {
			continue
		}
		if bytes, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			cache += bytes
		}
	}
	return cache
}
