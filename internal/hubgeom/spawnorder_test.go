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

// TestRunStartTime pins that a pair's start time is its run seed's modification time, and that an absent seed has none.
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
	if err := os.WriteFile(seed, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	if err := os.Chtimes(seed, want, want); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, ok := runStartTime(location)
	if !ok || !got.Equal(want) {
		t.Errorf("runStartTime() = (%v, %v), want (%v, true)", got, ok, want)
	}
}
