//go:build tier0

// ready_test.go covers a beam's ready record: which records flag which beam.

package pawlengine_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
)

func TestReadyFor(t *testing.T) {
	t.Parallel()
	setAt := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		// written is the slug of the record written first; empty writes none.
		written string
		want    bool
	}{
		{name: "AbsentRecordIsNotFlagged"},
		{name: "OwnRecordIsFlagged", written: "b", want: true},
		{name: "RecordOfAnotherBeamIsNotFlagged", written: "outer", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "_lym", "landing", "beam-ready.json")
			lockPath := filepath.Join(dir, ".lym", "landing", "beam-ready.json.lock")
			if tt.written != "" {
				if err := pawlengine.WriteReady(path, lockPath, pawlengine.ReadyRecord{Slug: tt.written, SetAt: setAt}); err != nil {
					t.Fatalf("WriteReady() = %v; want nil", err)
				}
			}
			got, err := pawlengine.ReadyFor(path, lockPath, "b")
			if err != nil {
				t.Fatalf("ReadyFor() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("ReadyFor(b) after a record of %q = %t; want %t", tt.written, got, tt.want)
			}
		})
	}
}
