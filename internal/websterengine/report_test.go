// report_test.go covers Report's round-trip through WriteReport->ParseReport for both terminal
// statuses and both empty and populated deviation lists, plus ParseReport's strict-decode
// rejections: an unknown key, a missing head_sha, an unrecognized status, malformed YAML, an
// empty file and a missing file.
// Tier 1: no git, only t.TempDir().

package websterengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReport_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		number   int
		slug     string
		wantFile string
		want     *Report
	}{
		{
			name:     "OK with empty deviations",
			number:   1,
			slug:     "slug",
			wantFile: "01-slug.yaml",
			want:     &Report{Status: ReportStatusOK, HeadSHA: "abc123", Deviations: nil},
		},
		{
			name:     "FAILED with populated deviations",
			number:   3,
			slug:     "some-slug",
			wantFile: "03-some-slug.yaml",
			want: &Report{
				Status:     ReportStatusFailed,
				HeadSHA:    "def456",
				Deviations: []string{"internal/foo/bar.go", "docs/notes.md"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fileName := ReportFileName(tt.number, tt.slug)
			if fileName != tt.wantFile {
				t.Errorf("ReportFileName(%d, %q) = %q; want %q", tt.number, tt.slug, fileName, tt.wantFile)
			}
			path := filepath.Join(t.TempDir(), fileName)

			if err := WriteReport(path, tt.want); err != nil {
				t.Fatalf("WriteReport() error = %v; want nil", err)
			}

			got, err := ParseReport(path)
			if err != nil {
				t.Fatalf("ParseReport() error = %v; want nil", err)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q; want %q", got.Status, tt.want.Status)
			}
			if got.HeadSHA != tt.want.HeadSHA {
				t.Errorf("HeadSHA = %q; want %q", got.HeadSHA, tt.want.HeadSHA)
			}
			if !slices.Equal(got.Deviations, tt.want.Deviations) {
				t.Errorf("Deviations = %v; want %v", got.Deviations, tt.want.Deviations)
			}
		})
	}
}

func TestParseReport_Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// content is the file body; missing leaves the file absent instead.
		content string
		missing bool
	}{
		{name: "an unknown key", content: "status: OK\nhead_sha: abc123\ndeviations: []\nextra_field: surprise\n"},
		{name: "a missing head_sha", content: "status: OK\nhead_sha: \"\"\ndeviations: []\n"},
		{name: "an unrecognized status", content: "status: MAYBE\nhead_sha: abc123\ndeviations: []\n"},
		{name: "malformed YAML", content: "status: [this is not a mapping\n"},
		{name: "an empty file", content: ""},
		{name: "a missing file", missing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "report.yaml")
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			if _, err := ParseReport(path); err == nil {
				t.Fatalf("ParseReport() error = nil; want an error")
			}
		})
	}
}
