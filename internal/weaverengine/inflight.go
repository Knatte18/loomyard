// inflight.go reports whether a Weaver run is mid-flight, read from the run's durable files alone.

package weaverengine

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RunInFlight reports whether a Weaver run is mid-flight at anchorRoot.
// It lets another package ask without constructing Weaver's paths itself.
//
// A run is in flight when state.json exists and outcome.yaml is either absent or names an outcome other than done.
// stuck counts as in flight because the run resumes; only done ends it.
// Run removes outcome.yaml on entry, so a re-stepped run reads as in flight until its Done return.
//
// A state.json stat failure other than not-exist, and an unreadable or malformed outcome.yaml, return the error.
// A malformed outcome.yaml holds an unknown key or an outcome that is neither done nor stuck.
func RunInFlight(anchorRoot string) (bool, error) {
	dir := Dir(anchorRoot)
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("weaver: stat state file under %s: %w", dir, err)
	}

	outcomePath := OutcomePath(dir)
	data, err := os.ReadFile(outcomePath)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("weaver: read outcome file %s: %w", outcomePath, err)
	}

	var record outcomeRecord
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&record); err != nil {
		return false, fmt.Errorf("weaver: outcome file %s: %w", outcomePath, err)
	}
	switch record.Outcome {
	case resultDone:
		return false, nil
	case resultStuck:
		return true, nil
	}
	return false, fmt.Errorf("weaver: outcome file %s: unrecognized outcome %q; want %q or %q", outcomePath, record.Outcome, resultDone, resultStuck)
}
