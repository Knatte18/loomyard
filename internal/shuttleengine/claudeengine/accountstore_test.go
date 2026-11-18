// accountstore_test.go tests the account file and the usage records: their merge, their reads and their rewrites.

package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMergeUsage pins the usage merge and the reads and rewrites of the two cache files.
func TestMergeUsage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	window := func(percentage float64, resetsAt int64) usageWindow {
		return usageWindow{UsedPercentage: percentage, ResetsAt: resetsAt}
	}
	record := func(windows map[string]usageWindow) usageRecord { return usageRecord{Windows: windows} }
	blockedUntil := time.Unix(5_000, 0).UTC()

	merges := []struct {
		name         string
		stored, read usageRecord
		want         usageRecord
	}{
		{"ALaterResetReplaces", record(map[string]usageWindow{"five_hour": window(90, 100)}), record(map[string]usageWindow{"five_hour": window(10, 200)}), record(map[string]usageWindow{"five_hour": window(10, 200)})},
		{"TheSameResetKeepsTheHigherPercentage", record(map[string]usageWindow{"five_hour": window(40, 100)}), record(map[string]usageWindow{"five_hour": window(60, 100)}), record(map[string]usageWindow{"five_hour": window(60, 100)})},
		{"TheSameResetWithALowerPercentageKeepsTheStoredWindow", record(map[string]usageWindow{"five_hour": window(60, 100)}), record(map[string]usageWindow{"five_hour": window(40, 100)}), record(map[string]usageWindow{"five_hour": window(60, 100)})},
		{"AnEarlierResetIsDropped", record(map[string]usageWindow{"five_hour": window(10, 200)}), record(map[string]usageWindow{"five_hour": window(90, 100)}), record(map[string]usageWindow{"five_hour": window(10, 200)})},
		{"AWindowOnlyOneSideHoldsIsKept", record(map[string]usageWindow{"five_hour": window(10, 100)}), record(map[string]usageWindow{"seven_day": window(20, 900)}), record(map[string]usageWindow{"five_hour": window(10, 100), "seven_day": window(20, 900)})},
		{"BlockedUntilSurvivesAMerge", usageRecord{BlockedUntil: blockedUntil}, record(map[string]usageWindow{"five_hour": window(10, 100)}), usageRecord{Windows: map[string]usageWindow{"five_hour": window(10, 100)}, BlockedUntil: blockedUntil}},
	}
	for _, tt := range merges {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mergeUsage(tt.stored, tt.read); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeUsage(%+v, %+v) = %+v; want %+v", tt.stored, tt.read, got, tt.want)
			}
		})
	}

	t.Run("AMissingUsageFileReadsAsTheZeroRecord", func(t *testing.T) {
		t.Parallel()

		got, err := readUsage(t.TempDir(), []AccountConfig{{Name: "a"}})
		if want := map[string]usageRecord{"a": {}}; err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("readUsage() = %+v, %v; want %+v, nil", got, err, want)
		}
	})

	t.Run("AnUndecodableAccountFileAndUsageFileReadAsZeroRecords", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		for _, path := range []string{accountFilePath(dir), usageFilePath(dir, "a")} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create the directory of %s: %v", path, err)
			}
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
		if file, err := readAccountFile(dir); err != nil || !reflect.DeepEqual(file, accountFile{}) {
			t.Errorf("readAccountFile() = %+v, %v; want the zero file, nil", file, err)
		}
		if usage, err := readUsage(dir, []AccountConfig{{Name: "a"}}); err != nil || !reflect.DeepEqual(usage["a"], usageRecord{}) {
			t.Errorf("readUsage() = %+v, %v; want the zero record, nil", usage, err)
		}
	})

	t.Run("RecordBlockedKeepsTheLaterTimeAndTheWindowsAndWriteUsageKeepsTheBlock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		later, earlier := time.Unix(9_000, 0).UTC(), time.Unix(8_000, 0).UTC()
		steps := []func() error{
			func() error {
				return writeUsage(dir, "a", record(map[string]usageWindow{"five_hour": window(30, 100)}))
			},
			func() error { return recordBlocked(dir, "a", later) },
			func() error { return recordBlocked(dir, "a", earlier) },
			func() error {
				return writeUsage(dir, "a", record(map[string]usageWindow{"five_hour": window(50, 100)}))
			},
		}
		for i, step := range steps {
			if err := step(); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		}
		got, err := readUsage(dir, []AccountConfig{{Name: "a"}})
		want := usageRecord{Windows: map[string]usageWindow{"five_hour": window(50, 100)}, BlockedUntil: later}
		if err != nil || !got["a"].BlockedUntil.Equal(later) || !reflect.DeepEqual(got["a"].Windows, want.Windows) {
			t.Errorf("readUsage() = %+v, %v; want %+v", got["a"], err, want)
		}
	})

	t.Run("UpdateAccountFileKeepsAnUnknownKeyAndReplacesAnUndecodableFile", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := accountFilePath(dir)
		if err := os.WriteFile(path, []byte(`{"counts":{"a":1},"future_key":7}`), 0o644); err != nil {
			t.Fatalf("write the account file: %v", err)
		}
		bump := func(file accountFile) (accountFile, error) {
			if file.Counts == nil {
				file.Counts = map[string]float64{}
			}
			file.Counts["a"]++
			return file, nil
		}
		if err := updateAccountFile(dir, bump); err != nil {
			t.Fatalf("updateAccountFile() = %v; want nil", err)
		}
		data, err := os.ReadFile(path)
		var raw map[string]json.RawMessage
		if err != nil || json.Unmarshal(data, &raw) != nil || string(raw["future_key"]) != "7" {
			t.Fatalf("account file after the update = %s, %v; want the unknown key kept", data, err)
		}
		if file, err := readAccountFile(dir); err != nil || file.Counts["a"] != 2 {
			t.Errorf("readAccountFile() = %+v, %v; want a's count 2", file, err)
		}

		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write the undecodable account file: %v", err)
		}
		if err := updateAccountFile(dir, bump); err != nil {
			t.Fatalf("updateAccountFile() over an undecodable file = %v; want nil", err)
		}
		if file, err := readAccountFile(dir); err != nil || file.Counts["a"] != 1 {
			t.Errorf("readAccountFile() = %+v, %v; want a's count 1 over the zero file", file, err)
		}
	})
}
