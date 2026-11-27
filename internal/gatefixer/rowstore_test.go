// rowstore_test.go covers the row fixer's record store.
// It pins the round trip through records.json, the keys a newer binary wrote, the live record and the terminal mark.

package gatefixer

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRowStore pins the records file's round trip and its keeping of unknown keys, the live read and the terminal mark of one guid.
func TestRowStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RecordsRoundTripAndSaveRewritesByGuid", func(t *testing.T) {
		t.Parallel()
		store := newRowStore(t.TempDir(), "verify")
		first := Record{StrandGUID: "a", SpawnedAt: "1970-01-01T00:00:00Z", ReportPath: "/r/report-1.md", ReplyPath: "/r/reply-1.md"}
		if err := store.Save(&first); err != nil {
			t.Fatalf("Save(a) error = %v; want none", err)
		}
		if err := store.Save(&Record{StrandGUID: "b"}); err != nil {
			t.Fatalf("Save(b) error = %v; want none", err)
		}
		first.EventsOffset = 42
		if err := store.Save(&first); err != nil {
			t.Fatalf("Save(a again) error = %v; want none", err)
		}

		records, err := store.load()
		if err != nil || len(records) != 2 || records[0] != first || records[1].StrandGUID != "b" {
			t.Errorf("load() = %+v, %v; want a rewritten in place, then b", records, err)
		}
		if count, err := store.count(); err != nil || count != 2 {
			t.Errorf("count() = %d, %v; want 2", count, err)
		}
		if report, reply := store.reportPath(3), store.replyPath(3); !strings.HasSuffix(report, "verify/report-3.md") || !strings.HasSuffix(reply, "verify/reply-3.md") {
			t.Errorf("reportPath(3) = %q, replyPath(3) = %q; want report-3.md and reply-3.md in the entry directory", report, reply)
		}
	})

	t.Run("AnUnknownKeyInTheFileSurvivesARewrite", func(t *testing.T) {
		t.Parallel()
		store := newRowStore(t.TempDir(), "verify")
		if err := store.Save(&Record{StrandGUID: "a", Status: StatusDone}); err != nil {
			t.Fatalf("Save(a) error = %v; want none", err)
		}
		seeded := `[{"strandGuid":"a","spawnedAt":"","status":"done","fromNewerBinary":"keep me"}]`
		if err := os.WriteFile(store.recordsPath(), []byte(seeded), 0o644); err != nil {
			t.Fatalf("seed records file: %v", err)
		}

		if err := store.Save(&Record{StrandGUID: "b"}); err != nil {
			t.Fatalf("Save(b) error = %v; want none", err)
		}
		data, err := os.ReadFile(store.recordsPath())
		if err != nil || !strings.Contains(string(data), `"fromNewerBinary": "keep me"`) {
			t.Errorf("records file = %q, %v; want the unknown key kept", data, err)
		}
	})

	t.Run("LiveReturnsOnlyARecordWithNoTerminalStatus", func(t *testing.T) {
		t.Parallel()
		store := newRowStore(t.TempDir(), "verify")
		if live, err := store.Live(); err != nil || live != nil {
			t.Fatalf("Live() on an absent file = %+v, %v; want nil", live, err)
		}
		for _, rec := range []Record{{StrandGUID: "old", Status: StatusDone}, {StrandGUID: "now"}, {StrandGUID: "dead", Status: StatusDead}} {
			if err := store.Save(&rec); err != nil {
				t.Fatalf("Save(%s) error = %v; want none", rec.StrandGUID, err)
			}
		}
		if live, err := store.Live(); err != nil || live == nil || live.StrandGUID != "now" {
			t.Errorf("Live() = %+v, %v; want the record now", live, err)
		}
	})

	t.Run("MarkTerminalChangesOneRecordAndRefusesAnUnknownGuid", func(t *testing.T) {
		t.Parallel()
		store := newRowStore(t.TempDir(), "verify")
		for _, guid := range []string{"a", "b"} {
			if err := store.Save(&Record{StrandGUID: guid}); err != nil {
				t.Fatalf("Save(%s) error = %v; want none", guid, err)
			}
		}
		if err := store.markTerminal("a", StatusDead, DeadReasonTimeout); err != nil {
			t.Fatalf("markTerminal(a) error = %v; want none", err)
		}
		records, _ := store.load()
		if records[0].Status != StatusDead || records[0].DeadReason != DeadReasonTimeout || records[1].Status != "" {
			t.Errorf("records = %+v; want a dead for timeout and b untouched", records)
		}
		if err := store.markTerminal("missing", StatusDone, ""); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Errorf("markTerminal(missing) error = %v; want one naming the guid", err)
		}
	})
}
