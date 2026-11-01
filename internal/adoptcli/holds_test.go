// holds_test.go pins the holds and fix-tries files of a subject and the pause-record entries that hold a slug.

package adoptcli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHoldStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AppendKeepsOneOfEachSlugAndRemoveDropsOnlyTheNamed", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := appendHolds(dir, []string{"a", "b"}); err != nil {
			t.Fatalf("appendHolds: %v", err)
		}
		if err := appendHolds(dir, []string{"b", "c"}); err != nil {
			t.Fatalf("appendHolds again: %v", err)
		}
		if got, err := readHolds(dir); err != nil || !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Fatalf("readHolds() = %v, %v; want a, b, c once each", got, err)
		}
		if err := removeHolds(dir, []string{"b"}); err != nil {
			t.Fatalf("removeHolds: %v", err)
		}
		if got, err := readHolds(dir); err != nil || !slices.Equal(got, []string{"a", "c"}) {
			t.Errorf("readHolds() after the removal = %v, %v; want a, c", got, err)
		}
	})

	t.Run("AbsentFilesReadEmptyAndCreateNothing", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "subject")
		if got, err := readHolds(dir); err != nil || len(got) != 0 {
			t.Errorf("readHolds() = %v, %v; want none", got, err)
		}
		if got, err := readTries(dir); err != nil || got != 0 {
			t.Errorf("readTries() = %d, %v; want 0", got, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("Stat(%s) error = %v; want a read to create no directory", dir, err)
		}
	})

	t.Run("TriesReadWhatWasWritten", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := writeTries(dir, 2); err != nil {
			t.Fatalf("writeTries: %v", err)
		}
		if got, err := readTries(dir); err != nil || got != 2 {
			t.Errorf("readTries() = %d, %v; want 2", got, err)
		}
	})

	t.Run("AFileHoldingAnUnknownKeyReadsAndKeepsIt", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, holdsFileName)
		if err := os.WriteFile(path, []byte(`{"slugs":["a"],"newer":7}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, err := readHolds(dir); err != nil || !slices.Equal(got, []string{"a"}) {
			t.Fatalf("readHolds() = %v, %v; want a", got, err)
		}
		if err := appendHolds(dir, []string{"b"}); err != nil {
			t.Fatalf("appendHolds: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"newer"`) {
			t.Errorf("holds file = %s; want the unknown key kept", data)
		}
	})
}

func TestHoldEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	paused := time.Unix(100, 0)
	now := time.Unix(500, 0)
	tests := []struct {
		name       string
		live       []pauserecord.Entry
		wantLive   []pauserecord.Entry
		wantHeld   []string
		wantPaused map[string]string
	}{
		{
			name:       "OwnerEntryTurnsIntoAHoldKeepingItsPauseTime",
			live:       []pauserecord.Entry{{Scope: "a", Caller: pauserecord.OwnerCaller, PausedAt: paused}},
			wantLive:   []pauserecord.Entry{{Scope: "a", Caller: pauserecord.HoldCaller, PausedAt: paused}},
			wantHeld:   []string{"a"},
			wantPaused: map[string]string{},
		},
		{
			name:       "ExistingHoldStaysOne",
			live:       []pauserecord.Entry{{Scope: "a", Caller: pauserecord.HoldCaller, PausedAt: paused}},
			wantLive:   []pauserecord.Entry{{Scope: "a", Caller: pauserecord.HoldCaller, PausedAt: paused}},
			wantHeld:   []string{"a"},
			wantPaused: map[string]string{},
		},
		{
			name:       "OperatorEntryStaysAndItsSlugIsReturnedPausedWithItsCaller",
			live:       []pauserecord.Entry{{Scope: "a", Caller: "operator", PausedAt: paused}},
			wantLive:   []pauserecord.Entry{{Scope: "a", Caller: "operator", PausedAt: paused}},
			wantPaused: map[string]string{"a": "operator"},
		},
		{
			name:       "UncoveredSlugGetsANewHoldAtNowAndAHubEntryIsNoCover",
			live:       []pauserecord.Entry{{Scope: pauserecord.ScopeHub, Caller: "operator", PausedAt: paused}},
			wantLive:   []pauserecord.Entry{{Scope: pauserecord.ScopeHub, Caller: "operator", PausedAt: paused}, {Scope: "a", Caller: pauserecord.HoldCaller, PausedAt: now, GraceEnd: now}},
			wantHeld:   []string{"a"},
			wantPaused: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record := pauserecord.Record{Live: slices.Clone(tt.live)}
			held, pausedBy := holdEntries(&record, []string{"a"}, now)
			if !slices.Equal(held, tt.wantHeld) || !reflect.DeepEqual(pausedBy, tt.wantPaused) {
				t.Errorf("holdEntries() = %v, %v; want %v, %v", held, pausedBy, tt.wantHeld, tt.wantPaused)
			}
			if !reflect.DeepEqual(record.Live, tt.wantLive) {
				t.Errorf("live entries = %+v; want %+v", record.Live, tt.wantLive)
			}
		})
	}
}
