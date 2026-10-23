// amendment_test.go covers the plan amendment record: its append-only writes, tolerant read, removal and the settle rule.

package loomshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAmendmentRecord(t *testing.T) {
	t.Parallel()

	written := time.Date(2026, 10, 23, 7, 30, 15, 123456789, time.UTC)

	t.Run("an absent record reads not found", func(t *testing.T) {
		t.Parallel()
		_, found, err := ReadAmendment(filepath.Join(t.TempDir(), "amendment.yaml"))
		if err != nil || found {
			t.Errorf("ReadAmendment(absent) = found %v, err %v; want not found and no error", found, err)
		}
	})

	t.Run("appends keep earlier titles and move the write time", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nested", "amendment.yaml")
		if err := AppendAmendment(path, "first", written); err != nil {
			t.Fatalf("AppendAmendment(first) = %v; want nil", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "2026-10-23T07:30:15.123456789Z") {
			t.Errorf("amendment record = %q; want the write time at nanosecond precision", raw)
		}
		record, found, err := ReadAmendment(path)
		if err != nil || !found || len(record.Titles) != 1 || record.Titles[0] != "first" || !record.WrittenAt.Equal(written) {
			t.Fatalf("ReadAmendment() = %+v, found %v, err %v; want one title %q written at %v", record, found, err, "first", written)
		}

		later := written.Add(time.Minute).In(time.FixedZone("offset", 3600))
		if err := AppendAmendment(path, "second", later); err != nil {
			t.Fatalf("AppendAmendment(second) = %v; want nil", err)
		}
		record, _, err = ReadAmendment(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Titles) != 2 || record.Titles[0] != "first" || record.Titles[1] != "second" || !record.WrittenAt.Equal(later) {
			t.Errorf("ReadAmendment() = %+v; want titles first and second written at %v", record, later)
		}
	})

	t.Run("a record with an unknown key reads", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "amendment.yaml")
		if err := os.WriteFile(path, []byte("titles: [only]\nwritten_at: 2026-10-23T07:30:15Z\nfuture_key: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		record, found, err := ReadAmendment(path)
		if err != nil || !found || len(record.Titles) != 1 || record.Titles[0] != "only" {
			t.Errorf("ReadAmendment() = %+v, found %v, err %v; want the one title read", record, found, err)
		}
	})

	t.Run("an undecodable file is an error naming the path and the way forward", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "amendment.yaml")
		if err := os.WriteFile(path, []byte("titles: [unclosed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := ReadAmendment(path)
		if err == nil {
			t.Fatal("ReadAmendment(undecodable) = nil; want an error")
		}
		for _, want := range []string{path, "rewrite the file as a valid record", "remove it when no amendment is pending", "lyx fabric commit", "re-run the refused step"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ReadAmendment(undecodable) error = %q; want it to contain %q", err.Error(), want)
			}
		}
		if appendErr := AppendAmendment(path, "title", written); appendErr == nil || appendErr.Error() != err.Error() {
			t.Errorf("AppendAmendment(undecodable) = %v; want the read's error unchanged", appendErr)
		}
	})

	t.Run("removal deletes the record and tolerates an absent file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "amendment.yaml")
		if err := RemoveAmendment(path); err != nil {
			t.Errorf("RemoveAmendment(absent) = %v; want nil", err)
		}
		if err := AppendAmendment(path, "title", written); err != nil {
			t.Fatal(err)
		}
		if err := RemoveAmendment(path); err != nil {
			t.Errorf("RemoveAmendment() = %v; want nil", err)
		}
		if _, found, _ := ReadAmendment(path); found {
			t.Error("ReadAmendment() after removal found a record; want none")
		}
	})

	settled := []struct {
		name       string
		roundStart time.Time
		want       bool
	}{
		{"a round start after the write", written.Truncate(time.Second).Add(time.Second), true},
		{"a round start before the write", written.Add(-time.Minute), false},
		{"a round start equal to the write", written, false},
		{"a write later in the start's own second", written.Truncate(time.Second), false},
		{"a zero round start", time.Time{}, false},
	}
	for _, tt := range settled {
		t.Run("AmendmentSettledBy "+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := AmendmentSettledBy(AmendmentRecord{WrittenAt: written}, tt.roundStart); got != tt.want {
				t.Errorf("AmendmentSettledBy(%v) = %v; want %v", tt.roundStart, got, tt.want)
			}
		})
	}
}
