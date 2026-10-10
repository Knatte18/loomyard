// spawnorder_test.go pins the spawn order the teller hands reed: the sort rule and where a pair's start time is read from.

package hubgeom

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

func spawnCandidateNamed(name string, prime bool, started time.Time, dated bool) spawnCandidate {
	return spawnCandidate{location: &lyxcwd.Location{WorktreeName: name}, prime: prime, started: started, dated: dated}
}

// TestSortSpawnOrder pins the ordering rule: the prime first whatever its time, dated pairs ascending, undated pairs after them by name, ties by name.
func TestSortSpawnOrder(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		input []spawnCandidate
		want  []string
	}{
		{
			name: "PrimeFirstWhateverItsTime",
			input: []spawnCandidate{
				spawnCandidateNamed("early", false, base, true),
				spawnCandidateNamed("main", true, base.Add(time.Hour), true),
			},
			want: []string{"main", "early"},
		},
		{
			name: "DatedPairsAscending",
			input: []spawnCandidate{
				spawnCandidateNamed("late", false, base.Add(2*time.Hour), true),
				spawnCandidateNamed("early", false, base, true),
				spawnCandidateNamed("middle", false, base.Add(time.Hour), true),
			},
			want: []string{"early", "middle", "late"},
		},
		{
			name: "UndatedAfterDatedByName",
			input: []spawnCandidate{
				spawnCandidateNamed("zulu", false, time.Time{}, false),
				spawnCandidateNamed("dated", false, base, true),
				spawnCandidateNamed("alpha", false, time.Time{}, false),
			},
			want: []string{"dated", "alpha", "zulu"},
		},
		{
			name: "EqualTimesBreakByName",
			input: []spawnCandidate{
				spawnCandidateNamed("b", false, base, true),
				spawnCandidateNamed("a", false, base, true),
			},
			want: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for _, candidate := range sortSpawnOrder(tt.input) {
				got = append(got, candidate.location.WorktreeName)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("sortSpawnOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRunStartTime pins that a pair's start time is its run seed's started_at stamp, that a seed without a parseable stamp or an absent seed has none, and that the seed file's modification time is never read.
func TestRunStartTime(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	location := &lyxcwd.Location{HubPath: hub, WorktreeName: "pair", AnchorRel: "."}

	if _, ok := runStartTime(location); ok {
		t.Error("runStartTime() with no seed file = ok, want none")
	}

	seed := shedrun.SeedFile(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(seed), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(seed, []byte(`{"recipe":"loom","driver":"go"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := runStartTime(location); ok {
		t.Error("runStartTime() with a seed without the stamp = ok, want none")
	}
	if err := os.WriteFile(seed, []byte(`{"recipe":"loom","driver":"go","started_at":"not a time"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := runStartTime(location); ok {
		t.Error("runStartTime() with an unparseable stamp = ok, want none")
	}

	if err := os.Remove(seed); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	stamped := shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo, StartedAt: want.Format(time.RFC3339)}
	if err := shedrun.WriteSeed(location, shedrun.SelfRunID, stamped); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	if got, ok := runStartTime(location); !ok || !got.Equal(want) {
		t.Errorf("runStartTime() = (%v, %v), want (%v, true)", got, ok, want)
	}

	// A re-written agreeing seed keeps its time whatever the file's mtime.
	touched := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(seed, touched, touched); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if err := shedrun.WriteSeed(location, shedrun.SelfRunID, stamped); err != nil {
		t.Fatalf("second WriteSeed: %v", err)
	}
	if got, ok := runStartTime(location); !ok || !got.Equal(want) {
		t.Errorf("runStartTime() after a rewrite and a touch = (%v, %v), want (%v, true)", got, ok, want)
	}

	// A seed written without a stamp is dated by WriteSeed.
	unstamped := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	if err := shedrun.WriteSeed(unstamped, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed unstamped: %v", err)
	}
	if got, ok := runStartTime(unstamped); !ok || got.IsZero() {
		t.Errorf("runStartTime() after an unstamped WriteSeed = (%v, %v), want a date", got, ok)
	}
}
