// report_test.go covers Report's round-trip through WriteReport->ParseReport
// for both terminal statuses and both empty and populated deviation lists,
// plus ParseReport's strict-decode rejections: an unknown key, a missing
// head_sha, and an unrecognized status. Tier 1: no git, only t.TempDir().

package websterengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReport_RoundTrip_OK_EmptyDeviations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "01-slug.yaml")
	want := &Report{Status: ReportStatusOK, HeadSHA: "abc123", Deviations: nil}

	if err := WriteReport(path, want); err != nil {
		t.Fatalf("WriteReport() error = %v; want nil", err)
	}

	got, err := ParseReport(path)
	if err != nil {
		t.Fatalf("ParseReport() error = %v; want nil", err)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q; want %q", got.Status, want.Status)
	}
	if got.HeadSHA != want.HeadSHA {
		t.Errorf("HeadSHA = %q; want %q", got.HeadSHA, want.HeadSHA)
	}
	if len(got.Deviations) != 0 {
		t.Errorf("Deviations = %v; want empty", got.Deviations)
	}
}

func TestReport_RoundTrip_FAILED_PopulatedDeviations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "02-slug.yaml")
	want := &Report{
		Status:     ReportStatusFailed,
		HeadSHA:    "def456",
		Deviations: []string{"internal/foo/bar.go", "docs/notes.md"},
	}

	if err := WriteReport(path, want); err != nil {
		t.Fatalf("WriteReport() error = %v; want nil", err)
	}

	got, err := ParseReport(path)
	if err != nil {
		t.Fatalf("ParseReport() error = %v; want nil", err)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q; want %q", got.Status, want.Status)
	}
	if got.HeadSHA != want.HeadSHA {
		t.Errorf("HeadSHA = %q; want %q", got.HeadSHA, want.HeadSHA)
	}
	if len(got.Deviations) != len(want.Deviations) {
		t.Fatalf("Deviations = %v; want %v", got.Deviations, want.Deviations)
	}
	for i, d := range want.Deviations {
		if got.Deviations[i] != d {
			t.Errorf("Deviations[%d] = %q; want %q", i, got.Deviations[i], d)
		}
	}
}

func TestReportFileName(t *testing.T) {
	t.Parallel()

	if got, want := ReportFileName(3, "some-slug"), "03-some-slug.yaml"; got != want {
		t.Errorf("ReportFileName(3, %q) = %q; want %q", "some-slug", got, want)
	}
}

func TestParseReport_RejectsUnknownKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.yaml")
	content := "status: OK\nhead_sha: abc123\ndeviations: []\nextra_field: surprise\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ParseReport(path); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for unknown key")
	}
}

func TestParseReport_RejectsMissingHeadSHA(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.yaml")
	content := "status: OK\nhead_sha: \"\"\ndeviations: []\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ParseReport(path); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for missing head_sha")
	}
}

func TestParseReport_RejectsBadStatus(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.yaml")
	content := "status: MAYBE\nhead_sha: abc123\ndeviations: []\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ParseReport(path); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for unrecognized status")
	}
}

func TestParseReport_RejectsMalformedYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.yaml")
	content := "status: [this is not a mapping\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ParseReport(path); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for malformed YAML")
	}
}

func TestParseReport_RejectsEmptyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.yaml")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ParseReport(path); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for empty file")
	}
}

func TestParseReport_MissingFile(t *testing.T) {
	t.Parallel()

	if _, err := ParseReport(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatalf("ParseReport() error = nil; want error for missing file")
	}
}
