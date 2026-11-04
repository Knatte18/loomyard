// ready_test.go covers a beam's ready record: which records flag which beam, and which record its removal takes.

package pawlengine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRemoveReady(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	setAt := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		// written is the slug of the record written first; empty writes none.
		written      string
		want         bool
		wantStanding bool
	}{
		{name: "OwnRecordIsRemoved", written: "b", want: true},
		{name: "AbsentRecordIsNotRemovedAndNothingIsCreated"},
		{name: "RecordOfAnotherBeamIsLeftStanding", written: "outer", wantStanding: true},
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
			got, err := pawlengine.RemoveReady(path, lockPath, "b")
			if err != nil {
				t.Fatalf("RemoveReady() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("RemoveReady(b) after a record of %q = %t; want %t", tt.written, got, tt.want)
			}
			_, statErr := os.Stat(path)
			if standing := statErr == nil; standing != tt.wantStanding {
				t.Errorf("record standing after RemoveReady(b) over a record of %q = %t; want %t", tt.written, standing, tt.wantStanding)
			}
			if tt.written == "" {
				if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
					t.Errorf("stat of the record's directory after RemoveReady(b) over no record = %v; want it not created", err)
				}
			}
		})
	}
}

func TestReadyFor(t *testing.T) {
	tiergate.Require(t, 0)
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
