package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// RunInFlight reports whether a Webster run is mid-flight at anchorRoot.
// It exists so another package can ask the question without constructing webster's paths, per the Cwd Resolution and Told-Geometry invariants;
// every path is built from the told anchorRoot through Dir and OutcomePath.
//
// A run is in flight when state.json exists and outcome.yaml is either absent or names an outcome other than done.
// paused and stuck count as in flight because both resume with `lyx webster run`;
// only done ends the run.
// Only webster's own durable files are read, never loom's shed status.
//
// A state.json stat failure other than not-exist, and an unreadable or malformed outcome.yaml, return the error.
func RunInFlight(anchorRoot string) (bool, error) {
	dir := Dir(anchorRoot)
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("websterengine: stat state file under %s: %w", dir, err)
	}

	outcomePath := OutcomePath(dir)
	if _, err := os.Stat(outcomePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("websterengine: stat outcome file %s: %w", outcomePath, err)
	}

	o, err := parseOutcome(outcomePath)
	if err != nil {
		return false, err
	}
	return o.Outcome != outcomeDone, nil
}
