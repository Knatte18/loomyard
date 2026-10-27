// anchor.go implements the recorded lym-anchor subpath marker.
// The marker is the plain single-line ".lym-anchor" file that records the subpath lym anchors at (e.g. "backend" or ".").
// A worktree's own marker sits at the root of its records sibling, and the legacy hub-wide marker sits at the board root.
// The read order is the own marker, then the board marker, then ".".
// It is read here, never written: only fabricengine writes the marker or reads it from git.
// This file imports the standard library and internal/weftname only, so lymcwd never gains a YAML dependency.

package lymcwd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// boardDirName is the name of the board data directory inside the hub (i.e.
// <hub>/_board). It stays private to lymcwd: the exported BoardDir(hub)
// constructor moved to internal/fabricengine, which declares its own copy
// of this literal; readRecordedAnchor below is the sole reason lymcwd still
// needs the name itself, to find the recorded-anchor marker.
const boardDirName = "_board"

// boardDir returns the absolute path to the board data directory inside hub.
// Private: see boardDirName's comment for why lymcwd retains this one reader.
func boardDir(hub string) string {
	return filepath.Join(hub, boardDirName)
}

// AnchorFileName is the filename of the recorded lym-anchor subpath marker.
// It sits at the root of a worktree's records sibling for that worktree's own anchor.
// It sits at the board root (<boardDir(hub)>/.lym-anchor) for the legacy hub-wide fallback the own marker overrides.
// It holds only the subpath string (e.g. "backend" or ".").
// This is a structural geometry artifact recorded when the worktree is created or moved, never a config/env override;
// per the Cwd Resolution Invariant, only lymcwd constructs and reads this path.
// There is no compatibility fallback read for the pre-rename ".fabric-anchor" name: the marker
// anchors the whole repo, not the fabric module, so the old name is simply wrong now, not
// merely renamed — see clone.go's stale-marker guard.
const AnchorFileName = ".lym-anchor"

// StaleAnchorFileName is the pre-rename spelling of the recorded lym-anchor marker.
// It is never read as an anchor value — the marker anchors the whole repo, not the fabric module,
// so the old name is simply wrong now rather than merely renamed — but its presence must be
// DETECTED, because falling back to "." for a hub that recorded a real subpath under the old name
// silently re-anchors the whole repo at its root.
// It is exported so fabric's clone-time guard names the same literal this package's read-time guard
// does, rather than each declaring its own copy.
const StaleAnchorFileName = ".fabric-anchor"

// ErrStaleAnchorMarker is the hard-error sentinel returned when a hub's board directory carries the
// pre-rename StaleAnchorFileName marker with no AnchorFileName beside it.
// Resolving such a hub at all would mean re-anchoring it at ".", so every resolver refuses instead
// of guessing.
var ErrStaleAnchorMarker = errors.New("stale pre-rename fabric anchor marker with no renamed marker beside it")

// ErrInvalidAnchor is the hard-error sentinel returned for an anchor subpath that is not a
// worktree-relative path staying inside the worktree.
var ErrInvalidAnchor = errors.New("invalid lym anchor subpath")

// ValidateAnchorRel normalizes raw into the worktree-relative form AnchorRel holds, rejecting
// anything that is not one.
// It is the single validator for the anchor value on both sides of the marker: fabric's clone calls
// it before recording a --subpath, and readRecordedAnchor calls it on every read, so a value that
// could never be recorded today can also never be honoured if an older binary recorded it.
//
// An empty or whitespace-only raw normalizes to ".", the repo-root anchor.
// An absolute path, a Windows volume-rooted path, and a path whose first segment escapes the
// worktree (".." or "../…") are all rejected: filepath.Join silently swallows a leading separator,
// so an absolute anchor LOOKS like it resolves, while every anchor-scoped git pathspec later built
// on top of it becomes an absolute path git refuses outright — permanently breaking every commit
// routed through the anchored subtree.
func ValidateAnchorRel(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ".", nil
	}

	if filepath.VolumeName(trimmed) != "" {
		return "", fmt.Errorf("%w %q: must be relative to the worktree root, not volume-rooted", ErrInvalidAnchor, raw)
	}

	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, string(filepath.Separator)) {
		return "", fmt.Errorf("%w %q: must be relative to the worktree root, not absolute", ErrInvalidAnchor, raw)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w %q: must stay inside the worktree, not escape it", ErrInvalidAnchor, raw)
	}

	return cleaned, nil
}

// ErrCwdOutsideAnchor is the hard-error sentinel Resolve returns when cwd does not equal the
// anchored directory exactly.
// It exists so a lym invocation from anywhere else in the worktree — a subdirectory, a parent, or a
// sibling of a subpath-anchored repo — fails loudly and immediately instead of resolving the wrong
// AnchorRel and dying later downstream with a confusing error.
var ErrCwdOutsideAnchor = errors.New("cwd is outside the recorded fabric anchor subtree")

// samePath reports whether a and b name the same filesystem location. Each
// side is normalized through filepath.EvalSymlinks then filepath.Clean,
// falling back to Clean-only for whichever side EvalSymlinks fails on (the
// path may not exist yet, e.g. during clone). Normalization is not optional:
// the worktree side comes from git rev-parse --show-toplevel while cwd comes
// from os.Getwd, and the two disagree routinely — macOS's /tmp is a symlink
// to /private/tmp, gitkit fixtures live under symlinked temp dirs, and
// Windows/macOS filesystems are case-insensitive while Go string comparison
// is not. Comparison is byte-exact on Linux/macOS and case-insensitive
// (strings.EqualFold) on Windows.
func samePath(a, b string) bool {
	na := normalizePath(a)
	nb := normalizePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(na, nb)
	}
	return na == nb
}

// normalizePath resolves symlinks in p, falling back to filepath.Clean alone
// when EvalSymlinks fails (p may not exist on disk yet).
func normalizePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(resolved)
}

// checkCwdAnchorGate returns nil when cwd is exactly the anchored directory
// (filepath.Join(worktreePath, anchorRel)), and otherwise wraps
// ErrCwdOutsideAnchor in a message naming both sides and the marker file.
func checkCwdAnchorGate(cwd, anchorRel, worktreePath string) error {
	anchorAbs := filepath.Join(worktreePath, anchorRel)
	if samePath(cwd, anchorAbs) {
		return nil
	}
	return fmt.Errorf("%w: cwd %s does not equal the anchored directory %s (recorded by %s)",
		ErrCwdOutsideAnchor, cwd, anchorAbs, AnchorFileName)
}

// readRecordedAnchor reads the recorded lym-anchor subpath marker of the worktree named worktreeName in hub.
// It reports whether a usable anchor was found.
// It reads the worktree's own marker at the root of its records sibling first.
// It reads the legacy hub-wide marker at <BoardDir(hub)>/.lym-anchor second.
// An own marker that fails ValidateAnchorRel is an error naming the file;
// an empty or whitespace-only own marker counts as absent and the read falls through to the board marker.
// An own marker that exists but cannot be read is an error naming the file, as an invalid one is.
// It returns ("", false, nil) when neither marker is usable:
// an absent directory, an absent file or an empty one each mean the caller must fall back to "."
// (mid-clone, a gitkit fixture repo standing in for a hub, or a non-fabric git repo).
// An anchor must never resolve to an empty subpath.
// This helper spawns no git.
//
// The one absence that is NOT a fallback is a hub whose board still carries the pre-rename StaleAnchorFileName marker, no renamed one and no own marker.
// That hub did record a subpath under the old name.
// Answering "." for it re-anchors the whole repo at its root, after which `lym fabric reconcile` wires a second junction set there.
// That case returns ErrStaleAnchorMarker so every resolver refuses rather than guessing.
func readRecordedAnchor(hub, worktreeName string) (anchor string, found bool, err error) {
	ownMarker := filepath.Join(weftname.SiblingPath(hub, worktreeName), AnchorFileName)
	data, readErr := os.ReadFile(ownMarker)
	switch {
	case readErr == nil:
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			validated, validateErr := ValidateAnchorRel(trimmed)
			if validateErr != nil {
				return "", false, fmt.Errorf("recorded anchor at %s is unusable: %w", ownMarker, validateErr)
			}
			return validated, true, nil
		}
	case !errors.Is(readErr, fs.ErrNotExist):
		return "", false, fmt.Errorf("recorded anchor at %s cannot be read: %w", ownMarker, readErr)
	}

	board := boardDir(hub)

	data, readErr = os.ReadFile(filepath.Join(board, AnchorFileName))
	if readErr != nil {
		if _, staleErr := os.Stat(filepath.Join(board, StaleAnchorFileName)); staleErr == nil {
			return "", false, fmt.Errorf("%w: rename %s to %s in the hub's %s worktree and commit it (at %s)",
				ErrStaleAnchorMarker, StaleAnchorFileName, AnchorFileName, boardDirName, board)
		}
		return "", false, nil
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false, nil
	}

	validated, validateErr := ValidateAnchorRel(trimmed)
	if validateErr != nil {
		return "", false, fmt.Errorf("recorded anchor at %s is unusable: %w", board, validateErr)
	}
	return validated, true, nil
}
