// report.go implements webster's own fork-return report contract: the
// minimal YAML shape a fork writes as its final action to the per-batch
// report file under hubgeometry.WebsterReportsDir(...), and the strict
// parser that reads it back. This replaces builderengine.ParseReport/Report
// and its done/stuck/green/red/skipped/out_of_scope grammar entirely —
// webster's flat card format carries no per-batch Scope, so there is
// nothing to justify an out-of-scope entry against; a fork simply reports
// whether it succeeded, the head SHA it left, and an informational list of
// files it touched beyond the plan's prediction (see the deviation-list-is-
// informational Shared Decision — ParseReport never fails on Deviations
// content, only on Status/HeadSHA shape violations).

package websterengine

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// The two legal Report.Status values.
const (
	ReportStatusOK     = "OK"
	ReportStatusFailed = "FAILED"
)

// Report is a fork's return contract: the in-memory form of the per-batch
// report YAML file a fork writes as its final action.
type Report struct {
	// Status is either ReportStatusOK or ReportStatusFailed.
	Status string `yaml:"status"`
	// HeadSHA is the commit SHA the fork left the worktree at.
	HeadSHA string `yaml:"head_sha"`
	// Deviations is the (possibly empty) list of worktree-relative paths
	// the fork touched beyond the batch's declared file-ops union. It is
	// always informational — see the deviation-list-is-informational
	// Shared Decision — and never causes ParseReport to fail.
	Deviations []string `yaml:"deviations"`
}

// ReportFileName returns the per-batch report file's name,
// "NN-<slug>.yaml", webster's own naming (replacing
// builderengine.BatchReportFileName).
func ReportFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.yaml", number, slug)
}

// ParseReport reads and strictly decodes the fork-return report YAML file
// at path (yaml.Decoder.KnownFields(true), so an unrecognized key is a
// fail-loud error, not silently ignored), then enforces the schema's
// vocabulary and cross-field rules:
//   - status: must be ReportStatusOK or ReportStatusFailed;
//   - head_sha: must be non-empty.
//
// Deviations is never validated beyond its shape (a YAML list of strings);
// a non-empty deviation list is never itself an error, per the deviation-
// list-is-informational Shared Decision. Each violation is returned as its
// own distinct wrapped error naming path and the offending field.
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

	switch r.Status {
	case ReportStatusOK, ReportStatusFailed:
	default:
		return nil, fmt.Errorf("websterengine: report %s: unrecognized status %q; want %q or %q", path, r.Status, ReportStatusOK, ReportStatusFailed)
	}

	if strings.TrimSpace(r.HeadSHA) == "" {
		return nil, fmt.Errorf("websterengine: report %s: missing required field %q", path, "head_sha")
	}

	return &r, nil
}

// WriteReport serializes r's three fields to path, the per-batch report
// file the fork writes under hubgeometry.WebsterReportsDir(...). The
// caller supplies path; WriteReport never constructs `_lyx` tokens itself,
// per the Hub Geometry Invariant.
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
