//go:build tier0

// fixreview_test.go pins ParseFixReviewReport's verdicts, findings and refusals.

package landingshed

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFixReviewReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    FixReviewReport
		wantErr string
	}{
		{
			name:    "approved with an empty findings section",
			content: "verdict: approved\n\n## Findings\n",
			want:    FixReviewReport{Verdict: FixReviewApproved},
		},
		{
			name:    "approved with a note under findings",
			content: "verdict: approved\n\n## Findings\n- f1: the lock is test-only and keeps every assertion\n",
			want:    FixReviewReport{Verdict: FixReviewApproved, Findings: []string{"f1: the lock is test-only and keeps every assertion"}},
		},
		{
			name:    "rejected with findings, bullets outside the section ignored",
			content: "verdict: rejected\n\n- stray\n\n## Findings\n\n- f1: skips TestRace\n- f2: drops an assertion\n",
			want:    FixReviewReport{Verdict: FixReviewRejected, Findings: []string{"f1: skips TestRace", "f2: drops an assertion"}},
		},
		{
			name:    "an unknown verdict",
			content: "verdict: fixed\n\n## Findings\n",
			wantErr: `line 1 "verdict: fixed" is no known verdict`,
		},
		{
			name:    "a rejection with no finding",
			content: "verdict: rejected\n\n## Findings\n",
			wantErr: "a rejected report needs at least one",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "review.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := ParseFixReviewReport(path)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), fixReviewSpecName) {
					t.Errorf("ParseFixReviewReport() error = %v; want one containing %q and naming %s", err, tt.wantErr, fixReviewSpecName)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFixReviewReport() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFixReviewReport() = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		_, err := ParseFixReviewReport(filepath.Join(t.TempDir(), "absent.md"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ParseFixReviewReport(absent) error = %v; want one wrapping fs.ErrNotExist", err)
		}
	})
}
