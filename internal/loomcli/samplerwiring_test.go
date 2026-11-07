// samplerwiring_test.go covers the watch on the Sampler-Session row's record.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOpenRecordWatch pins that the watch delivers when the record is written after it opened,
// and that it is nil when the record's directory cannot be created.
func TestOpenRecordWatch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a record write is delivered", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "scratch", "sampler.json")
		changed := openRecordWatch(path)
		if changed == nil {
			t.Fatal("openRecordWatch() = nil; want a channel")
		}
		if err := samplerengine.UpdateRecord(path, path+".lock", func(record *samplerengine.Record) { record.DoneStamp = 1 }); err != nil {
			t.Fatalf("UpdateRecord() = %v; want nil", err)
		}
		select {
		case <-changed:
		case <-time.After(10 * time.Second):
			t.Error("openRecordWatch() delivered nothing for a record write within the deadline")
		}
	})

	t.Run("a directory that cannot be created leaves the row on its timer", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "regular-file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", file, err)
		}
		if changed := openRecordWatch(filepath.Join(file, "sampler.json")); changed != nil {
			t.Error("openRecordWatch() under a regular file = a channel; want nil")
		}
	})
}
