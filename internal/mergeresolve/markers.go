// markers.go implements scanUnresolved, the mechanical re-scan that decides whether a conflicted
// path is genuinely resolved: never the session's own verdict, but a line-anchored, content-only
// scan for the marker triad git leaves behind.

package mergeresolve

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The three conflict-marker prefixes scanUnresolved treats a line as still-conflicted for. Each is
// the seven-character rune run followed by a space, except the equals run, which stands alone —
// git's own separator line carries no trailing ref name to space-delimit.
const (
	conflictMarkerOurs   = "<<<<<<< "
	conflictMarkerMiddle = "======="
	conflictMarkerTheirs = ">>>>>>> "
)

// scanUnresolved re-scans each of paths (joined onto worktreeRoot) for a still-present conflict
// marker, returning the subset that is still unresolved.
//
// A path whose file no longer exists is resolved by deletion, not a failure: ConflictedFiles()
// includes delete/modify conflicts whose correct resolution is that the file is gone, and the
// caller still passes such a path to the staging verb, which stages the removal. Such a path is
// skipped here and never added to unresolved.
//
// A read error that is not a not-exist error is a genuine failure, returned wrapped and distinct
// from the deletion case.
//
// Otherwise the file is scanned line by line, and it is still unresolved when any line STARTS WITH
// one of the three markers above — a line-anchored, content-only match, never a substring match
// anywhere in a line.
//
// Consequence, stated rather than hidden: a resolved file whose own legitimate content happens to
// carry a line-start conflict marker can never pass this scan, and its merge escalates to stuck.
// That is the safe direction, because the conclude step is irreversible at the Fabric layer, and
// such a file is out of scope for automated resolution.
//
// Layering note: this scan is only a pre-check. The Fabric-level index guard remains the
// authoritative gate — this scan can make this package refuse something Fabric would have
// accepted, never the reverse.
func scanUnresolved(worktreeRoot string, paths []string) (unresolved []string, err error) {
	for _, p := range paths {
		full := filepath.Join(worktreeRoot, p)

		f, openErr := os.Open(full)
		if openErr != nil {
			if errors.Is(openErr, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("mergeresolve: read %s for the conflict-marker scan: %w", full, openErr)
		}

		stillMarked, scanErr := fileHasLineStartMarker(f)
		closeErr := f.Close()
		if scanErr != nil {
			return nil, fmt.Errorf("mergeresolve: scan %s for conflict markers: %w", full, scanErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("mergeresolve: close %s after the conflict-marker scan: %w", full, closeErr)
		}

		if stillMarked {
			unresolved = append(unresolved, p)
		}
	}
	return unresolved, nil
}

// fileHasLineStartMarker reports whether any line read from f starts with one of the three
// conflict-marker prefixes.
func fileHasLineStartMarker(f *os.File) (bool, error) {
	scanner := bufio.NewScanner(f)
	// A conflicted file's diff hunks can exceed bufio.Scanner's 64KiB default token size; widen the
	// buffer well past any realistic single line rather than fail the scan on a long line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, conflictMarkerOurs) ||
			strings.HasPrefix(line, conflictMarkerMiddle) ||
			strings.HasPrefix(line, conflictMarkerTheirs) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}
