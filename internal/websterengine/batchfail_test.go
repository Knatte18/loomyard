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

//testtiming:keep pins the archive stamp name, each transcript appended once, every suspect path in the way forward and the terminal failed digest; the covering record and recover tests observe each only in part
func TestFailBatch(t *testing.T) {
	t.Parallel()

	t.Run("archives the report and frees the live path", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("no report archives nothing", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("records the batch terminal failed", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("an unrendered amendment adds a card_amended reason and flag, a rendered one does not, and neither is uncheckable", func(t *testing.T) {
		t.Parallel()

		in, _ := failBatchFixture(t, true)
		in.Batch.AmendedCards = []AmendedCard{{Card: "03-alpha"}, {Card: "04-beta", Rendered: true}}
		bfe, err := failBatch(in)
		if err != nil {
			t.Fatal(err)
		}
		const want = "card 03-alpha was amended after this attempt began; recovery re-runs the batch on the amended card"
		if !bfe.CardAmended {
			t.Fatal("CardAmended = false, want true")
		}
		for name, reasons := range map[string][]string{"error": bfe.Reasons, "digest": in.Batch.Digest.Reasons} {
			if got := strings.Count(strings.Join(reasons, "\n"), want); got != 1 {
				t.Fatalf("%s reasons carry the amended reason %d times: %v", name, got, reasons)
			}
			if strings.Contains(strings.Join(reasons, "\n"), "04-beta") {
				t.Fatalf("%s reasons name the rendered amendment: %v", name, reasons)
			}
		}
		if len(in.Batch.Uncheckable) != 0 {
			t.Fatalf("Uncheckable = %v, want none", in.Batch.Uncheckable)
		}

		in, _ = failBatchFixture(t, true)
		in.Batch.AmendedCards = []AmendedCard{{Card: "03-alpha", Rendered: true}}
		bfe, err = failBatch(in)
		if err != nil {
			t.Fatal(err)
		}
		if bfe.CardAmended {
			t.Fatal("CardAmended = true over a rendered amendment")
		}
	})

	t.Run("appends each transcript once", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("the error carries its way forward and wraps ErrBatchFailed", func(t *testing.T) {
		t.Parallel()

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

		in, _ = failBatchFixture(t, true)
		in.WayForward = "edit the plan"
		bfe, err = failBatch(in)
		if err != nil {
			t.Fatal(err)
		}
		msg = bfe.Error()
		if !strings.HasSuffix(msg, "; way forward: edit the plan") || strings.Contains(msg, "recover-batch") {
			t.Errorf("error %q; want the alternative way forward replacing the recover-batch default", msg)
		}
	})
}
