// planbaseline.go keeps the content behind every plan hash webster records, and writes it back on demand.
// State.PlanFileHashes records only hashes, and a plan directory is not tracked by git on a task branch (nor exists in any repository for a standalone run),
// so without a stored copy no command could restore a plan edited since the run recorded it.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// planBaselineDirName is the directory under WebsterDir holding one file per recorded plan content, named by its hex SHA-256.
const planBaselineDirName = "plan-baseline"

// ErrPlanBaselineMissing is returned by RestorePlan when a changed plan file has no stored copy to restore from, or the state recorded no plan hashes at all.
var ErrPlanBaselineMissing = errors.New("websterengine: plan baseline copy missing")

// planBaselineWayForward is the way forward named when a plan cannot be restored from the store.
const planBaselineWayForward = `way forward: reset the branch to the run's start commit with git and run "lyx webster run --fresh"`

// planBaselinePath returns the stored copy's path for a content hash.
func planBaselinePath(websterDir, hash string) string {
	return filepath.Join(websterDir, planBaselineDirName, hash)
}

// storePlanBaseline writes each plan file's bytes to its hash's file under websterDir when that file is absent.
// The store is content-addressed, so a repeat write is a no-op and no stored copy is ever wrong.
// An empty websterDir is a wiring error, returned rather than resolved against the working directory.
func storePlanBaseline(websterDir, planDir string, hashes map[string]string) error {
	if websterDir == "" {
		return errors.New("websterengine: store plan baseline: empty webster dir")
	}
	dir := filepath.Join(websterDir, planBaselineDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("websterengine: store plan baseline: %w", err)
	}
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst := planBaselinePath(websterDir, hashes[name])
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return fmt.Errorf("websterengine: store plan baseline: read %s: %w", name, err)
		}
		if err := writeFileAtomic(dir, dst, data); err != nil {
			return fmt.Errorf("websterengine: store plan baseline: %s: %w", name, err)
		}
	}
	return nil
}

// writeFileAtomic writes data to dst through a temp file in dir and a rename.
func writeFileAtomic(dir, dst string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// RestorePlan restores every plan file changedPlanFiles reports.
// A recorded file is written back from the store after checking the stored bytes hash to the recorded value, and a file the run never recorded is removed.
// Every needed copy is checked before anything is written; a missing or corrupt copy refuses with ErrPlanBaselineMissing naming each such file, changing nothing.
// It returns the restored file names, sorted, and never touches state.json.
func RestorePlan(st *State, geom Geometry) ([]string, error) {
	if len(st.PlanFileHashes) == 0 {
		return nil, fmt.Errorf("%w: state.json recorded no plan hashes; %s", ErrPlanBaselineMissing, planBaselineWayForward)
	}
	changed, err := changedPlanFiles(st, geom.PlanDir)
	if err != nil {
		return nil, err
	}
	contents := make(map[string][]byte, len(changed))
	var missing []string
	for _, name := range changed {
		want, recorded := st.PlanFileHashes[name]
		if !recorded {
			continue
		}
		data, err := os.ReadFile(planBaselinePath(geom.WebsterDir, want))
		if err != nil {
			missing = append(missing, name)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			missing = append(missing, name)
			continue
		}
		contents[name] = data
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s; %s", ErrPlanBaselineMissing, strings.Join(missing, ", "), planBaselineWayForward)
	}
	for _, name := range changed {
		path := filepath.Join(geom.PlanDir, name)
		data, recorded := contents[name]
		if !recorded {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("websterengine: restore plan: remove %s: %w", name, err)
			}
			continue
		}
		if err := writeFileAtomic(geom.PlanDir, path, data); err != nil {
			return nil, fmt.Errorf("websterengine: restore plan: write %s: %w", name, err)
		}
	}
	return changed, nil
}
