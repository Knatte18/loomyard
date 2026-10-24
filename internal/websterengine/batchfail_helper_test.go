// batchfail_helper_test.go holds the failed-batch fixture and the verify-fix prompt path the tier0 and tier1 tests of this package share.

package websterengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testFixPromptPath is the verify-fix prompt path the gate tests hand the gate.
const testFixPromptPath = "/prompts/verify-fix.md"

func failBatchFixture(t *testing.T, withReport bool) (failBatchInput, string) {
	t.Helper()
	dir := t.TempDir()
	live := filepath.Join(dir, ReportFileName(3, "alpha"))
	if withReport {
		if err := os.WriteFile(live, []byte("status: OK\nhead_sha: abc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := &State{CurrentBatch: 3}
	return failBatchInput{
		State:      st,
		Batch:      &BatchState{},
		Number:     3,
		Slug:       "alpha",
		ReportsDir: dir,
		HeadSHA:    "abc",
		Reasons:    []string{"correctness finding"},
		Now:        func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
	}, live
}
