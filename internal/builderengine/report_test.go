// report_test.go covers ParseReport against plan-format.md's two worked
// examples (a minimal done report, and one with a justified out_of_scope
// entry) and every distinct rejection case the schema enforces.

package builderengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// writeReport writes content to a fresh report.yaml under a temp dir and
// returns its path.
func writeReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

func TestParseReport_WorkedExampleMinimal(t *testing.T) {
	t.Parallel()

	path := writeReport(t, `batch: 01-json-flag
status: done
tests: green
stuck_reason: null
`)

	r, err := builderengine.ParseReport(path)
	if err != nil {
		t.Fatalf("ParseReport() error = %v; want nil", err)
	}
	if r.Batch != "01-json-flag" {
		t.Errorf("Batch = %q; want %q", r.Batch, "01-json-flag")
	}
	if r.Status != builderengine.ReportStatusDone {
		t.Errorf("Status = %q; want %q", r.Status, builderengine.ReportStatusDone)
	}
	if r.Tests != builderengine.ReportTestsGreen {
		t.Errorf("Tests = %q; want %q", r.Tests, builderengine.ReportTestsGreen)
	}
	if r.StuckReason != "" {
		t.Errorf("StuckReason = %q; want empty (YAML null)", r.StuckReason)
	}
	if len(r.OutOfScope) != 0 {
		t.Errorf("OutOfScope = %+v; want empty", r.OutOfScope)
	}
}

func TestParseReport_WorkedExampleOutOfScope(t *testing.T) {
	t.Parallel()

	path := writeReport(t, `batch: 01-json-flag
status: done
tests: green
stuck_reason: null
out_of_scope:
  - path: internal/output/envelope.go
    why: "Ok() lacked an io.Writer variant the list path needs; added OkTo()"
`)

	r, err := builderengine.ParseReport(path)
	if err != nil {
		t.Fatalf("ParseReport() error = %v; want nil", err)
	}
	if len(r.OutOfScope) != 1 {
		t.Fatalf("OutOfScope = %+v; want exactly 1 entry", r.OutOfScope)
	}
	if r.OutOfScope[0].Path != "internal/output/envelope.go" {
		t.Errorf("OutOfScope[0].Path = %q; want %q", r.OutOfScope[0].Path, "internal/output/envelope.go")
	}
	if r.OutOfScope[0].Why == "" {
		t.Errorf("OutOfScope[0].Why is empty; want the worked example's justification")
	}
}

func TestParseReport_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		wantSubstr string
	}{
		{
			name: "empty batch",
			content: `batch: ""
status: done
tests: green
`,
			wantSubstr: "batch",
		},
		{
			name: "unknown status",
			content: `batch: 01-x
status: finished
tests: green
`,
			wantSubstr: "status",
		},
		{
			name: "unknown tests value",
			content: `batch: 01-x
status: done
tests: mostly-green
`,
			wantSubstr: "tests",
		},
		{
			name: "stuck with empty stuck_reason",
			content: `batch: 01-x
status: stuck
tests: red
stuck_reason: null
`,
			wantSubstr: "stuck_reason",
		},
		{
			name: "out_of_scope missing path",
			content: `batch: 01-x
status: done
tests: green
out_of_scope:
  - why: "no path given"
`,
			wantSubstr: "path",
		},
		{
			name: "out_of_scope missing why",
			content: `batch: 01-x
status: done
tests: green
out_of_scope:
  - path: internal/x.go
`,
			wantSubstr: "why",
		},
		{
			name: "unknown field rejected by KnownFields",
			content: `batch: 01-x
status: done
tests: green
extra_field: surprise
`,
			wantSubstr: "extra_field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeReport(t, tt.content)
			_, err := builderengine.ParseReport(path)
			if err == nil {
				t.Fatalf("ParseReport(%s) error = nil; want error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ParseReport(%s) error = %q; want substring %q", tt.name, err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestParseReport_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := builderengine.ParseReport(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("ParseReport(absent file) error = nil; want error")
	}
}
