// retention.go implements the durable trace-file retention sweep: a standalone function over a directory path that enforces an age bound and a count bound over trace groups.
// A trace group is every file sharing one trace id,
// and a group's activity time is the newest mtime among its files, so a long-running step that keeps writing stays recent even though its filename timestamp records only when it started.
// It is deliberately independent of the durable sink, lyxcwd, and trace identity — Sweep takes a directory path and bounds from its caller and needs only internal/proc's liveness probe plus the standard library.

package logger

import (
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

// RetentionBounds is the pair of limits Sweep enforces.
// Count is the number of non-live trace groups to keep,
// and MaxAge is how long a non-live group may go without activity before it is deleted.
type RetentionBounds struct {
	Count  int
	MaxAge time.Duration
}

// DefaultRetentionBounds returns the compiled-in bounds every process sweeps with until the exit path passes configured ones: 200 traces and 14 days.
func DefaultRetentionBounds() RetentionBounds {
	return RetentionBounds{Count: 200, MaxAge: 14 * 24 * time.Hour}
}

type traceGroup struct {
	paths    []string
	activity time.Time
	live     bool
}

// Sweep enforces the trace-file directory's retention policy over trace groups.
// Files are grouped by trace id,
// and a group's activity time is the newest mtime among its files;
// a file whose stat fails contributes its filename timestamp instead.
// A group is live when any of its files carries the sweeping process's pid or a pid that is alive.
// Live groups are never deleted and do not consume count budget.
// A non-live group with no activity within bounds.MaxAge has every file deleted.
// The remaining non-live groups are ranked by activity time, newest first,
// and only the first bounds.Count survive;
// every file of each later group is deleted.
// Non-matching files and subdirectories are untouched.
// Delete failures are silently tolerated.
// Empty or absent directories return nil.
func Sweep(dir string, bounds RetentionBounds) error {
	return sweep(dir, bounds, time.Now(), os.Stat)
}

func sweep(dir string, bounds RetentionBounds, now time.Time, stat func(string) (fs.FileInfo, error)) error {
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
	var ranked []*traceGroup
	for _, group := range groups {
		if group.live {
			continue
		}
		if group.activity.Before(cutoff) {
			removeGroup(group)
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
		removeGroup(group)
	}
	return nil
}

func removeGroup(group *traceGroup) {
	for _, path := range group.paths {
		_ = os.Remove(path)
	}
}
