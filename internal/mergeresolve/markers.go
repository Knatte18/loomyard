// markers.go implements scanUnresolved, the mechanical re-scan that decides whether a conflicted path is resolved.
// The scan trusts no session verdict.
// It is a line-anchored, content-only match for the marker triad git leaves behind.

package mergeresolve

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The three conflict-marker prefixes that mark a line as still conflicted.
// Each is the seven-character rune run followed by a space, except the equals run, which stands alone:
// git's own separator line carries no trailing ref name to space-delimit.
const (
	conflictMarkerOurs   = "<<<<<<< "
	conflictMarkerMiddle = "======="
	conflictMarkerTheirs = ">>>>>>> "
)

// scanUnresolved re-scans each of paths, joined onto worktreeRoot, for a still-present conflict marker.
// It returns the subset of paths that remain unresolved.
//
// A path whose file no longer exists counts as resolved by deletion, not as a failure.
// `ConflictedFiles()` includes delete/modify conflicts whose correct resolution is that the file is gone,
// and the caller still passes such a path to the staging step, which stages the removal.
// Such a path is skipped and is never added to unresolved.
//
// A read error that is not a not-exist error is a genuine failure:
// it is returned wrapped and is distinct from the deletion case.
//
// Otherwise the file is scanned line by line, and it is unresolved when any line starts with one of the three markers.
// The match is line-anchored and content-only, never a substring match anywhere in a line.
//
// A resolved file whose legitimate content carries a line-start conflict marker never passes this scan,
// and its merge escalates to stuck.
// That is the safe direction: the conclude step is irreversible at the Fabric layer.
// Such a file is out of scope for automated resolution.
//
// This scan is only a pre-check: the Fabric-level index guard, in `MergeContinue`, remains the authoritative gate.
// This scan can make this package refuse something Fabric would have accepted, never the reverse.
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

// fileHasLineStartMarker reports whether any line read from f starts with a conflict-marker prefix.
func fileHasLineStartMarker(f *os.File) (bool, error) {
	scanner := bufio.NewScanner(f)
	// A conflicted file's diff hunks can exceed the 64KiB default token size of bufio.Scanner.
	// Widen the buffer well past any realistic single line rather than fail the scan on a long line.
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
