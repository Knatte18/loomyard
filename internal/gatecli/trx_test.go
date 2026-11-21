// trx_test.go covers reading the TRX results files of a vstest invocation and folding them into the vstest counter.
// It reads fixture files from a temporary directory and starts no process.

package gatecli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func writeTRX(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadTRXResults(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const file = `<?xml version="1.0" encoding="utf-8"?>
<TestRun xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>
    <UnitTestResult testName="A" outcome="Passed" />
    <UnitTestResult testName="B" outcome="Failed"><Output><ErrorInfo><Message>boom
  second</Message><StackTrace>at B()</StackTrace></ErrorInfo></Output></UnitTestResult>
    <UnitTestResult testName="C" outcome="NotExecuted" />
  </Results>
  <ResultSummary outcome="Failed"><RunInfos>
    <RunInfo outcome="Error"><Text>host crashed
running: A</Text></RunInfo>
    <RunInfo outcome="Warning"><Text>no match</Text></RunInfo>
  </RunInfos></ResultSummary>
</TestRun>
`
	t.Run("sums the files, maps the outcomes and keeps only error run messages", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeTRX(t, dir, "a.trx", file)
		writeTRX(t, dir, "b.trx", file)
		writeTRX(t, dir, "ignored.txt", "not a results file")

		got, read := readTRXResults(dir)

		if !read {
			t.Fatal("readTRXResults() read = false; want true")
		}
		if want := (tierCounts{discovered: 6, passed: 2, failed: 2, skipped: 2}); got.counts != want {
			t.Errorf("readTRXResults() counts = %+v; want %+v", got.counts, want)
		}
		wantFailure := runFailure{test: "B", lines: []string{"boom", "second", "at B()"}}
		if diff := cmp.Diff([]runFailure{wantFailure, wantFailure}, got.failures, cmp.AllowUnexported(runFailure{})); diff != "" {
			t.Errorf("readTRXResults() failures mismatch (-want +got):\n%s", diff)
		}
		wantMessage := "host crashed; running: A"
		if diff := cmp.Diff([]string{wantMessage, wantMessage}, got.runMessages); diff != "" {
			t.Errorf("readTRXResults() run messages mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("reports nothing read for a malformed file, a missing directory and an empty one", func(t *testing.T) {
		t.Parallel()
		malformed := t.TempDir()
		writeTRX(t, malformed, "bad.trx", "<TestRun")
		for name, dir := range map[string]string{"malformed": malformed, "missing": filepath.Join(t.TempDir(), "none"), "empty": t.TempDir()} {
			if _, read := readTRXResults(dir); read {
				t.Errorf("readTRXResults(%s) read = true; want false", name)
			}
		}
	})
}
