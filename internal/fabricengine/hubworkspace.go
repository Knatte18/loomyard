package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// hubWorkspaceExt is the file extension of the hub workspace file.
const hubWorkspaceExt = ".code-workspace"

// hubWorkspaceLockWait is how long a regeneration waits for the hub workspace lock before it fails.
const hubWorkspaceLockWait = 5 * time.Second

// hubWorkspaceLockFileName is the hub workspace lock's file name under the hub scratch directory.
const hubWorkspaceLockFileName = "hub-workspace.lock"

// hubWorkspaceBeforeLockHook, nil in production, runs just before the hub workspace lock is taken.
// A test that sets it must not be parallel.
var hubWorkspaceBeforeLockHook func()

// HubWorkspacePath returns <hub>/<RepoName>.code-workspace, the one VS Code workspace file of the hub.
func HubWorkspacePath(l *lyxcwd.Location) string {
	return filepath.Join(l.HubPath, l.RepoName+hubWorkspaceExt)
}

// HubWorkspaceLockPath returns the lock file that serializes regenerations of the hub workspace file: <HubScratchDir(hubPath)>/hub-workspace.lock.
func HubWorkspaceLockPath(hubPath string) string {
	return filepath.Join(HubScratchDir(hubPath), hubWorkspaceLockFileName)
}

// RegenerateHubWorkspace rewrites the hub workspace file from the hub's current worktrees and the prime's .vscode/settings.json, and returns its path.
// The file lists the prime, the board as Bolt, then every task pair, and is rewritten only when its bytes change; a rewrite is recorded in rec.
// The regeneration runs under the hub workspace lock, so two callers never interleave their listing and write.
// A prime that cannot be resolved, unreadable prime settings, a lock that stays held for the wait, or a failed write returns an error and writes nothing.
func RegenerateHubWorkspace(rec *Mutations, l *lyxcwd.Location) (string, error) {
	return regenerateHubWorkspace(rec, l, hubWorkspaceLockWait)
}

// regenerateAfterVerb regenerates the hub workspace file at the end of a fabric verb that changed the hub's worktrees.
// A failure is logged at Warn, naming the verb and the repair, and is never returned:
// a stale file is rewritten by the next regenerating verb or prime spawn, so it never fails or rolls back the verb.
func regenerateAfterVerb(rec *Mutations, l *lyxcwd.Location, verb string) {
	if _, err := RegenerateHubWorkspace(rec, l); err != nil {
		logger.Warn(`fabricengine: hub workspace regeneration failed; run "lyx fabric reconcile" once the cause is fixed`, "verb", verb, "error", err)
	}
}

// regenerateHubWorkspace is RegenerateHubWorkspace with the lock wait as a parameter.
// A hub without a board takes no lock and lists no Bolt folder.
func regenerateHubWorkspace(rec *Mutations, l *lyxcwd.Location, lockWait time.Duration) (string, error) {
	prime, err := PrimeName(l)
	if err != nil {
		return "", err
	}
	if retiredHubArtifactsPresent(l) {
		logger.Warn(`fabricengine: the hub still holds retired portal links or the old _launchers workspace file; run "lyx fabric reconcile" to remove them`)
	}
	settings, err := vscode.ReadSettings(filepath.Join(WorktreePath(l, prime), l.AnchorRel))
	if err != nil {
		return "", fmt.Errorf("read prime settings: %w", err)
	}
	if keys := vscode.RelativeSettingKeys(settings); len(keys) > 0 {
		logger.Warn("prime settings hold relative paths that now resolve against the hub directory, where the workspace file sits", "keys", keys)
	}

	if pathPresent(BoardDir(l.HubPath)) {
		release, err := lockHubWorkspace(rec, l.HubPath, lockWait)
		if err != nil {
			return "", err
		}
		defer release()
	}

	folders, err := hubWorkspaceFolders(l, prime)
	if err != nil {
		return "", fmt.Errorf("compute workspace folders: %w", err)
	}
	content, err := vscode.BuildWorkspace(folders, settings)
	if err != nil {
		return "", fmt.Errorf("build workspace: %w", err)
	}
	wrote, err := writeHubWorkspace(l, content)
	if err != nil {
		return "", fmt.Errorf("write workspace: %w", err)
	}
	path := HubWorkspacePath(l)
	if wrote {
		rec.Append(KindFileWritten, path, "")
	}
	return path, nil
}

// lockHubWorkspace takes the hub workspace lock exclusive, waiting at most wait, and returns its release function.
// The scratch directory is created through an os.Root at the hub when absent.
// A lock file this call brings into being is recorded in rec, whether or not the lock was won, since the file outlives the call.
// The lock is a leaf lock: nothing else is locked while it is held.
func lockHubWorkspace(rec *Mutations, hubPath string, wait time.Duration) (func(), error) {
	lockPath := HubWorkspaceLockPath(hubPath)
	existed := pathPresent(lockPath)

	root, err := os.OpenRoot(hubPath)
	if err != nil {
		return nil, fmt.Errorf("open hub root %s: %w", hubPath, err)
	}
	defer root.Close()
	scratchRel, err := hubRel(hubPath, HubScratchDir(hubPath))
	if err != nil {
		return nil, err
	}
	if err := root.MkdirAll(scratchRel, 0o755); err != nil {
		return nil, fmt.Errorf("create hub scratch dir %s: %w", scratchRel, err)
	}

	if hubWorkspaceBeforeLockHook != nil {
		hubWorkspaceBeforeLockHook()
	}
	held, acquired, err := lock.AcquireWriteLockWithin(lockPath, wait)
	if !existed && pathPresent(lockPath) {
		rec.Append(KindFileWritten, lockPath, "")
	}
	if err != nil {
		return nil, fmt.Errorf("take the hub workspace lock %s: %w", lockPath, err)
	}
	if !acquired {
		return nil, fmt.Errorf("the hub workspace lock %s is still held after %s; re-run once the other regeneration finishes", lockPath, wait)
	}
	return func() { _ = held.Release() }, nil
}

// hubWorkspaceFolders returns the workspace file's folders in order, each path relative to the hub and slash-separated.
// They are the prime's anchor dir, named after its branch (its directory name when HEAD is detached), then the board named Bolt, then every task pair named after its slug in byte order.
// A folder whose directory does not exist is left out, and a stat error other than not-exist is returned.
// A pair that fails to resolve, or whose worktree is not a direct child of the hub, is skipped with a debug log, so one half-made pair never drops the others.
func hubWorkspaceFolders(l *lyxcwd.Location, primeName string) ([]vscode.WorkspaceFolder, error) {
	var folders []vscode.WorkspaceFolder

	primeRoot := WorktreePath(l, primeName)
	primeAnchor := filepath.Join(primeRoot, l.AnchorRel)
	present, err := dirPresent(primeAnchor)
	if err != nil {
		return nil, err
	}
	if present {
		primeLabel := primeName
		branch, err := readBranch(primeRoot)
		if err != nil {
			return nil, err
		}
		if branch != "HEAD" {
			primeLabel = branch
		}
		folder, err := hubWorkspaceFolder(l.HubPath, primeLabel, primeAnchor)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}

	boardPresent, err := dirPresent(BoardDir(l.HubPath))
	if err != nil {
		return nil, err
	}
	if boardPresent {
		folder, err := hubWorkspaceFolder(l.HubPath, "Bolt", BoardDir(l.HubPath))
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}

	entries, err := List(l.AnchorPath())
	if err != nil {
		return nil, err
	}
	hub := filepath.Clean(l.HubPath)
	var pairs []vscode.WorkspaceFolder
	for _, entry := range entries {
		if entry.Main || entry.Prunable {
			continue
		}
		worktree := filepath.Clean(filepath.FromSlash(entry.Path))
		if filepath.Dir(worktree) != hub {
			logger.Debug("fabricengine: a warp worktree outside the hub is left out of the hub workspace", "path", worktree)
			continue
		}
		pairLocation, err := lyxcwd.ResolveWorktree(worktree)
		if err != nil {
			logger.Debug("fabricengine: a warp worktree that does not resolve is left out of the hub workspace", "path", worktree, "error", err)
			continue
		}
		anchor := pairLocation.AnchorPath()
		present, err := dirPresent(anchor)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		folder, err := hubWorkspaceFolder(l.HubPath, filepath.Base(worktree), anchor)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, folder)
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Name < pairs[j].Name })
	return append(folders, pairs...), nil
}

// hubWorkspaceFolder returns the folder entry for dir, its path relative to the hub and slash-separated.
// A filepath.Rel error propagates rather than collapsing to an empty path, because a wrong path written into the file is worse than a failed write.
func hubWorkspaceFolder(hubPath, name, dir string) (vscode.WorkspaceFolder, error) {
	rel, err := filepath.Rel(hubPath, dir)
	if err != nil {
		return vscode.WorkspaceFolder{}, fmt.Errorf("relate hub %s to workspace folder %s at %s: %w", hubPath, name, dir, err)
	}
	return vscode.WorkspaceFolder{Name: name, Path: filepath.ToSlash(rel)}, nil
}

// dirPresent reports whether dir exists; a stat error other than not-exist is returned.
func dirPresent(dir string) (bool, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat workspace folder %s: %w", dir, err)
	}
	return true, nil
}
