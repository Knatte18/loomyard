// serverlog.go maps the debug_log config value to tmux verbose-logging flags.
// It also plans the boot-time pruning of the per-hub server's log files under the Bolt checkout's .lym/logs/.
// Both functions are pure planning helpers with no filesystem or process I/O;
// the caller in lifecycle.go performs the tmux spawn and the file removals.

package reedengine

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// debugLogArgs maps a validated debug_log config value to tmux global flags:
// "0" yields none, "1" yields -v, "2" yields -vv.
func debugLogArgs(level string) ([]string, error) {
	switch strings.TrimSpace(level) {
	case "0":
		return nil, nil
	case "1":
		return []string{"-v"}, nil
	case "2":
		return []string{"-vv"}, nil
	default:
		return nil, fmt.Errorf("invalid debug_log %q: must be 0, 1 or 2", level)
	}
}

// planLogPrune returns the names to delete so that only the keep newest remain.
// Ties are broken by input order, so the result is deterministic.
func planLogPrune(names []string, mtimes []time.Time, keep int) []string {
	if keep < 0 {
		keep = 0
	}
	if len(names) <= keep {
		return nil
	}

	type namedMtime struct {
		name  string
		mtime time.Time
	}
	entries := make([]namedMtime, len(names))
	for i, n := range names {
		entries[i] = namedMtime{name: n, mtime: mtimes[i]}
	}
	// Sort newest-first with a stable sort, which keeps the input order of entries with equal mtimes.
	// Tie-breaking is therefore deterministic, not dependent on sort's internal pivot choice.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].mtime.After(entries[j].mtime)
	})

	toDelete := make([]string, 0, len(entries)-keep)
	for _, e := range entries[keep:] {
		toDelete = append(toDelete, e.name)
	}
	return toDelete
}
