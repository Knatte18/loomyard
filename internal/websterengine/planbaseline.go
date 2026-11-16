// planbaseline.go keeps the content behind every plan hash webster records, and writes it back on demand.
// State.PlanFileHashes records only hashes,
// and a plan directory is not tracked by git on a task branch (nor exists in any repository for a standalone run),
// so without a stored copy no command could restore a plan edited since the run recorded it.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/runplan"
)

// ErrPlanBaselineMissing reports that a plan file cannot be restored from the store.
// The stored copy is absent or does not match its recorded hash, or the state recorded no plan hashes at all.
var ErrPlanBaselineMissing = errors.New("websterengine: plan baseline copy missing")

// freshRestartSteps is the ordered step list that discards the run and starts it over: the reset verb, then reentry.
// Callers embed it after their own lead-in, so it carries no "way forward: " prefix.
func freshRestartSteps(reentry string) string {
	return strings.TrimPrefix(resetToStartSteps(reentry), "way forward: ")
}

// planBaselineWayForward is the way forward named when a plan cannot be restored from the store.
func planBaselineWayForward(reentry string) string {
	return resetToStartSteps(reentry)
}

// planBaselineHas reports whether the store holds a copy for hash.
// An absent file is false with no error;
// any other stat failure is returned.
func planBaselineHas(websterDir, hash string) (bool, error) {
	_, err := os.Stat(runplan.PlanBaselinePath(websterDir, hash))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("websterengine: stat plan baseline copy %s: %w", hash, err)
}

// RestorePlan restores every plan file that has changed since the run recorded it.
// A recorded file is written back from the store after checking the stored bytes hash to the recorded value,
// and a file the run never recorded is removed.
// Every needed copy is checked before anything is written;
// a missing or corrupt copy refuses with ErrPlanBaselineMissing naming each such file, changing nothing.
// It returns the restored file names, sorted, and never touches state.json.
func RestorePlan(st *State, geom Geometry) ([]string, error) {
	if len(st.PlanFileHashes) == 0 {
		return nil, fmt.Errorf("%w: state.json recorded no plan hashes; %s", ErrPlanBaselineMissing, planBaselineWayForward(stepRun))
	}
	changed, err := runplan.ChangedPlanFiles(&st.State, geom.PlanDir)
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
		data, err := os.ReadFile(runplan.PlanBaselinePath(geom.WebsterDir, want))
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
		return nil, fmt.Errorf("%w: %s; %s", ErrPlanBaselineMissing, strings.Join(missing, ", "), planBaselineWayForward(stepRun))
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
		if err := runplan.WriteFileAtomic(geom.PlanDir, path, data); err != nil {
			return nil, fmt.Errorf("websterengine: restore plan: write %s: %w", name, err)
		}
	}
	return changed, nil
}
