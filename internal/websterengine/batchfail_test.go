package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestFailBatch_ArchivesReportAndFreesLivePath(t *testing.T) {
	in, live := failBatchFixture(t, true)
	bfe, err := failBatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(live); !os.IsNotExist(statErr) {
		t.Fatalf("live report path not freed: %v", statErr)
	}
	want := filepath.Join(in.ReportsDir, "03-alpha-20260930T120000Z.yaml")
	if bfe.ArchivedReport != want {
		t.Fatalf("ArchivedReport = %q, want %q", bfe.ArchivedReport, want)
	}
	if _, statErr := os.Stat(want); statErr != nil {
		t.Fatalf("archived report missing: %v", statErr)
	}
}

func TestFailBatch_NoReportArchivesNothing(t *testing.T) {
	in, _ := failBatchFixture(t, false)
	bfe, err := failBatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if bfe.ArchivedReport != "" {
		t.Fatalf("ArchivedReport = %q, want empty", bfe.ArchivedReport)
	}
	if strings.Contains(bfe.Error(), "report archived") {
		t.Fatalf("error mentions an archive: %s", bfe.Error())
	}
}

func TestFailBatch_RecordsTerminalFailed(t *testing.T) {
	in, _ := failBatchFixture(t, true)
	in.SuspectPaths = []string{"a/b.go"}
	if _, err := failBatch(in); err != nil {
		t.Fatal(err)
	}
	b := in.Batch
	if !b.Terminal || b.Status != DigestStatusFailed {
		t.Fatalf("batch = terminal %v status %q", b.Terminal, b.Status)
	}
	if b.Digest == nil || b.Digest.Status != DigestStatusFailed || b.Digest.HeadSHA != "abc" || b.Digest.Batch != "03-alpha" {
		t.Fatalf("digest = %+v", b.Digest)
	}
	joined := strings.Join(b.Digest.Reasons, "\n")
	if !strings.Contains(joined, "correctness finding") || !strings.Contains(joined, "a/b.go") {
		t.Fatalf("reasons = %v", b.Digest.Reasons)
	}
	if in.State.CurrentBatch != 0 {
		t.Fatalf("CurrentBatch = %d, want 0", in.State.CurrentBatch)
	}
}

func TestFailBatch_TranscriptsAppendedOnce(t *testing.T) {
	in, _ := failBatchFixture(t, true)
	in.NewTranscripts = []string{"t1", "t1", "t2"}
	if _, err := failBatch(in); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(in.State.SeenForkTranscripts, ","); got != "t1,t2" {
		t.Fatalf("SeenForkTranscripts = %s", got)
	}
	if got := strings.Join(in.Batch.ForkTranscripts, ","); got != "t1,t2" {
		t.Fatalf("ForkTranscripts = %s", got)
	}
}

func TestBatchFailedError_TextAndSentinel(t *testing.T) {
	in, _ := failBatchFixture(t, true)
	in.SuspectPaths = []string{"x/one.go", "x/two.go"}
	bfe, err := failBatch(in)
	if err != nil {
		t.Fatal(err)
	}
	msg := bfe.Error()
	for _, want := range []string{"lyx webster recover-batch 03", "x/one.go", "x/two.go", "way forward:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if !errors.Is(bfe, ErrBatchFailed) {
		t.Fatal("errors.Is(err, ErrBatchFailed) = false")
	}
}
