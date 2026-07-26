// outcome.go implements webster's own Master-outcome contract: outcome, the
// in-memory schema parseOutcome strictly decodes, and archiveStaleOutcome,
// the archive-never-refuse act performed before Master's own outcome.yaml
// is next written. Unlike gitwrap.go/fingerprint.go/pause.go/archive.go —
// webster-local copies of a builder mechanism with an in-tree builder
// caller — outcome.go is a rewrite-anyway contract: webster defines its own
// shape rather than importing builder's Outcome/ParseOutcome, per the
// Shared Decision builder-is-frozen-copy-not-move. Consumed only
// in-package (runlevel.go/summary.go, wired in batch 7), so every symbol
// here stays unexported/engine-scoped.

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

// The three legal outcome.Outcome values.
const (
	outcomeDone   = "done"
	outcomePaused = "paused"
	outcomeStuck  = "stuck"
)

// outcomeFileName is outcome.yaml's fixed filename inside a webster dir.
const outcomeFileName = "outcome.yaml"

// outcome is Master's final-action file's in-memory form: the terminal
// judgment it reached over one whole plan run.
type outcome struct {
	// Outcome is one of outcomeDone, outcomeStuck, or outcomePaused.
	Outcome string `yaml:"outcome"`
	// StuckReason is Master's one-line account of the blocker, required
	// non-empty when Outcome is outcomeStuck; empty (YAML null) for
	// outcomeDone and outcomePaused.
	StuckReason string `yaml:"stuck_reason"`
	// BatchesDone is the count of batches that reached status: done this
	// run.
	BatchesDone int `yaml:"batches_done"`
}

// parseOutcome reads and strictly decodes the outcome.yaml file at path
// (yaml.Decoder.KnownFields(true), so an unrecognized key is a fail-loud
// error, never silently ignored), then enforces the schema's vocabulary
// and cross-field rule: outcome must be one of outcomeDone, outcomeStuck,
// or outcomePaused, and outcomeStuck requires a non-empty stuck_reason.
// Every violation is its own distinct wrapped error naming path and the
// offending field — the fail-loud verdict-parse discipline: an unparseable
// outcome file is a hard error, never a guessed result.
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
	case outcomeDone, outcomeStuck, outcomePaused:
	default:
		return nil, fmt.Errorf("websterengine: outcome file %s: unrecognized outcome %q; want %q, %q, or %q", path, o.Outcome, outcomeDone, outcomeStuck, outcomePaused)
	}

	if o.Outcome == outcomeStuck && strings.TrimSpace(o.StuckReason) == "" {
		return nil, fmt.Errorf("websterengine: outcome file %s: outcome is %q but stuck_reason is empty", path, outcomeStuck)
	}

	return &o, nil
}

// archiveStaleOutcome renames websterDir's outcome.yaml, if present, to
// outcome-<UTC compact timestamp>.yaml in place — resume (re-running `lyx
// webster run`) must never be blocked by a prior run's leftover outcome
// file, and the prior run's own judgment stays on disk, auditable, rather
// than being silently overwritten or deleted. now is a seam so tests can
// pin the timestamp deterministically instead of racing the real clock;
// production callers pass time.Now.
//
// Absent file: returns ("", nil) — not an error, since a fresh run has
// never written one yet.
//
// Collision: a second archive attempt in the same second (two calls whose
// now() truncates to an identical compact timestamp) would otherwise
// silently overwrite the first archive's content; archiveStaleOutcome
// instead appends a numeric suffix ("-1", "-2", ...) via
// firstFreeArchivePath (archive.go, card 18) until it finds a target path
// that does not yet exist, so no prior run's judgment is ever clobbered.
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
