// integrationreport.go implements the integration fork's own report shape.
// It is a superset of Report: the fork writes status, head_sha and deviations,
// and Go adds the optional failures and triage fields afterwards.
// Batch reports keep their strict three-field Report shape.

package websterengine

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// The three legal IntegrationFailure.Kind values.
const (
	FailureKindTest    = "test"
	FailureKindPackage = "package"
	FailureKindOpaque  = "opaque"
)

// The three legal IntegrationTriage.Verdict values.
const (
	TriageVerdictFlaky       = "flaky"
	TriageVerdictPreExisting = "pre-existing"
	TriageVerdictRegression  = "regression"
)

// The two legal IntegrationTriage.Rerun values.
const (
	TriageRerunPassed = "passed"
	TriageRerunFailed = "failed"
)

// IntegrationFailure names one failing identity from the verify run and the tail of its output.
type IntegrationFailure struct {
	// ID is the failure identity: a test name, a package path, or an opaque token.
	ID string `yaml:"id"`
	// Kind is one of FailureKindTest, FailureKindPackage, FailureKindOpaque.
	Kind string `yaml:"kind"`
	// Tail is the last lines of the identity's output.
	Tail string `yaml:"tail"`
}

// IntegrationTriage is Go's classification of a red integration verify.
type IntegrationTriage struct {
	// Verdict is one of the TriageVerdict* values.
	Verdict string `yaml:"verdict"`
	// Rerun is TriageRerunPassed or TriageRerunFailed.
	Rerun string `yaml:"rerun"`
	// BaselineSHA is the commit the failures were compared against.
	BaselineSHA string `yaml:"baseline_sha,omitempty"`
	// Flaky lists identities that failed once and passed on rerun.
	Flaky []string `yaml:"flaky,omitempty"`
	// PreExisting lists identities that also fail at the baseline.
	PreExisting []string `yaml:"pre_existing,omitempty"`
	// Regressions lists identities that fail at head and not at the baseline.
	Regressions []string `yaml:"regressions,omitempty"`
}

// IntegrationReport is the integration fork's report: the fork-written Status, HeadSHA and Deviations, plus the Go-written optional Failures and Triage.
type IntegrationReport struct {
	Status     string               `yaml:"status"`
	HeadSHA    string               `yaml:"head_sha"`
	Deviations []string             `yaml:"deviations"`
	Failures   []IntegrationFailure `yaml:"failures,omitempty"`
	Triage     *IntegrationTriage   `yaml:"triage,omitempty"`
}

// ParseIntegrationReport reads and strictly decodes the integration report at path, then applies the same status and head_sha validation as ParseReport.
// A fork-only report (no failures, no triage) parses cleanly.
func ParseIntegrationReport(path string) (*IntegrationReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("websterengine: read report %s: %w", path, err)
	}

	var r IntegrationReport
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("websterengine: report %s: %w", path, err)
	}

	if err := validateReportFields(path, r.Status, r.HeadSHA); err != nil {
		return nil, err
	}

	return &r, nil
}

// WriteIntegrationReport serializes the whole of r to path.
func WriteIntegrationReport(path string, r *IntegrationReport) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("websterengine: marshal report for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("websterengine: write report %s: %w", path, err)
	}
	return nil
}
