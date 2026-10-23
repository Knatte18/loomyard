// fixerreport_test.go pins ParseFixerReport's verdicts, sections and refusals.

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

func TestParseFixerReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    FixerReport
		wantErr string
	}{
		{
			name:    "fixed with parent changes and fixes",
			content: "verdict: fixed\n\n## Parent changes\n\n- abc123: renames Foo; fits decision D2\n- def456: adds Bar; no conflict\n\n## Fixes\n\n- 0123abc: restored the call to Foo\n",
			want: FixerReport{
				Verdict:       FixerFixed,
				ParentChanges: []string{"abc123: renames Foo; fits decision D2", "def456: adds Bar; no conflict"},
				Fixes:         []string{"0123abc: restored the call to Foo"},
			},
		},
		{
			name:    "finalize mode fixed with an empty parent changes section",
			content: "verdict: fixed\n\n## Parent changes\n\n## Fixes\n\n- 0123abc: fixed the failing test\n",
			want: FixerReport{
				Verdict: FixerFixed,
				Fixes:   []string{"0123abc: fixed the failing test"},
			},
		},
		{
			name:    "escalate with its conflict",
			content: "verdict: escalate\n\n## Parent changes\n\n- abc123: drops Foo\n\n## Fixes\n\n## Conflict\n\nabc123 drops Foo, which decision D2 keeps.\nNo change satisfies both.\n",
			want: FixerReport{
				Verdict:       FixerEscalate,
				ParentChanges: []string{"abc123: drops Foo"},
				Conflict:      "abc123 drops Foo, which decision D2 keeps.\nNo change satisfies both.",
			},
		},
		{
			name:    "unknown verdict",
			content: "verdict: maybe\n\n## Fixes\n",
			wantErr: "line 1",
		},
		{
			name:    "escalate without its conflict",
			content: "verdict: escalate\n\n## Parent changes\n\n- abc123: drops Foo\n",
			wantErr: "## Conflict",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "report.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write report: %v", err)
			}
			got, err := ParseFixerReport(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "landing-fixer-report-spec.md") {
					t.Fatalf("ParseFixerReport() error = %v; want it to contain %q and name the spec", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFixerReport() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFixerReport() = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		_, err := ParseFixerReport(filepath.Join(t.TempDir(), "absent.md"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ParseFixerReport() error = %v; want it to wrap fs.ErrNotExist", err)
		}
	})
}
