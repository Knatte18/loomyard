// anchor.go reads the recorded lym-anchor subpath marker: a single-line file naming the subpath lym anchors at.
// The marker is read here and never written: only fabricengine writes it or reads it from git.
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

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// BoltDirName is the name of the Bolt checkout inside a hub, re-exported from lymdirs.
// Callers take the name from here rather than declaring their own copy.
const BoltDirName = lymdirs.BoltDirName

// boardDir returns the absolute path to the Bolt checkout inside hub, where the hub-wide anchor marker lives.
func boardDir(hub string) string {
	return filepath.Join(hub, BoltDirName)
}

// AnchorFileName is the filename of the recorded lym-anchor subpath marker.
// A worktree's own anchor sits at the root of its records sibling.
// The legacy hub-wide anchor sits at the root of the Bolt checkout, and the own marker overrides it.
// The marker holds only the subpath string, such as "backend" or ".".
// It is a structural geometry artifact recorded when the worktree is created or moved.
// It is never a config or env override.
// Per the Cwd Resolution Invariant, only lymcwd constructs and reads this path.
// The pre-rename ".fabric-anchor" name has no compatibility read.
// The marker anchors the whole repo, not the fabric module, so the old name is wrong now, not merely renamed.
const AnchorFileName = ".lym-anchor"

// StaleAnchorFileName is the pre-rename spelling of the recorded lym-anchor marker.
// Its value is never read as an anchor.
// Its presence must still be detected.
// Otherwise a hub that recorded a subpath under the old name would fall back to "." and re-anchor at the root.
// It is exported so the clone-time guard in fabricengine names the same literal as the read-time guard here.
const StaleAnchorFileName = ".fabric-anchor"

// ErrStaleAnchorMarker is returned for a hub whose Bolt checkout carries the pre-rename StaleAnchorFileName marker.
// No AnchorFileName sits beside that marker.
// Resolving such a hub would re-anchor it at ".", so every resolver returns this error instead of guessing.
var ErrStaleAnchorMarker = errors.New("stale pre-rename fabric anchor marker with no renamed marker beside it")

// ErrInvalidAnchor is returned for an anchor subpath that is not a relative path inside the worktree.
var ErrInvalidAnchor = errors.New("invalid lym anchor subpath")

// ValidateAnchorRel normalizes raw into the worktree-relative form that `AnchorRel` holds, and rejects anything else.
// It is the single validator for the anchor value, on both the write side and the read side of the marker.
// A value that could never be recorded today must also never be honoured when an older binary recorded it.
//
// An empty or whitespace-only raw normalizes to ".", the repo-root anchor.
// An absolute path and a Windows volume-rooted path are rejected.
// So is a path whose first segment escapes the worktree, such as ".." or "../…".
// filepath.Join silently drops a leading separator, so an absolute anchor looks like it resolves.
// Every anchor-scoped git pathspec built on such an anchor then becomes an absolute path that git refuses,
// and every commit routed through the anchored subtree fails permanently.
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

// ErrCwdOutsideAnchor is returned when cwd is not where the recorded anchor admits a lym invocation:
// exactly the anchored directory for the strict gate, that directory or below it for the subtree check.
// A lym invocation from elsewhere in the worktree fails here at once.
// Without this check it would resolve the wrong anchor and fail later, downstream, with a confusing error.
var ErrCwdOutsideAnchor = errors.New("cwd is not at the recorded fabric anchor")

// samePath reports whether a and b name the same filesystem location.
// Each side is normalized through filepath.EvalSymlinks and then filepath.Clean.
// A side that EvalSymlinks fails on falls back to filepath.Clean alone.
// That happens when the path does not exist yet, as during clone.
// The worktree side comes from git rev-parse --show-toplevel and cwd from os.Getwd, and the two disagree routinely.
// macOS's /tmp is a symlink to /private/tmp, and test fixtures live under symlinked temp dirs.
// Windows and macOS filesystems are case-insensitive, while Go string comparison is not.
// Comparison is byte-exact on Linux and macOS, and case-insensitive (strings.EqualFold) on Windows.
func samePath(a, b string) bool {
	na := normalizePath(a)
	nb := normalizePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(na, nb)
	}
	return na == nb
}

// normalizePath resolves symlinks in p and cleans the result.
// When EvalSymlinks fails, as it does for a path that does not exist yet, it cleans p alone.
func normalizePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(resolved)
}

// checkCwdAnchorGate returns nil when cwd is exactly the anchored directory, worktreePath joined with anchorRel.
// Otherwise it wraps ErrCwdOutsideAnchor in an error that names both paths and the marker file.
func checkCwdAnchorGate(cwd, anchorRel, worktreePath string) error {
	anchorAbs := filepath.Join(worktreePath, anchorRel)
	if samePath(cwd, anchorAbs) {
		return nil
	}
	return fmt.Errorf("%w: cwd %s does not equal the anchored directory %s (recorded by %s)",
		ErrCwdOutsideAnchor, cwd, anchorAbs, AnchorFileName)
}

// checkCwdWithinAnchor returns nil when cwd lies at or below the anchored directory.
// The anchored directory is worktreePath joined with anchorRel.
// Otherwise it wraps ErrCwdOutsideAnchor in an error that names both paths and the marker file.
func checkCwdWithinAnchor(cwd, anchorRel, worktreePath string) error {
	anchorAbs := filepath.Join(worktreePath, anchorRel)
	if pathWithin(cwd, anchorAbs) {
		return nil
	}
	return fmt.Errorf("%w: cwd %s lies outside the anchored directory %s (recorded by %s); run it from that directory or below it",
		ErrCwdOutsideAnchor, cwd, anchorAbs, AnchorFileName)
}

// pathWithin reports whether child names parent or a location below it.
// Both sides are normalized and compared as samePath compares them.
func pathWithin(child, parent string) bool {
	normalizedChild := normalizePath(child)
	normalizedParent := normalizePath(parent)
	if runtime.GOOS == "windows" {
		normalizedChild = strings.ToLower(normalizedChild)
		normalizedParent = strings.ToLower(normalizedParent)
	}
	rel, err := filepath.Rel(normalizedParent, normalizedChild)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readRecordedAnchor reads the recorded lym-anchor subpath marker of the worktree named worktreeName in hub.
// It reports whether a usable anchor was found.
// It reads the worktree's own marker first, at the root of its records sibling.
// It reads the legacy hub-wide marker second, at the root of the Bolt checkout.
// An own marker with an invalid anchor value is an error naming the file.
// An empty or whitespace-only own marker counts as absent, and the read falls through to the Bolt checkout's marker.
// An own marker that exists but cannot be read is an error naming the file.
// It returns ("", false, nil) when neither marker is usable.
// An absent directory, an absent file or an empty one each mean the caller must fall back to ".".
// That covers a hub mid-clone, a test fixture repo standing in for a hub, and a git repo that is not a fabric hub.
// An anchor must never resolve to an empty subpath.
// This helper spawns no git.
//
// The one absence that is not a fallback: a Bolt checkout still carrying the pre-rename StaleAnchorFileName marker.
// That hub recorded a subpath under the old name.
// Answering "." would re-anchor the whole repo at its root.
// After that, `lym fabric reconcile` wires a second junction set there.
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
				ErrStaleAnchorMarker, StaleAnchorFileName, AnchorFileName, BoltDirName, board)
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
