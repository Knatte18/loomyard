// hubwide.go — the board-dir branch of the config verbs.
//
// A hub-wide module's file, and any module's repo default (--layer default), is read and written at the board dir and its edits are committed in _board through a commit seam, where a worktree-layer write syncs fabric.

package configcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// configDirs holds what a config verb addresses:
// the worktree's base dir, the hub's board dir, and the layer --layer selected.
type configDirs struct {
	worktree string
	board    string

	// layer is the --layer value: empty, or one of the three layer names.
	// Empty selects the module's own default, the worktree file of a per-worktree module and the hub file of a hub-wide one.
	layer string

	// hubWriteGuard, when set, is called before a hub-wide write and returns a refusal message, or "" to allow it.
	// A print or a per-worktree write never calls it, so those spawn no git.
	hubWriteGuard func() (string, error)
}

// hubWriteRefusal returns a refusal message when a hub-wide write from l must not go through, and "" when it may.
// A write is refused from a worktree that is not the hub's prime, and from a session whose strandName parses as a name carrying a slug, wherever its worktree resolves;
// the hub orch's slug-free name and an empty name pass.
// The refusal guards a mistaken write, not a determined agent: it reads the strand name the caller passes and nothing else about the session.
func hubWriteRefusal(l *lyxcwd.Location, strandName string) (string, error) {
	if agentname.TaskSlug(strandName) != "" {
		return fmt.Sprintf("a hub-wide config write is refused from the task session %s; way forward: the operator or the hub orch runs the write from the prime worktree", strandName), nil
	}
	isPrime, err := fabricengine.IsPrimeWorktree(l)
	if err != nil {
		return "", fmt.Errorf("resolve the hub's prime worktree: %w", err)
	}
	if !isPrime {
		return fmt.Sprintf("a hub-wide config write is refused from the task worktree %s; way forward: the operator or the hub orch runs the write from the hub's prime worktree", l.WorktreeName), nil
	}
	return "", nil
}

// refuseHubWrite runs the hub-wide write guard and, when it refuses or fails, writes the JSON error to out and reports true with the exit code to return.
// A nil guard allows the write.
func (d configDirs) refuseHubWrite(out io.Writer) (exitCode int, refused bool) {
	if d.hubWriteGuard == nil {
		return 0, false
	}
	refusal, err := d.hubWriteGuard()
	if err != nil {
		return output.Err(out, err.Error()), true
	}
	if refusal != "" {
		return output.Err(out, refusal), true
	}
	return 0, false
}

// dirsOf returns the config base dirs for the worktree l resolves to.
func dirsOf(l *lyxcwd.Location) configDirs {
	return configDirs{worktree: baseDirOf(l), board: fabricengine.BoardDir(l.HubPath)}
}

// layerNames are the values --layer takes.
var layerNames = []configengine.Layer{configengine.LayerDefault, configengine.LayerWorktree, configengine.LayerLocal}

// validLayer reports whether layer is empty or one of the three layer names.
func validLayer(layer string) bool {
	return layer == "" || slices.Contains(layerNames, configengine.Layer(layer))
}

// engineDirs returns the told geometry configengine addresses a layer file by.
func (d configDirs) engineDirs() configengine.Dirs {
	return configengine.Dirs{Anchor: d.worktree, Board: d.board}
}

// selectedLayer returns the layer a verb addresses for module.
// An empty --layer selects the module's own default, and --layer worktree on a hub-wide module is refused with the ways forward.
func (d configDirs) selectedLayer(module configreg.Module) (configengine.Layer, error) {
	switch {
	case d.layer == "" && module.HubWide:
		return configengine.LayerDefault, nil
	case d.layer == "":
		return configengine.LayerWorktree, nil
	case d.layer == string(configengine.LayerWorktree) && module.HubWide:
		return "", fmt.Errorf("%s is a hub-wide module and has no worktree file; way forward: --layer default from the hub's prime worktree, or --layer local", module.Name)
	}
	return configengine.Layer(d.layer), nil
}

// layerFile returns the selected layer's file of module and the layer.
func (d configDirs) layerFile(module configreg.Module) (string, configengine.Layer, error) {
	layer, err := d.selectedLayer(module)
	if err != nil {
		return "", "", err
	}
	path, applies := configengine.LayerFile(d.engineDirs(), module.Spec(), layer)
	if !applies {
		return "", "", fmt.Errorf("the %s layer of %s does not apply here", layer, module.Name)
	}
	return path, layer, nil
}

// hubCommitFunc runs write under the board write lock and commits the one module file it wrote in _board.
// It returns write's own error unwrapped when write fails, and any commit or push failure otherwise.
type hubCommitFunc func(module string, write func() error) error

// errHubFileChanged marks an editor edit refused because the hub file changed while the editor was open.
var errHubFileChanged = errors.New("hub config file changed while the editor was open")

// newHubCommit returns the hub-commit seam over the board repo at boardDir:
// a scoped commit of the module file under the board write lock, then a push.
func newHubCommit(boardDir string) hubCommitFunc {
	return func(module string, write func() error) error {
		bolt := fabricengine.NewBolt(boardDir)
		opts := fabricengine.EnvSyncOptions()
		_, _, err := bolt.CommitWritten("config: edit "+module, func() ([]string, error) {
			if err := write(); err != nil {
				return nil, err
			}
			return []string{configengine.ConfigFileRel(module)}, nil
		}, opts)
		if err != nil {
			return err
		}
		return bolt.Push(opts)
	}
}

// reconcileDefaultLayer reconciles every module's file under the hub's board dir against its template, seeded from the prime's anchor, and reports the reconcile envelope.
// A dry run writes nothing and takes no guard.
// apply is refused when guard refuses.
// Otherwise it holds the board write lock and runs a dry run first, so a file that fails to parse or migrate leaves every file unwritten.
// Then the reconcile writes, its files are committed in _board and the commit is pushed.
// A write that fails part-way is undone, restoring every config file of the board dir.
// A write that succeeded but was not committed, or committed but not pushed, is reported as an error naming the files left for the next board sync.
func reconcileDefaultLayer(l *lyxcwd.Location, out io.Writer, apply bool, guard func() (string, error)) int {
	board := fabricengine.ConfigDirs(l).Board
	if board == "" {
		return output.Err(out, "reconcile --layer default needs a hub's board dir, and this checkout is not a hub; way forward: run it with no --layer")
	}
	worktrees, err := fabricengine.CodeWorktrees(l.WorktreePath())
	if err != nil {
		return output.Err(out, fmt.Sprintf("list the hub's code worktrees: %v", err))
	}
	primeAnchor := ""
	for _, w := range worktrees {
		if w.Main {
			primeAnchor = w.Anchor
		}
	}
	opts := configsync.HubWideOptions{SeedDefaults: true}

	if !apply {
		results, err := configsync.ReconcileHubWideAt(board, primeAnchor, opts)
		if err != nil {
			return output.Err(out, fmt.Sprintf("reconcile: %v", err))
		}
		return output.Ok(out, reconcileEnvelope(false, results))
	}

	refusal, err := guard()
	if err != nil {
		return output.Err(out, err.Error())
	}
	if refusal != "" {
		return output.Err(out, refusal)
	}
	bolt := fabricengine.NewBolt(board)
	syncOpts := fabricengine.EnvSyncOptions()
	var results []configsync.Result
	var written []string
	_, committed, commitErr := bolt.CommitWritten("config: reconcile the board dir's files", func() ([]string, error) {
		if _, err := configsync.ReconcileHubWideAt(board, primeAnchor, opts); err != nil {
			return nil, err
		}
		prior, err := snapshotBoardConfig(board)
		if err != nil {
			return nil, err
		}
		applyOpts := opts
		applyOpts.Apply = true
		results, err = configsync.ReconcileHubWideAt(board, primeAnchor, applyOpts)
		if err != nil {
			results = nil
			return nil, restoreBoardConfig(prior, err)
		}
		for _, result := range results {
			if !result.Applied {
				continue
			}
			written = append(written, configengine.ConfigFileRel(result.Module))
			for _, legacy := range result.MigratedFrom {
				written = append(written, configengine.ConfigFileRel(legacy))
			}
		}
		return written, nil
	}, syncOpts)
	switch {
	case commitErr != nil && results == nil:
		return output.Err(out, fmt.Sprintf("reconcile: %v", commitErr))
	case commitErr != nil:
		return output.Err(out, fmt.Sprintf("reconciled %s in %s but did not commit them: %v; the next board sync commits them", strings.Join(written, ", "), board, commitErr))
	}
	if committed {
		if pushErr := bolt.Push(syncOpts); pushErr != nil {
			return output.Err(out, fmt.Sprintf("reconciled %s in %s and committed them in _board but did not push: %v; the next board push carries them", strings.Join(written, ", "), board, pushErr))
		}
	}
	return output.Ok(out, reconcileEnvelope(true, results))
}

// snapshotBoardConfig reads every file in the board dir's config dir and every registry module's config file there; an absent module file maps to a snapshot of absence.
func snapshotBoardConfig(board string) (map[string]fileSnapshot, error) {
	prior := make(map[string]fileSnapshot)
	configDir := configengine.ConfigDir(board)
	entries, err := os.ReadDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("snapshot config dir %s: %w", configDir, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			paths = append(paths, filepath.Join(configDir, entry.Name()))
		}
	}
	for _, module := range configreg.Modules() {
		paths = append(paths, configengine.ConfigFile(board, module.Name))
	}
	for _, path := range paths {
		snap, err := snapshotFile(path)
		if err != nil {
			return nil, err
		}
		prior[path] = snap
	}
	return prior, nil
}

// restoreBoardConfig puts back every file of prior that differs from its snapshot and returns cause, joined with any restore failure.
// A file already as snapshotted is left alone, so a failure before any write reports no restore failure.
func restoreBoardConfig(prior map[string]fileSnapshot, cause error) error {
	for path, snap := range prior {
		current, err := snapshotFile(path)
		if err == nil && current.existed == snap.existed && bytes.Equal(current.data, snap.data) {
			continue
		}
		cause = rollBack(path, snap, cause)
	}
	return cause
}

// fileSnapshot is a file's bytes, or its absence, taken so a failed write can put the file back.
type fileSnapshot struct {
	data    []byte
	existed bool
}

func snapshotFile(path string) (fileSnapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read config file: %w", err)
	}
	return fileSnapshot{data: data, existed: true}, nil
}

// rollBack puts path back to snap and returns cause.
// A failed restore is joined into the returned error, which then names path as possibly invalid.
func rollBack(path string, snap fileSnapshot, cause error) error {
	var restoreErr error
	if snap.existed {
		restoreErr = fsx.AtomicWriteBytes(path, snap.data)
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		restoreErr = err
	}
	if restoreErr != nil {
		return errors.Join(cause, fmt.Errorf("restoring %s failed, it may be invalid: %w", path, restoreErr))
	}
	return cause
}

// validateHubFile loads module's board file strictly, alone, so a malformed edit never reaches _board.
func validateHubFile(boardDir string, module configreg.Module) error {
	_, err := configengine.Load(configengine.Dirs{Board: boardDir}, module.Spec())
	return err
}

// reportHubWrite turns the outcome of a hub-commit round trip into the command's envelope.
// writeErr is the write closure's own error, which tells a refused write from a failed commit.
func reportHubWrite(out io.Writer, module, path string, commitErr, writeErr error, extra map[string]any) int {
	switch {
	case writeErr != nil:
		return output.Err(out, fmt.Sprintf("%v; %s unchanged", writeErr, path))
	case commitErr != nil:
		return output.Err(out, fmt.Sprintf("edited %s but it is uncommitted: %v; the next board sync commits it", path, commitErr))
	}
	fields := map[string]any{
		"module":  module,
		"message": fmt.Sprintf("edited and committed %s in _board", path),
	}
	for key, value := range extra {
		fields[key] = value
	}
	return output.Ok(out, fields)
}

// setHubWide writes pairs into a hub-wide module's file at the board dir and commits it in _board.
// The file is validated strictly before the commit, then loaded alone through the module's own loader, and restored on any failure.
// The loader refuses the write only with a failure the file did not already load with.
func setHubWide(dirs configDirs, out io.Writer, module configreg.Module, pairs []yamlengine.KV, commit hubCommitFunc) int {
	path := configengine.ConfigFile(dirs.board, module.Name)
	boardDirs := configengine.Dirs{Board: dirs.board}
	var preserved []string
	var writeErr error
	commitErr := commit(module.Name, func() error {
		snap, err := snapshotFile(path)
		if err == nil {
			before := module.CheckLoad(boardDirs)
			preserved, err = configengine.Set(boardDirs, module.Spec(), configengine.LayerDefault, pairs)
			if err == nil {
				err = validateHubFile(dirs.board, module)
			}
			if err == nil {
				err = newLoadFailure(module, boardDirs, before)
			}
			if err != nil {
				err = rollBack(path, snap, err)
			}
		}
		writeErr = err
		return err
	})
	extra := map[string]any{}
	if len(preserved) > 0 {
		extra["preserved"] = preserved
	}
	return reportHubWrite(out, module.Name, path, commitErr, writeErr, extra)
}

// editHubWide edits a hub-wide module's file through a staging copy under the worktree's .lyx,
// so the editor never holds the board write lock.
// Only the copy into the board dir, its validation and its commit run under the lock.
func editHubWide(dirs configDirs, out io.Writer, module configreg.Module, edit configengine.EditorFunc, commit hubCommitFunc) int {
	template := module.Template()
	path := configengine.ConfigFile(dirs.board, module.Name)
	staging := configengine.StagingFile(dirs.worktree, module.Name)

	snap, err := snapshotFile(path)
	if err != nil {
		return output.Err(out, err.Error())
	}
	base := []byte(template)
	if snap.existed {
		base = snap.data
	}
	if err := os.MkdirAll(filepath.Dir(staging), 0o755); err != nil {
		return output.Err(out, fmt.Errorf("create staging directory: %w", err).Error())
	}
	if err := os.WriteFile(staging, base, 0o644); err != nil {
		return output.Err(out, fmt.Errorf("write staging copy: %w", err).Error())
	}

	if err := configengine.EditPath(staging, template, edit); err != nil {
		_ = os.Remove(staging)
		if errors.Is(err, configengine.ErrAborted) {
			return output.Err(out, fmt.Sprintf("aborted: %s unchanged", path))
		}
		return output.Err(out, err.Error())
	}

	var writeErr error
	commitErr := commit(module.Name, func() error {
		writeErr = copyStagedToHub(path, staging, base, template, dirs.board, module)
		return writeErr
	})
	if errors.Is(writeErr, errHubFileChanged) {
		return output.Err(out, fmt.Sprintf("%s changed while the editor was open; your edit is kept at %s, re-apply it", path, staging))
	}
	_ = os.Remove(staging)
	return reportHubWrite(out, module.Name, path, commitErr, writeErr, nil)
}

// copyStagedToHub writes the staged bytes to the hub file when it still holds the bytes the staging copy was taken from,
// validates it strictly and restores the previous file on failure.
func copyStagedToHub(path, staging string, base []byte, template, boardDir string, module configreg.Module) error {
	snap, err := snapshotFile(path)
	if err != nil {
		return err
	}
	current := []byte(template)
	if snap.existed {
		current = snap.data
	}
	if !bytes.Equal(current, base) {
		return errHubFileChanged
	}
	staged, err := os.ReadFile(staging)
	if err != nil {
		return fmt.Errorf("read staging copy: %w", err)
	}
	if err := fsx.AtomicWriteBytes(path, staged); err != nil {
		return rollBack(path, snap, err)
	}
	if err := validateHubFile(boardDir, module); err != nil {
		return rollBack(path, snap, err)
	}
	return nil
}
