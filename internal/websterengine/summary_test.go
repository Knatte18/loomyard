// summary_test.go exercises the Append* helpers that add sections to the final summary.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// writeSummaryFile writes raw content to path, creating its parent
// directory first, failing the test on any error.
func writeSummaryFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// readSummaryFile returns the summary.md content under dir.
func readSummaryFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(summaryparser.Path(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

const triageSectionHead = "\n\n## Integration suite triage\n\nThe plan-level `## verify:` suite failed, but webster's triage did not attribute the failure to this run.\n"

// TestAppendSummarySections pins each Append* helper: an empty list leaves the file
// byte-identical, and a non-empty list appends its section after the existing content in order.
func TestAppendSummarySections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		append func(dir string) error
		// want is the full summary.md after the call.
		want string
	}{
		{
			name:   "audit warnings: empty is a no-op",
			append: func(dir string) error { return websterengine.AppendAuditWarnings(dir, nil) },
			want:   "# S\n",
		},
		{
			name:   "audit warnings: bullets follow the existing content in order",
			append: func(dir string) error { return websterengine.AppendAuditWarnings(dir, []string{"first", "second"}) },
			want:   "# S\n\n\n## Audit warnings\n\nThese findings (fork-audit policy findings, and drift about a later card) were recorded as warnings and did not stop the run.\n\n- first\n- second\n",
		},
		{
			name:   "background shells: empty is a no-op",
			append: func(dir string) error { return websterengine.AppendBackgroundShells(dir, nil) },
			want:   "# S\n",
		},
		{
			name:   "background shells: one bullet per label in order",
			append: func(dir string) error { return websterengine.AppendBackgroundShells(dir, []string{"first", "second"}) },
			want:   "# S\n\n\n## Background shells waited out\n\nMaster's turn end was counted while these background shells were still running, which comes after `background_shell_wait_min` for a shell only the transcript reports and at once for one the Stop payload reports when Master's output files exist; they may still be running in the session.\n\n- `first`\n- `second`\n",
		},
		{
			name:   "integration triage: empty is a no-op",
			append: func(dir string) error { return websterengine.AppendIntegrationTriage(dir, nil) },
			want:   "# S\n",
		},
		{
			name: "integration triage: flaky tests are listed by identity",
			append: func(dir string) error {
				return websterengine.AppendIntegrationTriage(dir, []string{"TestF1", "TestF2"})
			},
			want: "# S\n" + triageSectionHead + "\nFlaky (passed on rerun):\n\n- `TestF1`\n- `TestF2`\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeSummaryFile(t, summaryparser.Path(dir), "# S\n")

			if err := tt.append(dir); err != nil {
				t.Fatalf("append: %v", err)
			}
			if got := readSummaryFile(t, dir); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("a missing summary file errors naming its path", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		err := websterengine.AppendAuditWarnings(dir, []string{"w"})
		if err == nil {
			t.Fatal("AppendAuditWarnings on missing file: want error, got nil")
		}
		if !strings.Contains(err.Error(), summaryparser.Path(dir)) {
			t.Errorf("error %q does not name path %q", err, summaryparser.Path(dir))
		}
	})
}
