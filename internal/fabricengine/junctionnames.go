// junctionnames.go turns the repo-wide fabric.yaml pathspec into the wired name set the junction primitives use.
// It applies the hub-reserved-name guard and holds the two structural directory sets, which never come from config.
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
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// structuralCommittedDirs is the durable, git-tracked structural directory set: `_lym` alone.
// Its membership is structural, never config or env-overridable (the Cwd Resolution Invariant):
// every lym module fails without `_lym`, so an operator-editable fabric.yaml `pathspec` must not decide it.
// Injecting it in code means no config value can tear away the durable tree.
var structuralCommittedDirs = []string{lymdirs.LymDirName}

// structuralNeverCommittedDirs is the machine-local, never-git-tracked structural directory set: `.lym` alone.
// Its membership is structural, never config or env-overridable (the Cwd Resolution Invariant):
// every lym module fails without `.lym`, so an operator-editable fabric.yaml `pathspec` must not decide it.
// Injecting it in code means no config value can leave machine-local scratch unwired.
var structuralNeverCommittedDirs = []string{lymdirs.DotLymDirName}

// dedupUnion concatenates every slice in groups, keeping only the first occurrence of each name.
// Names keep their first-occurrence order across the whole call.
// The dedup is required, because a deployed `pathspec: _lym _extra` fabric.yaml delivers `_lym` twice in one call:
// from structuralCommittedDirs and from cfg.Dirs().
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
// structuralCommittedDirs unioned with cfg's hub-reserved-filtered directory names.
// It never contains a structuralNeverCommittedDirs entry:
// fabric's own weft sync and Commit's classification build from this set.
// Committing a never-committed path would be a bug.
func pathspecNames(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs()))
}

// slugReservedNames returns the full slug-reservation set for cfg:
// both structural directory sets, cfg.Dirs() taken raw, and HubReservedNames().
// cfg.Dirs() is not routing-filtered:
// a worktree slug must be refused for every one of these names, whether or not the name would wire a junction.
func slugReservedNames(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, cfg.Dirs(), HubReservedNames())
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

// BoardWriteLockFile is the name of the lock file that serialises every write to the board directory.
// Every holder of that lock uses this value, so the name is declared once.
const BoardWriteLockFile = "board.lock"

// BoardPushLockFile is the name of the Bolt checkout's absorbing push lock.
// It is held across a board sync's whole commit-and-push loop.
const BoardPushLockFile = "board.push.lock"

// BoardWriteLockPath returns the absolute path to the Bolt write lock file of hub, creating its directory.
func BoardWriteLockPath(hub string) (string, error) {
	return BoltLockPath(BoltDir(hub), BoardWriteLockFile)
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

// BoltDirName is the name of the Bolt checkout inside a hub.
// Use BoltDir(hub) to obtain the full path.
const BoltDirName = lymcwd.BoltDirName

// BoardDirName is the name of the directory holding the board's own data inside the hub.
// The board lives in the Bolt checkout, so the name is the Bolt checkout's name.
// Use BoardDir(hub) to obtain the full path.
const BoardDirName = BoltDirName

// HubSuffix is the suffix appended to a repo name to form the hub container directory:
// "loomyard" becomes "loomyard-LYMHUB".
// Use HubPath(parent, name) to obtain the full path.
// internal/lymcwd keeps its own private copy because Location.RepoName derives from it.
const HubSuffix = "-LYMHUB"

// BoltDir returns the absolute path to the Bolt checkout inside hub.
func BoltDir(hub string) string {
	return filepath.Join(hub, BoltDirName)
}

// BoardDir returns the absolute path to the board's own data inside hub:
// board.json, its overview and design files, and its decision records.
// Everything else the Bolt checkout holds lives under BoltDir.
func BoardDir(hub string) string {
	return filepath.Join(hub, BoardDirName)
}

// ConfigDirs returns the directories that a layered config load for the location draws from:
// the anchor, plus the hub's Bolt checkout and the paired weft checkout when each exists as a directory.
// A git checkout that is not a hub has no Bolt checkout and no weft checkout, so Board and Records stay empty.
// A stat failure other than absence is treated as absence and logged for either checkout:
// a loader then degrades or refuses under its own base-layer rule.
func ConfigDirs(l *lymcwd.Location) configengine.Dirs {
	return configengine.Dirs{
		Anchor:  l.AnchorPath(),
		Board:   existingDirOrLog(BoltDir(l.HubPath), "fabricengine: Bolt checkout not statable, loading the standalone config stack"),
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

// stencilsDirName is the name of the stencils directory nested under the Bolt checkout's `_lym` component.
// It stays unexported: "stencils" is not a policed geometry token, so the geometry policy does not track it.
const stencilsDirName = "stencils"

// StencilsDir returns the hub-wide stencils directory, shared by every worktree, under the Bolt checkout's `_lym`.
// internal/stencilstore receives this value as its baseDir and never joins the Bolt directory name or `_lym` itself.
func StencilsDir(hub string) string {
	return filepath.Join(BoltDir(hub), lymdirs.LymDirName, stencilsDirName)
}

// specsDirName is the name of the deployed-specs directory nested under the Bolt checkout's `_lym` component.
// It stays unexported: "specs" is not a policed geometry token, so the geometry policy does not track it.
const specsDirName = "specs"

// SpecsDir returns the hub-wide deployed-specs directory, shared by every worktree, under the Bolt checkout's `_lym`.
// internal/stencilstore receives this value as its baseDir and never joins the intermediate components itself.
// It holds the normative docs that producer prompts point at.
func SpecsDir(hub string) string {
	return filepath.Join(BoltDir(hub), lymdirs.LymDirName, specsDirName)
}

// HubScratchDir returns the hub-wide, machine-local scratch tree shared by every worktree in the hub:
// the Bolt checkout's `.lym`.
// It is the ephemeral counterpart of the durable `_lym` tree in the Bolt checkout.
// It is always a real directory, never a junction, and HubScratchDir is the sole constructor of this path.
func HubScratchDir(hub string) string {
	return filepath.Join(BoltDir(hub), lymdirs.DotLymDirName)
}

// AdoptScratchDir returns the directory that holds the adopt record, its lock and the worker's output.
// It sits beside the adopt check's record, output and lock, and beside the log of the test suite an adopt starts.
// It is `.lym/adopt` in the Bolt checkout.
func AdoptScratchDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), lymdirs.AdoptDirName)
}

// FrontTaskScratchDir returns the directory that holds the front-task record and its lock.
// It also holds the output of the actions a front task runs.
// It is `.lym/fronttask` in the Bolt checkout.
func FrontTaskScratchDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), lymdirs.FrontTaskDirName)
}

// HubLogsDir returns the hub-level directory where the shared per-hub reed server writes its runtime log.
// It is `logs` in the Bolt checkout's `.lym`.
// It is hub-anchored, so one server per hub resolves to one deterministic place.
// The path is derived in this one function rather than re-joined at each caller.
func HubLogsDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), "logs")
}

// LogArchiveDir returns the hub-level directory where a torn-down pair's logs are archived and pruned.
// It is `log-archive` in the Bolt checkout's `.lym`.
// The pair teardown's copy and its prune both resolve the root through it.
// It is never tracked.
func LogArchiveDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), "log-archive")
}

// RunProcDir returns the hub-level directory holding the owner record of the processes a run starts.
// It is `runproc` in the Bolt checkout's `.lym`.
// It is handed to internal/runproc, which derives no path of its own, and is never tracked.
func RunProcDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), lymdirs.RunProcDirName)
}

// HubPath returns the absolute path to the hub container directory for the given repo name inside parent.
func HubPath(parent, name string) string {
	return filepath.Join(parent, name+HubSuffix)
}

// HubReservedNames returns the hub-structural names that must never wire a per-worktree junction:
// the Bolt checkout, `_portals` and `_launchers`.
// `_portals` and `_launchers` are retired but stay reserved.
// `.lym` is not a member: listing it would keep it from being wired into each worktree.
// It would also keep it from ever being torn down.
// It is reserved through a separate structural set.
func HubReservedNames() []string {
	return []string{BoltDirName, portalsDirName, launchersDirName}
}

// IsReservedHubName reports whether name is a hub-level entry name that a worktree slug must never claim.
// It covers the hub-structural names and the structural directory names.
// The caller's junctionNames, which come from fabric config, are reserved too.
// It refuses `_lym` and `.lym` even when the config names neither.
func IsReservedHubName(name string, junctionNames []string) bool {
	for _, reserved := range HubReservedNames() {
		if name == reserved {
			return true
		}
	}
	for _, structuralName := range structuralCommittedDirs {
		if name == structuralName {
			return true
		}
	}
	for _, structuralName := range structuralNeverCommittedDirs {
		if name == structuralName {
			return true
		}
	}
	for _, junctionName := range junctionNames {
		if name == junctionName {
			return true
		}
	}
	return false
}

// filterHubReserved drops every name in names that is a hub-reserved name, preserving the input order of the rest.
// This is the wiring guard.
// A hub-structural name in the fabric config's pathspec must never wire a per-worktree junction.
// Such a junction would collide with the hub-level path of the same name.
func filterHubReserved(names []string) []string {
	reserved := make(map[string]bool)
	for _, r := range HubReservedNames() {
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
	return dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs())), nil
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
