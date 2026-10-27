// report.go implements webster's own fork-return report contract: the minimal YAML shape a fork
// writes as its final action to the per-batch report file under websterengine.ReportsDir(...),
// and the strict parser that reads it back.
// webster's flat card format carries no per-batch Scope, so there is nothing to
// justify an out-of-scope entry against;
// a fork simply reports whether it succeeded, or that it waits on a step only its parent can take, the head SHA it left, and the glyphs it renamed.
// An older fork's deviations key is still accepted and never read;
// ParseReport fails only on Status/HeadSHA/Need/Renames shape violations.

package websterengine

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// The three legal Report.Status values.
// NEEDS_PARENT is a batch stopped on a step only the run's parent can take, which no recovery strand could take either.
const (
	ReportStatusOK          = "OK"
	ReportStatusFailed      = "FAILED"
	ReportStatusNeedsParent = "NEEDS_PARENT"
)

// Report is a fork's return contract: the in-memory form of the per-batch report YAML file a fork
// writes as its final action.
type Report struct {
	// Status is one of ReportStatusOK, ReportStatusFailed or ReportStatusNeedsParent.
	Status string `yaml:"status"`
	// HeadSHA is the commit SHA the fork left the worktree at.
	HeadSHA string `yaml:"head_sha"`
	// Need is the one-line step only the parent can take, required with ReportStatusNeedsParent and refused with any other status.
	Need string `yaml:"need,omitempty"`
	// Renames lists every glyph the batch renamed, so a later card still naming the old glyph is rewritten to the new one.
	Renames []ReportRename `yaml:"renames,omitempty"`
	// Deviations is accepted and never read: an older fork's report carries the key,
	// and the known-fields decoding must keep accepting it.
	Deviations []string `yaml:"deviations,omitempty"`
}

// ReportRename is one glyph rename a fork reports: the glyph before the batch, the glyph that replaced it, and why the fork could not keep the plan's name.
type ReportRename struct {
	Old    string `yaml:"old" json:"old"`
	New    string `yaml:"new" json:"new"`
	Reason string `yaml:"reason" json:"reason"`
}

// ReportFileName returns the per-batch report file's name, "NN-<slug>.yaml", webster's own naming.
func ReportFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.yaml", number, slug)
}

// ParseReport reads and strictly decodes the fork-return report YAML file at path, then enforces strict validation.
// Status must be one of the three legal values, head_sha must be non-empty, need must be non-empty exactly when status is ReportStatusNeedsParent,
// and each renames entry must name two different, non-empty glyphs.
// Each violation is returned as its own distinct wrapped error.
func ParseReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("websterengine: read report %s: %w", path, err)
	}

	var r Report
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("websterengine: report %s: %w", path, err)
	}

	if err := validateReportFields(path, r); err != nil {
		return nil, err
	}

	return &r, nil
}

// validateReportFields enforces ParseReport's status, head_sha and need shape.
func validateReportFields(path string, r Report) error {
	switch r.Status {
	case ReportStatusOK, ReportStatusFailed, ReportStatusNeedsParent:
	default:
		return fmt.Errorf("websterengine: report %s: unrecognized status %q; want %q, %q or %q", path, r.Status, ReportStatusOK, ReportStatusFailed, ReportStatusNeedsParent)
	}

	if strings.TrimSpace(r.HeadSHA) == "" {
		return fmt.Errorf("websterengine: report %s: missing required field %q", path, "head_sha")
	}

	hasNeed := strings.TrimSpace(r.Need) != ""
	if r.Status == ReportStatusNeedsParent && !hasNeed {
		return fmt.Errorf("websterengine: report %s: status %q requires a non-empty %q naming the step only the parent can take", path, r.Status, "need")
	}
	if r.Status != ReportStatusNeedsParent && hasNeed {
		return fmt.Errorf("websterengine: report %s: %q is allowed only with status %q, not %q", path, "need", ReportStatusNeedsParent, r.Status)
	}

	for i, rn := range r.Renames {
		if strings.TrimSpace(rn.Old) == "" || strings.TrimSpace(rn.New) == "" {
			return fmt.Errorf("websterengine: report %s: renames entry %d needs %q and %q", path, i+1, "old", "new")
		}
		if rn.Old == rn.New {
			return fmt.Errorf("websterengine: report %s: renames entry %d names %q as both its old and its new glyph", path, i+1, rn.Old)
		}
	}

	return nil
}

// WriteReport serializes r's fields to path, the per-batch report file the fork writes under
// websterengine.ReportsDir(...).
// The caller supplies path;
// WriteReport never constructs `_lym` tokens itself, per the Cwd Resolution Invariant.
func WriteReport(path string, r *Report) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("websterengine: marshal report for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("websterengine: write report %s: %w", path, err)
	}
	return nil
}
