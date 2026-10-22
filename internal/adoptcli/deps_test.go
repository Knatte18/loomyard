// deps_test.go covers the hub-only pause reading that makes a pair under a slug-scope pause read live.

package adoptcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

func TestHubOnlyPaused(t *testing.T) {
	t.Parallel()

	writeRecord := func(t *testing.T, live ...pauserecord.Entry) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		if _, err := pauserecord.Update(path, time.Unix(0, 0), func(record *pauserecord.Record) error {
			record.Live = live
			return nil
		}); err != nil {
			t.Fatalf("write the pause record: %v", err)
		}
		return path
	}

	t.Run("HubEntryCovers", func(t *testing.T) {
		t.Parallel()
		covered, err := hubOnlyPaused(writeRecord(t, pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: "operator"}))("alpha")
		if err != nil || !covered {
			t.Errorf("hubOnlyPaused(alpha) = %v, %v; want true under a hub entry", covered, err)
		}
	})

	t.Run("SlugEntryDoesNotCover", func(t *testing.T) {
		t.Parallel()
		covered, err := hubOnlyPaused(writeRecord(t, pauserecord.Entry{Scope: "alpha", Caller: pauserecord.OwnerCaller}))("alpha")
		if err != nil || covered {
			t.Errorf("hubOnlyPaused(alpha) = %v, %v; want false under its own slug entry", covered, err)
		}
	})

	t.Run("UnreadableRecordIsReturned", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write the broken record: %v", err)
		}
		if _, err := hubOnlyPaused(path)("alpha"); err == nil {
			t.Errorf("hubOnlyPaused over an undecodable record returned no error")
		}
	})
}
