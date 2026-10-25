package fabricengine

import (
	"bytes"
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

// beamWorkspaceMark is the top-level key a generated beam workspace file carries, set to true and written first.
// VS Code ignores unknown keys, and the mark is what lets a regeneration tell its own beam file from an operator's file of the same name.
const beamWorkspaceMark = "lyx.generated"

// BeamWorkspacePath returns <hub>/<RepoName>-<beam>.code-workspace, the workspace file of the beam pair named beam, beside the hub's own.
func BeamWorkspacePath(l *lyxcwd.Location, beam string) string {
	return filepath.Join(l.HubPath, l.RepoName+"-"+beam+hubWorkspaceExt)
}

// HubWorkspaceLockPath returns the lock file that serializes regenerations of the hub workspace file: <HubScratchDir(hubPath)>/hub-workspace.lock.
func HubWorkspaceLockPath(hubPath string) string {
	return filepath.Join(HubScratchDir(hubPath), hubWorkspaceLockFileName)
}

// RegenerateHubWorkspace rewrites the hub workspace file and a workspace file per beam from the hub's current worktrees, the pairs' origin records and the prime's .vscode/settings.json, and returns the hub file's path.
// The hub file lists the prime, the board as Bolt, then every pair whose parent is the prime, beams included, and no pair under a beam.
// A beam's file, BeamWorkspacePath, lists the beam, its parent, Bolt and every pair whose parent is the beam, and a file of a beam that is gone is removed when it carries the generated mark.
// A pair with no readable origin record is listed in the hub file as a pair of the prime.
// Each file is rewritten only when its bytes change; a write or removal is recorded in rec.
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
	primeLoc, err := lyxcwd.NamedWorktree(l.HubPath, prime)
	if err != nil {
		return "", fmt.Errorf("read the prime's anchor: %w", err)
	}
	if retiredHubArtifactsPresent(primeLoc) {
		logger.Warn(`fabricengine: the hub still holds retired portal links or the old _launchers workspace file; run "lyx fabric reconcile" to remove them`)
	}
	settings, err := vscode.ReadSettings(filepath.Join(WorktreePath(l, prime), primeLoc.AnchorRel))
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

	layout, err := hubWorkspaceFolders(l, prime)
	if err != nil {
		return "", fmt.Errorf("compute workspace folders: %w", err)
	}
	content, err := vscode.BuildWorkspace(layout.hub, settings)
	if err != nil {
		return "", fmt.Errorf("build workspace: %w", err)
	}
	path := HubWorkspacePath(l)
	wrote, err := writeHubWorkspace(l, path, content)
	if err != nil {
		return "", fmt.Errorf("write workspace: %w", err)
	}
	if wrote {
		rec.Append(KindFileWritten, path, "")
	}

	kept := map[string]bool{}
	for _, beam := range layout.beams {
		kept[beam.slug] = true
		beamContent, err := vscode.BuildWorkspace(beam.folders, settings)
		if err != nil {
			return "", fmt.Errorf("build workspace of beam %s: %w", beam.slug, err)
		}
		beamPath := BeamWorkspacePath(l, beam.slug)
		wrote, err := writeHubWorkspace(l, beamPath, markBeamWorkspace(beamContent))
		if err != nil {
			return "", fmt.Errorf("write workspace of beam %s: %w", beam.slug, err)
		}
		if wrote {
			rec.Append(KindFileWritten, beamPath, "")
		}
	}
	for _, slug := range layout.unrecorded {
		kept[slug] = true
	}
	if err := removeStaleBeamWorkspaces(rec, l, kept); err != nil {
		return "", fmt.Errorf("remove stale beam workspaces: %w", err)
	}
	return path, nil
}

// markBeamWorkspace returns the workspace file bytes content with beamWorkspaceMark set to true as the first top-level key.
// content is the output of vscode.BuildWorkspace, which opens with a bare "{" line.
func markBeamWorkspace(content []byte) []byte {
	marked := []byte("{\n  \"" + beamWorkspaceMark + "\": true,\n")
	return append(marked, bytes.TrimPrefix(content, []byte("{\n"))...)
}

// errHubWorkspaceLockHeld is the sentinel lockHubWorkspace's error wraps when another regeneration holds the lock past the wait.
var errHubWorkspaceLockHeld = errors.New("the hub workspace lock is held")

// lockHubWorkspace takes the hub workspace lock exclusive, waiting at most wait, and returns its release function.
// A zero wait makes one attempt.
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
	var held *lock.FileLock
	var acquired bool
	if wait <= 0 {
		held, acquired, err = lock.TryAcquireWriteLock(lockPath)
	} else {
		held, acquired, err = lock.AcquireWriteLockWithin(lockPath, wait)
	}
	if !existed && pathPresent(lockPath) {
		rec.Append(KindFileWritten, lockPath, "")
	}
	if err != nil {
		return nil, fmt.Errorf("take the hub workspace lock %s: %w", lockPath, err)
	}
	if !acquired {
		return nil, fmt.Errorf("the hub workspace lock %s is still held after %s; re-run once the other regeneration finishes: %w", lockPath, wait, errHubWorkspaceLockHeld)
	}
	return func() { _ = held.Release() }, nil
}

// workspaceLayout is what a regeneration writes: the hub file's folders and one entry per beam.
type workspaceLayout struct {
	// hub is the hub file's folders in order.
	hub []vscode.WorkspaceFolder
	// beams are the beam files, ordered by slug.
	beams []beamWorkspace
	// unrecorded are the slugs of listed pairs whose origin record could not be read, whose beam file is kept if one exists.
	unrecorded []string
}

// beamWorkspace is one beam's file: the beam's slug and its folders in order.
type beamWorkspace struct {
	slug    string
	folders []vscode.WorkspaceFolder
}

// workspacePair is a listed pair: its slug, folder, the slug of its live parent pair ("" for the prime's) and whether it is a beam.
type workspacePair struct {
	slug   string
	folder vscode.WorkspaceFolder
	parent string
	beam   bool
}

// hubWorkspaceFolders returns the folders of the hub workspace file and of each beam file, each path relative to the hub and slash-separated.
// The hub file holds the prime's anchor dir, named after its branch (its directory name when HEAD is detached), then the board named Bolt, then every pair whose parent is the prime, named after its slug in byte order.
// A beam file holds the beam, its parent, Bolt, then every pair whose parent is the beam.
// A pair's parent is the ParentWorktree of its origin record; the prime when that is empty, names the prime or names no listed pair, and when the record is absent or cannot be read, the latter with a debug log.
// A folder whose directory does not exist is left out, and a stat error other than not-exist is returned.
// A pair that fails to resolve, or whose worktree is not a direct child of the hub, is skipped with a debug log, so one half-made pair never drops the others.
func hubWorkspaceFolders(l *lyxcwd.Location, primeName string) (workspaceLayout, error) {
	var layout workspaceLayout
	var primeFolder, boltFolder *vscode.WorkspaceFolder

	primeRoot := WorktreePath(l, primeName)
	primeLoc, err := lyxcwd.NamedWorktree(l.HubPath, primeName)
	if err != nil {
		return workspaceLayout{}, fmt.Errorf("read the prime's anchor: %w", err)
	}
	primeAnchor := filepath.Join(primeRoot, primeLoc.AnchorRel)
	present, err := dirPresent(primeAnchor)
	if err != nil {
		return workspaceLayout{}, err
	}
	if present {
		primeLabel := primeName
		branch, err := readBranch(primeRoot)
		if err != nil {
			return workspaceLayout{}, err
		}
		if branch != "HEAD" {
			primeLabel = branch
		}
		folder, err := hubWorkspaceFolder(l.HubPath, primeLabel, primeAnchor)
		if err != nil {
			return workspaceLayout{}, err
		}
		primeFolder = &folder
	}

	boardPresent, err := dirPresent(BoardDir(l.HubPath))
	if err != nil {
		return workspaceLayout{}, err
	}
	if boardPresent {
		folder, err := hubWorkspaceFolder(l.HubPath, "Bolt", BoardDir(l.HubPath))
		if err != nil {
			return workspaceLayout{}, err
		}
		boltFolder = &folder
	}

	entries, err := List(l.AnchorPath())
	if err != nil {
		return workspaceLayout{}, err
	}
	hub := filepath.Clean(l.HubPath)
	var pairs []workspacePair
	recordedParent := map[string]string{}
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
			return workspaceLayout{}, err
		}
		if !present {
			continue
		}
		slug := filepath.Base(worktree)
		folder, err := hubWorkspaceFolder(l.HubPath, slug, anchor)
		if err != nil {
			return workspaceLayout{}, err
		}
		pair := workspacePair{slug: slug, folder: folder}
		origin, found, err := ReadOriginFor(l, slug)
		if err != nil {
			logger.Debug("fabricengine: a pair whose origin record cannot be read is listed in the hub workspace as a pair of the prime", "slug", slug, "error", err)
			layout.unrecorded = append(layout.unrecorded, slug)
		} else if found {
			pair.beam = origin.Beam
			recordedParent[slug] = origin.ParentWorktree
		}
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].slug < pairs[j].slug })

	// A parent that is the prime, absent or no listed pair makes the pair the prime's.
	listed := map[string]*workspacePair{}
	for i := range pairs {
		listed[pairs[i].slug] = &pairs[i]
	}
	for i := range pairs {
		parent := recordedParent[pairs[i].slug]
		if _, ok := listed[parent]; ok && parent != pairs[i].slug {
			pairs[i].parent = parent
		}
	}

	if primeFolder != nil {
		layout.hub = append(layout.hub, *primeFolder)
	}
	if boltFolder != nil {
		layout.hub = append(layout.hub, *boltFolder)
	}
	for _, pair := range pairs {
		if pair.parent == "" {
			layout.hub = append(layout.hub, pair.folder)
		}
	}
	for _, beam := range pairs {
		if !beam.beam {
			continue
		}
		file := beamWorkspace{slug: beam.slug, folders: []vscode.WorkspaceFolder{beam.folder}}
		if beam.parent != "" {
			file.folders = append(file.folders, listed[beam.parent].folder)
		} else if primeFolder != nil {
			file.folders = append(file.folders, *primeFolder)
		}
		if boltFolder != nil {
			file.folders = append(file.folders, *boltFolder)
		}
		for _, member := range pairs {
			if member.parent == beam.slug {
				file.folders = append(file.folders, member.folder)
			}
		}
		layout.beams = append(layout.beams, file)
	}
	return layout, nil
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
