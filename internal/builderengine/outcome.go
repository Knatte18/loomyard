// outcome.go implements the orchestrator's outcome.yaml contract: Outcome,
// the in-memory schema; ParseOutcome, the fail-loud parser (the burler
// verdict-parse discipline applied here — an unrecognized outcome value or a
// stuck report with no stuck_reason is a loud error, never a guessed
// digest); and ArchiveStaleOutcome, the archive-never-refuse act `run`
// performs before spawning a fresh orchestrator, per the discussion's
// "Orchestrator outcome contract" decision.

package builderengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The three legal Outcome.Outcome values.
const (
	OutcomeDone   = "done"
	OutcomeStuck  = "stuck"
	OutcomePaused = "paused"
)

// outcomeFileName is outcome.yaml's fixed filename inside a builder dir.
const outcomeFileName = "outcome.yaml"

// Outcome is the orchestrator's final-action file's in-memory form: the
// terminal judgment it reached over one whole plan run.
type Outcome struct {
	// Outcome is one of OutcomeDone, OutcomeStuck, or OutcomePaused.
	Outcome string `yaml:"outcome"`
	// StuckReason is the orchestrator's one-line account of the blocker,
	// required non-empty when Outcome is OutcomeStuck; empty (YAML null) for
	// OutcomeDone and OutcomePaused.
	StuckReason string `yaml:"stuck_reason"`
	// BatchesDone is the count of batches that reached status: done this
	// run.
	BatchesDone int `yaml:"batches_done"`
}

// ParseOutcome reads and strictly decodes outcome.yaml, enforcing the
// schema vocabulary: outcome must be Done/Stuck/Paused, and Stuck requires
// a non-empty stuck_reason.
func ParseOutcome(path string) (*Outcome, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("builder: read outcome file %s: %w", path, err)
	}

	var o Outcome
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return nil, fmt.Errorf("builder: outcome file %s: %w", path, err)
	}

	switch o.Outcome {
	case OutcomeDone, OutcomeStuck, OutcomePaused:
	default:
		return nil, fmt.Errorf("builder: outcome file %s: unrecognized outcome %q; want %q, %q, or %q", path, o.Outcome, OutcomeDone, OutcomeStuck, OutcomePaused)
	}

	if o.Outcome == OutcomeStuck && strings.TrimSpace(o.StuckReason) == "" {
		return nil, fmt.Errorf("builder: outcome file %s: outcome is %q but stuck_reason is empty", path, OutcomeStuck)
	}

	return &o, nil
}

// ArchiveStaleOutcome renames builderDir's outcome.yaml to
// outcome-<UTC timestamp>.yaml so resume never blocks on leftover files.
// Absent file returns ("", nil). Collisions in the same second append
// numeric suffixes ("-1", "-2", ...) to avoid overwriting prior runs.
func ArchiveStaleOutcome(builderDir string, now func() time.Time) (archivedTo string, err error) {
	path := filepath.Join(builderDir, outcomeFileName)

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil
		}
		return "", fmt.Errorf("builder: stat outcome file %s: %w", path, statErr)
	}

	// Route the same-second collision loop through FirstFreeArchivePath so the
	// "-1"/"-2" suffix rule lives in exactly one place (runlevel.go), shared
	// with Run's --fresh state/reports archiving and the recovery report
	// archive; archiveTimestampFormat is the same shared UTC-compact stamp.
	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := FirstFreeArchivePath(func(suffix string) string {
		return filepath.Join(builderDir, fmt.Sprintf("outcome-%s%s.yaml", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("builder: find archive target for outcome file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("builder: archive stale outcome file %s: %w", path, err)
	}
	return target, nil
}
