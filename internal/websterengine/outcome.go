// outcome.go defines webster's Master-outcome contract: the outcome type, parseOutcome, and archiveStaleOutcome.
// parseOutcome strictly decodes outcome.yaml.
// archiveStaleOutcome moves an existing outcome.yaml aside before Master writes a new one.

package websterengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The two legal values of outcome's Outcome field.
const (
	outcomeDone  = "done"
	outcomeStuck = "stuck"
)

// outcomeFileName is outcome.yaml's fixed filename inside a webster dir.
const outcomeFileName = "outcome.yaml"

// outcome is the in-memory form of Master's final-action file:
// the terminal judgment Master reached over one whole plan run.
type outcome struct {
	// Outcome is one of outcomeDone or outcomeStuck.
	Outcome string `yaml:"outcome"`
	// StuckReason is Master's one-line account of the blocker: required for outcomeStuck, empty (YAML null) for outcomeDone.
	StuckReason string `yaml:"stuck_reason"`
	// BatchesDone is the count of batches that reached status: done.
	// It counts this run only.
	BatchesDone int `yaml:"batches_done"`
}

// parseOutcome reads and strictly decodes outcome.yaml. Enforces schema:
// outcome must be one of done/stuck; stuck requires non-empty stuck_reason.
func parseOutcome(path string) (*outcome, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("websterengine: read outcome file %s: %w", path, err)
	}

	var o outcome
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return nil, fmt.Errorf("websterengine: outcome file %s: %w", path, err)
	}

	switch o.Outcome {
	case outcomeDone, outcomeStuck:
	default:
		return nil, fmt.Errorf("websterengine: outcome file %s: unrecognized outcome %q; want %q or %q", path, o.Outcome, outcomeDone, outcomeStuck)
	}

	if o.Outcome == outcomeStuck && strings.TrimSpace(o.StuckReason) == "" {
		return nil, fmt.Errorf("websterengine: outcome file %s: outcome is %q but stuck_reason is empty", path, outcomeStuck)
	}

	return &o, nil
}

// archiveStaleOutcome renames outcome.yaml to outcome-<timestamp>.yaml in place.
// Absent file returns ("", nil). A name collision gets a numeric suffix ("-1", "-2", ...).
func archiveStaleOutcome(websterDir string, now func() time.Time) (archivedTo string, err error) {
	path := filepath.Join(websterDir, outcomeFileName)

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil
		}
		return "", fmt.Errorf("websterengine: stat outcome file %s: %w", path, statErr)
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(websterDir, fmt.Sprintf("outcome-%s%s.yaml", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("websterengine: find archive target for outcome file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("websterengine: archive stale outcome file %s: %w", path, err)
	}
	return target, nil
}
