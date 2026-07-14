// serverlog.go implements server-log concerns: mapping the opt-in debug_log
// config value to psmux verbose-logging flags, and boot-time pruning of the
// per-hub server's log files under the hub's .lyx/logs/. Both are pure
// planning helpers (no filesystem or process I/O); the caller (lifecycle.go)
// performs the actual psmux spawn and file removals.

package muxengine

import (
	"fmt"
	"strings"
)

// debugLogArgs maps a validated debug_log config value to the psmux global
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
