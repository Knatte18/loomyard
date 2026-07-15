// serverlog.go implements server-log concerns: mapping the opt-in debug_log
// config value to tmux verbose-logging flags, and boot-time pruning of the
// per-hub server's log files under the hub's .lyx/logs/. Both are pure
// planning helpers (no filesystem or process I/O); the caller (lifecycle.go)
// performs the actual tmux spawn and file removals.

package muxengine

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// debugLogArgs maps a validated debug_log config value to the tmux global
// flags the server-spawning invocation should prepend to its argv. level is
// trimmed of surrounding whitespace before comparison, so a template-sourced
// value like " 1 " resolves the same as "1". "0" (or empty after trimming —
// never reached today since the template default is "0", but treated the
// same for robustness) yields no flags; "1" yields -v; "2" yields -vv. Any
// other value is a misconfiguration and is reported as an error rather than
// silently ignored, so an invalid debug_log fails the boot loud instead of
// booting with the wrong verbosity.
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

// planLogPrune returns the subset of names to delete so that only the keep
// newest (by the parallel mtimes slice) remain — the planning half of the
// boot-time tmux-server-*.log prune (Shared Decision log-prune-keep-3); the
// caller does the actual os.Remove calls. names and mtimes must be the same
// length, each names[i] paired with mtimes[i]. When len(names) <= keep,
// nothing is pruned and nil is returned. Ties (equal mtimes) are broken by
// input order: entries earlier in names are treated as newer, so the result
// is deterministic given the same input order. This is a pure function with
// no filesystem I/O.
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
	// Stable sort newest-first; SliceStable preserves the input order for
	// entries with equal mtimes, which is what makes tie-breaking
	// deterministic rather than dependent on sort's internal pivot choice.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].mtime.After(entries[j].mtime)
	})

	toDelete := make([]string, 0, len(entries)-keep)
	for _, e := range entries[keep:] {
		toDelete = append(toDelete, e.name)
	}
	return toDelete
}
