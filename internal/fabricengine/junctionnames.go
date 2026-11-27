// junctionnames.go turns the repo-wide fabric.yaml pathspec into the wired name set the junction primitives use.
// It applies the hub-reserved-name guard and folds in the two structural directory sets, which never come from config.
// Wiring callers take names only from junctionNames, WiredNames or RepoWiredNames,
// so the guard cannot be skipped at a call site.
// reconcile and status callers read the name set from the repo-wide `weft:main` base:
// the board directory ConfigDirs reports, not each pair's own weft base.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// dedupUnion concatenates every slice in groups, keeping only the first occurrence of each name.
// Names keep their first-occurrence order across the whole call.
// The dedup is required, because a deployed `pathspec: _lym _extra` fabric.yaml delivers `_lym` twice in one call:
// from fabriclayout.StructuralCommittedDirs and from cfg.Dirs().
// Without dedup, the duplicate would reach WarpJunctions, ScopedPathspec and status output.
func dedupUnion(groups ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, group := range groups {
		for _, name := range group {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// pathspecNames returns the pathspec/commit-routing set for cfg:
// fabriclayout.StructuralCommittedDirs unioned with cfg's hub-reserved-filtered directory names.
// It never contains a fabriclayout.StructuralNeverCommittedDirs entry:
// fabric's own weft sync and Commit's classification build from this set.
// Committing a never-committed path would be a bug.
func pathspecNames(cfg Config) []string {
	return dedupUnion(fabriclayout.StructuralCommittedDirs(), filterHubReserved(cfg.Dirs()))
}

// PathspecNames loads the fabric config under dirs and returns its pathspec/commit-routing name set.
// Callers outside this package use it for weft sync.
// That sync must never fall back to the raw, unfiltered directory list, because the config may omit `_lym`.
func PathspecNames(dirs configengine.Dirs) ([]string, error) {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return nil, err
	}
	return pathspecNames(cfg), nil
}

// BoardWriteLockPath returns the absolute path to the Bolt write lock file of hub, creating its directory.
func BoardWriteLockPath(hub string) (string, error) {
	return BoltLockPath(fabriclayout.BoltDir(hub), fabriclayout.BoardWriteLockFile)
}

// BoltLockPath returns the path of the lock file named name in the Bolt checkout at boltDir.
// The file sits in the checkout's never-tracked lock directory, which BoltLockPath creates.
// A lock there never shows as a change in the board repository, so a stage-all board commit never sweeps it in.
func BoltLockPath(boltDir, name string) (string, error) {
	path := lock.TrackedLockPath(boltDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("fabricengine: create the Bolt lock directory of %s: %w", boltDir, err)
	}
	return path, nil
}

// ConfigDirs returns the directories that a layered config load for the location draws from:
// the anchor, plus the hub's Bolt checkout and the paired weft checkout when each exists as a directory.
// A git checkout that is not a hub has no Bolt checkout and no weft checkout, so Board and Records stay empty.
// A stat failure other than absence is treated as absence and logged for either checkout:
// a loader then degrades or refuses under its own base-layer rule.
func ConfigDirs(l *lymcwd.Location) configengine.Dirs {
	return configengine.Dirs{
		Anchor:  l.AnchorPath(),
		Board:   existingDirOrLog(fabriclayout.BoltDir(l.HubPath), "fabricengine: Bolt checkout not statable, loading the standalone config stack"),
		Records: existingDirOrLog(RecordsWorktree(l), "fabricengine: weft checkout not statable, telling no records checkout"),
	}
}

// existingDirOrLog returns path when it exists as a directory and the empty string otherwise.
// A stat failure other than absence is logged as a warning, with the path and the error.
func existingDirOrLog(path, warning string) string {
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return path
	case err != nil && !os.IsNotExist(err):
		logger.Warn(warning, "path", path, "error", err)
	}
	return ""
}

// filterHubReserved drops every name in names that is a hub-reserved name, preserving the input order of the rest.
// This is the wiring guard.
// A hub-structural name in the fabric config's pathspec must never wire a per-worktree junction.
// Such a junction would collide with the hub-level path of the same name.
func filterHubReserved(names []string) []string {
	reserved := make(map[string]bool)
	for _, r := range fabriclayout.HubReservedNames() {
		reserved[r] = true
	}

	kept := make([]string, 0, len(names))
	for _, name := range names {
		if reserved[name] {
			continue
		}
		kept = append(kept, name)
	}
	return kept
}

// junctionNames loads the fabric config under dirs and returns its wired name-set.
// The set is the structural directories plus the pathspec directory names.
// The wiring guard filters only the pathspec names:
// a structural name is never filtered, which is why `.lym` must stay out of the reserved names.
// The set is wider than the names used for pathspecs and commit routing.
// The difference is the structural never-committed directories:
// they are junctioned and excluded, but never named in a git invocation.
//
// It returns a nil set and the error when the config fails to load, with no fallback default.
func junctionNames(dirs configengine.Dirs) ([]string, error) {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return nil, err
	}
	return dedupUnion(fabriclayout.StructuralCommittedDirs(), fabriclayout.StructuralNeverCommittedDirs(), filterHubReserved(cfg.Dirs())), nil
}

// WiredNames returns the wired name-set of the fabric config under dirs, for callers outside this package.
// It applies the wiring guard to the config's names, so out-of-package callers get the same filtered set.
//
// Fabric health reports a worktree as unhealthy while its `.lym` junction is missing.
// It stays unhealthy until `lym fabric reconcile` runs.
func WiredNames(dirs configengine.Dirs) ([]string, error) {
	return junctionNames(dirs)
}

// RepoWiredNames returns the wired name-set of the repo-wide fabric config, read through the given location.
// Every worktree therefore converges on the one repo-wide pathspec.
func RepoWiredNames(l *lymcwd.Location) ([]string, error) {
	return WiredNames(ConfigDirs(l))
}
