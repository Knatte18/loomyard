// report.go implements ParseReport, the fail-loud parser for the batch-
// report YAML file an implementer writes as its final action to
// _lyx/builder/reports/NN-<batch-slug>.yaml (docs/reference/plan-format.md's
// "Batch-report" section). Every distinct malformation — an unrecognized
// status/tests value, a missing batch, a stuck report with no
// stuck_reason, or an out_of_scope entry missing path/why — is its own
// wrapped error: the burler verdict-parse discipline applied here means an
// unparseable report is a loud error, never a guessed digest.

package builderengine

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// The two legal Report.Status values.
const (
	ReportStatusDone  = "done"
	ReportStatusStuck = "stuck"
)

// The three legal Report.Tests values.
const (
	ReportTestsGreen   = "green"
	ReportTestsRed     = "red"
	ReportTestsSkipped = "skipped"
)

// Report is the batch-report contract's in-memory form.
type Report struct {
	// Batch is the report's "batch:" field: the NN-<batch-slug> stem
	// matching the report's own filename.
	Batch string `yaml:"batch"`
	// Status is either ReportStatusDone or ReportStatusStuck.
	Status string `yaml:"status"`
	// Tests is one of ReportTestsGreen, ReportTestsRed, or
	// ReportTestsSkipped (deferred-verify intermediates).
	Tests string `yaml:"tests"`
	// StuckReason is the report's "stuck_reason:" field: empty for the
	// YAML null a done report carries, required non-empty when Status is
	// ReportStatusStuck.
	StuckReason string `yaml:"stuck_reason"`
	// OutOfScope is the report's optional "out_of_scope:" list: files the
	// implementer touched outside the batch's declared Scope, each with
	// its one-line justification.
	OutOfScope []OutOfScopeEntry `yaml:"out_of_scope"`
}

// OutOfScopeEntry is one batch-report out_of_scope entry.
type OutOfScopeEntry struct {
	// Path is the out-of-scope file's path.
	Path string `yaml:"path" json:"path"`
	// Why is the implementer's one-line justification for touching it.
	Why string `yaml:"why" json:"why"`
}

// ParseReport reads and strictly decodes the batch-report YAML file at
// path (yaml.Decoder.KnownFields(true), so an unrecognized key is a fail-
// loud error, not silently ignored), then enforces the schema's vocabulary
// and cross-field rules:
//   - batch: must be non-empty;
//   - status: must be ReportStatusDone or ReportStatusStuck;
//   - tests: must be ReportTestsGreen, ReportTestsRed, or ReportTestsSkipped;
//   - status: stuck requires a non-empty stuck_reason;
//   - every out_of_scope entry requires both path and why.
//
// Each violation is returned as its own distinct wrapped error naming path
// and the offending field.
func ParseReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("builder: read batch report %s: %w", path, err)
	}

	var r Report
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("builder: batch report %s: %w", path, err)
	}

	if strings.TrimSpace(r.Batch) == "" {
		return nil, fmt.Errorf("builder: batch report %s: missing required field %q", path, "batch")
	}

	switch r.Status {
	case ReportStatusDone, ReportStatusStuck:
	default:
		return nil, fmt.Errorf("builder: batch report %s: unrecognized status %q; want %q or %q", path, r.Status, ReportStatusDone, ReportStatusStuck)
	}

	switch r.Tests {
	case ReportTestsGreen, ReportTestsRed, ReportTestsSkipped:
	default:
		return nil, fmt.Errorf("builder: batch report %s: unrecognized tests %q; want %q, %q, or %q", path, r.Tests, ReportTestsGreen, ReportTestsRed, ReportTestsSkipped)
	}

	if r.Status == ReportStatusStuck && strings.TrimSpace(r.StuckReason) == "" {
		return nil, fmt.Errorf("builder: batch report %s: status is %q but stuck_reason is empty", path, ReportStatusStuck)
	}

	for i, entry := range r.OutOfScope {
		if strings.TrimSpace(entry.Path) == "" {
			return nil, fmt.Errorf("builder: batch report %s: out_of_scope entry %d is missing path", path, i)
		}
		if strings.TrimSpace(entry.Why) == "" {
			return nil, fmt.Errorf("builder: batch report %s: out_of_scope entry %d is missing why", path, i)
		}
	}

	return &r, nil
}
