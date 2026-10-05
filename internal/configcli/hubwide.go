// hubwide.go — the hub-wide branch of the config verbs.
//
// A hub-wide module (configreg.Module.HubWide) is read and written at the board dir and its edits are committed in _board through a commit seam, where a per-worktree module syncs fabric.

package configcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// configDirs holds the two base dirs a config verb addresses:
// the worktree's, for per-worktree modules, and the hub's board dir, for hub-wide ones.
type configDirs struct {
	worktree string
	board    string
}

// dirsOf returns the config base dirs for the worktree l resolves to.
func dirsOf(l *lyxcwd.Location) configDirs {
	return configDirs{worktree: baseDirOf(l), board: fabricengine.BoardDir(l.HubPath)}
}

// baseFor returns the base dir holding module's config file.
func (d configDirs) baseFor(module configreg.Module) string {
	if module.HubWide {
		return d.board
	}
	return d.worktree
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

// validateHubFile loads module's hub file strictly, so a malformed edit never reaches _board.
func validateHubFile(boardDir string, module configreg.Module) error {
	_, err := configengine.Load(boardDir, module.Name, []byte(module.Template()), module.OpenMaps...)
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
// The file is validated strictly before the commit and restored on any failure.
func setHubWide(dirs configDirs, out io.Writer, module configreg.Module, pairs []yamlengine.KV, commit hubCommitFunc) int {
	path := configengine.ConfigFile(dirs.board, module.Name)
	var preserved []string
	var writeErr error
	commitErr := commit(module.Name, func() error {
		snap, err := snapshotFile(path)
		if err == nil {
			preserved, err = configengine.Set(dirs.board, module.Name, module.Template(), pairs, module.OpenMaps...)
			if err == nil {
				err = validateHubFile(dirs.board, module)
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
