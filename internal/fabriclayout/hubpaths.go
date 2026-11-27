// hubpaths.go holds the hub's directory and file names and the pure functions that build paths from them.
// It also holds the predicates over hub-level entry names.

package fabriclayout

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// BoardWriteLockFile is the name of the lock file that serialises every write to the board directory.
// Every holder of that lock uses this value, so the name is declared once.
const BoardWriteLockFile = "board.lock"

// BoardPushLockFile is the name of the Bolt checkout's absorbing push lock.
// It is held across a board sync's whole commit-and-push loop.
const BoardPushLockFile = "board.push.lock"

// BoltDirName is the name of the Bolt checkout inside a hub.
// Use BoltDir(hub) to obtain the full path.
const BoltDirName = lymdirs.BoltDirName

// BoardDirName is the name of the directory holding the board's own data inside the hub.
// The board lives in the Bolt checkout, so the name is the Bolt checkout's name.
// Use BoardDir(hub) to obtain the full path.
const BoardDirName = BoltDirName

// HubSuffix is the suffix appended to a repo name to form the hub container directory:
// "loomyard" becomes "loomyard-LYMHUB".
// Use HubPath(parent, name) to obtain the full path.
// internal/lymcwd keeps its own private copy because Location.RepoName derives from it.
const HubSuffix = "-LYMHUB"

// WorktreeStagingPrefix prefixes the name of a worktree staging directory.
// A new worktree is created in such a directory before it moves to its final path.
const WorktreeStagingPrefix = ".fabric-wt-staging-"

// RecordsLockDirName is the name of the lock directory inside a records worktree.
const RecordsLockDirName = ".weft"

// TeardownLockDirName is the directory under the hub scratch directory that holds one teardown lock file per pair slug.
const TeardownLockDirName = "teardown"

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

// StencilsSubtreeRel returns the slash-separated, board-repo-relative prefix of the stencils subtree.
func StencilsSubtreeRel() string {
	return path.Join(lymdirs.LymDirName, stencilsDirName)
}

// SpecsSubtreeRel returns the slash-separated, board-repo-relative prefix of the deployed-specs subtree.
func SpecsSubtreeRel() string {
	return path.Join(lymdirs.LymDirName, specsDirName)
}

// HubScratchDir returns the hub-wide, machine-local scratch tree shared by every worktree in the hub:
// the Bolt checkout's `.lym`.
// It is the ephemeral counterpart of the durable `_lym` tree in the Bolt checkout.
// It is always a real directory, never a junction, and HubScratchDir is the sole constructor of this path.
func HubScratchDir(hub string) string {
	return filepath.Join(BoltDir(hub), lymdirs.DotLymDirName)
}

// AdoptScratchDir returns the directory that holds the adopt check's files.
// It sits beside the log of the test suite an adopt starts.
// It is `.lym/adopt` in the Bolt checkout.
func AdoptScratchDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), lymdirs.AdoptDirName)
}

// ClaudeAccountsDir returns the directory that holds the Claude account file and usage records.
// It is `.lym/claude` in the Bolt checkout, a cache a fresh machine starts without.
func ClaudeAccountsDir(hub string) string {
	return filepath.Join(HubScratchDir(hub), lymdirs.ClaudeAccountsDirName)
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

// portalsDirName and launchersDirName are hub-level container names that no verb writes.
// They stay reserved, so no pair can claim the name of an entry a hub may hold.
const (
	portalsDirName   = "_portals"
	launchersDirName = "_launchers"
)

// HubReservedNames returns the hub-structural names that must never wire a per-worktree junction:
// the Bolt checkout, `_portals` and `_launchers`.
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
	for _, structuralName := range StructuralCommittedDirs() {
		if name == structuralName {
			return true
		}
	}
	for _, structuralName := range StructuralNeverCommittedDirs() {
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

// orphanFirstSightFileName is the file under the hub's orphan directory that holds each orphan's first-sight time.
const orphanFirstSightFileName = "first-sight.json"

// OrphanFirstSightPath returns the machine-local record of when the dobby first noticed each orphan pair of the hub:
// <HubScratchDir(hubPath)>/<lymdirs.OrphanDirName>/first-sight.json.
// A pair removal does not take it, since it sits under the hub's `.lym` and never in a pair.
func OrphanFirstSightPath(hubPath string) string {
	return filepath.Join(HubScratchDir(hubPath), lymdirs.OrphanDirName, orphanFirstSightFileName)
}

// TeardownLockPath returns the lock file that orders slug's pair against its pushes:
// <HubScratchDir(hubPath)>/teardown/<slug>.lock.
// A push of the pair takes it shared; a mutation of the pair takes it exclusive.
func TeardownLockPath(hubPath, slug string) string {
	return filepath.Join(HubScratchDir(hubPath), TeardownLockDirName, slug+".lock")
}

// HandOverMarkPath returns the file that holds this machine's hand-over mark of slug:
// <HubScratchDir(hubPath)>/<lymdirs.HandOverDirName>/<slug>.json.
func HandOverMarkPath(hubPath, slug string) string {
	return filepath.Join(HubScratchDir(hubPath), lymdirs.HandOverDirName, slug+".json")
}

// RecordsLockDirPath returns the lock directory path inside the records worktree at recordsWorktreePath.
// It builds the path only and does not create the directory.
func RecordsLockDirPath(recordsWorktreePath string) string {
	return filepath.Join(recordsWorktreePath, RecordsLockDirName)
}

// moveRecordFile is the name of the move record file in a records worktree's lock directory.
const moveRecordFile = "anchor-move.json"

// MoveRecordPath returns where the move record of the weft worktree at weftPath lives.
// It sits in the weft's lock directory, which git excludes and no move touches.
func MoveRecordPath(weftPath string) string {
	return filepath.Join(RecordsLockDirPath(weftPath), moveRecordFile)
}

// originRecordDirName and originRecordFileName are the origin record's path segments under the durable lym directory.
// This package declares them alone:
// a module's durable-storage subdirectory is its own private relative-path constant.
const (
	originRecordDirName  = "fabric"
	originRecordFileName = "origin.json"
)

// OriginRecordRel returns the origin record's path relative to a worktree's anchor.
// Every path accessor and commit pathspec for the record is built from it,
// so the segments are joined in one place.
func OriginRecordRel() string {
	return filepath.Join(lymdirs.LymDirName, originRecordDirName, originRecordFileName)
}

// InWorktreeStaging reports whether path lies under a worktree staging directory.
// A worktree being created sits in such a directory until it moves to its final path.
func InWorktreeStaging(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, WorktreeStagingPrefix) {
			return true
		}
	}
	return false
}
