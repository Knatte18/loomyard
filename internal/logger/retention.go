// retention.go implements the trace-file retention sweep: it deletes old and surplus trace groups from a directory.
// The sweep takes a directory path and its bounds from the caller.
// It stays independent of the durable sink and of trace identity.

package logger

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

const traceFileTimestampLayout = "20060102T150405Z"

var traceFilePattern = regexp.MustCompile(`^trace-(\d{8}T\d{6}Z)-([0-9a-f]{16})-(\d+)\.log$`)

// TraceFilesFor lists the sink files of the trace group traceID in dir as absolute paths, sorted by name.
// A group may span several files, because each name embeds a timestamp and pid that the id does not determine.
// The name sort puts the oldest file first, because the timestamp has a fixed width.
// An absent dir or a group with no file yields an empty list and no error.
func TraceFilesFor(dir, traceID string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		match := traceFilePattern.FindStringSubmatch(entry.Name())
		if entry.IsDir() || match == nil || match[2] != traceID {
			continue
		}
		paths = append(paths, filepath.Join(absDir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

// RetentionBounds holds the limits that decide which trace groups the retention sweep deletes.
// Count is how many non-live trace groups to keep, newest first.
// MaxAge is how long a non-live group may go without activity before its files are deleted.
type RetentionBounds struct {
	Count  int
	MaxAge time.Duration
}

// DefaultRetentionBounds returns the compiled-in bounds for a sweep that has no configured bounds.
func DefaultRetentionBounds() RetentionBounds {
	return RetentionBounds{Count: 200, MaxAge: 14 * 24 * time.Hour}
}

type traceGroup struct {
	paths    []string
	activity time.Time
	live     bool
}

// Sweep deletes the trace files in dir that fall outside bounds.
// Files sharing a trace id form a trace group, and the bounds apply to whole groups.
// A group's activity time is the newest mtime among its files;
// a file whose stat fails contributes its filename timestamp instead.
// A group is live when any of its files names the sweeping process's pid or a pid that is still alive.
// Live groups are never deleted and do not count toward bounds.Count.
// A non-live group idle for longer than bounds.MaxAge has all its files deleted.
// The other non-live groups are ranked by activity time, newest first;
// the first bounds.Count of them are kept, and the files of the rest are deleted.
// Files that do not match the trace file name, and subdirectories, are untouched.
// Failed deletes are ignored, and an empty or absent directory returns nil.
func Sweep(dir string, bounds RetentionBounds) error {
	return sweep(dir, bounds, time.Now(), os.Stat)
}

// SweepDir applies the retention bounds to dir at the current time and returns the paths to delete, sorted.
// It deletes them only when apply is true, so a dry run and an apply list the same paths.
// Failed deletes are ignored, and an empty or absent directory returns no paths.
func SweepDir(dir string, bounds RetentionBounds, apply bool) ([]string, error) {
	doomed := planSweep(dir, bounds, time.Now(), os.Stat)
	if apply {
		removePaths(doomed)
	}
	return doomed, nil
}

func sweep(dir string, bounds RetentionBounds, now time.Time, stat func(string) (fs.FileInfo, error)) error {
	removePaths(planSweep(dir, bounds, now, stat))
	return nil
}

// planSweep decides which trace files a sweep deletes and returns their paths, sorted, without deleting anything.
func planSweep(dir string, bounds RetentionBounds, now time.Time, stat func(string) (fs.FileInfo, error)) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	selfPID := os.Getpid()
	groups := make(map[string]*traceGroup)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := traceFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		ts, err := time.Parse(traceFileTimestampLayout, match[1])
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(match[3])
		if err != nil {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		activity := ts
		if info, err := stat(path); err == nil {
			activity = info.ModTime()
		}

		group := groups[match[2]]
		if group == nil {
			group = &traceGroup{}
			groups[match[2]] = group
		}
		group.paths = append(group.paths, path)
		if activity.After(group.activity) {
			group.activity = activity
		}
		if pid == selfPID || proc.IsAlive(pid) {
			group.live = true
		}
	}

	cutoff := now.Add(-bounds.MaxAge)
	var doomed []string
	var ranked []*traceGroup
	for _, group := range groups {
		if group.live {
			continue
		}
		if group.activity.Before(cutoff) {
			doomed = append(doomed, group.paths...)
			continue
		}
		ranked = append(ranked, group)
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].activity.After(ranked[j].activity)
	})
	keep := bounds.Count
	if keep < 0 {
		keep = 0
	}
	if keep > len(ranked) {
		keep = len(ranked)
	}
	for _, group := range ranked[keep:] {
		doomed = append(doomed, group.paths...)
	}
	sort.Strings(doomed)
	return doomed
}

func removePaths(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
