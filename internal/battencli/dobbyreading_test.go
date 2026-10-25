//go:build tier0

// dobbyreading_test.go pins what the pass wiring reads from the parent worktree's dobby: its pid, its recorded sleeps and the shared awake clock.

package battencli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

func TestDobbyReading(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	slept := pauserecord.Interval{Start: base.Add(10 * time.Minute), End: base.Add(40 * time.Minute)}
	const dobbyPID = 424242

	tests := []struct {
		name       string
		live       bool
		record     func(t *testing.T, path string)
		wantPID    int
		wantSleeps []pauserecord.Interval
	}{
		{name: "DeadDobbyReadsThisProcess", live: false, record: writeDobbyState(dobbyPID, slept), wantPID: os.Getpid()},
		{name: "MissingRecordReadsThisProcess", live: true, wantPID: os.Getpid()},
		{name: "LiveDobbyReadsItsPIDAndSleeps", live: true, record: writeDobbyState(dobbyPID, slept), wantPID: dobbyPID, wantSleeps: []pauserecord.Interval{slept}},
		{name: "UndecodableRecordReadsAsNoDobby", live: true, record: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write undecodable record: %v", err)
			}
		}, wantPID: os.Getpid()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "state.json")
			if tt.record != nil {
				tt.record(t, path)
			}
			pid, sleeps, err := dobbyStateReading(path, func() (bool, error) { return tt.live, nil })
			if err != nil {
				t.Fatalf("dobbyStateReading() error = %v; want nil", err)
			}
			if pid != tt.wantPID || !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("dobbyStateReading() = %d, %v; want %d, %v", pid, sleeps, tt.wantPID, tt.wantSleeps)
			}
		})
	}

	t.Run("AwakeReadingsDifferByTheWallGapLessTheSleep", func(t *testing.T) {
		t.Parallel()
		before, after := base, base.Add(time.Hour)
		gap := awakeReading(after, []pauserecord.Interval{slept}) - awakeReading(before, []pauserecord.Interval{slept})
		if want := time.Hour - slept.End.Sub(slept.Start); gap != want {
			t.Errorf("awake readings an hour apart with a thirty-minute sleep between differ by %v; want %v", gap, want)
		}
	})
}

// writeDobbyState returns a record writer that stores a state holding pid and sleeps.
func writeDobbyState(pid int, sleeps ...pauserecord.Interval) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		err := dobbyengine.UpdateState(path, func(s *dobbyengine.State) error {
			s.PID, s.Sleeps = pid, sleeps
			return nil
		})
		if err != nil {
			t.Fatalf("write dobby state: %v", err)
		}
	}
}
