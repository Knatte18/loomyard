// roundfiles_test.go table-drives roundToken/artifactPaths' naming shape: the bare round token for
// a first attempt, the letter-suffixed token for a retry, and the round-<token>-<kind>.md file
// names each produces inside a run dir.
// buildRoundProfile's own field-mapping test belongs with whichever runner adapter owns that
// mapping, never here — this package defines the paths, not the mapping onto a runner's input.

package treadleengine

import (
	"path/filepath"
	"testing"
)

func TestRoundToken(t *testing.T) {
	tests := []struct {
		name    string
		round   int
		attempt int
		want    string
	}{
		{"first attempt has no suffix", 3, 1, "3"},
		{"second attempt gets b", 3, 2, "3b"},
		{"third attempt gets c", 3, 3, "3c"},
		{"round one first attempt", 1, 1, "1"},
		{"round ten second attempt", 10, 2, "10b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundToken(tt.round, tt.attempt)
			if got != tt.want {
				t.Errorf("roundToken(%d, %d) = %q; want %q", tt.round, tt.attempt, got, tt.want)
			}
		})
	}
}

func TestArtifactPaths(t *testing.T) {
	runDir := filepath.Join("run", "dir")

	tests := []struct {
		name    string
		round   int
		attempt int
		want    roundArtifactPaths
	}{
		{
			name:    "first attempt of round 3",
			round:   3,
			attempt: 1,
			want: roundArtifactPaths{
				Review:      filepath.Join(runDir, "round-3-review.md"),
				FixerReport: filepath.Join(runDir, "round-3-fixer-report.md"),
				Judge:       filepath.Join(runDir, "round-3-judge.md"),
				Handoff:     filepath.Join(runDir, "round-3-handoff.md"),
				Gate:        filepath.Join(runDir, "round-3-gate.md"),
				Triage:      filepath.Join(runDir, "round-3-triage.md"),
				Seed:        filepath.Join(runDir, "round-3-seed.md"),
			},
		},
		{
			name:    "second attempt of round 3",
			round:   3,
			attempt: 2,
			want: roundArtifactPaths{
				Review:      filepath.Join(runDir, "round-3b-review.md"),
				FixerReport: filepath.Join(runDir, "round-3b-fixer-report.md"),
				Judge:       filepath.Join(runDir, "round-3b-judge.md"),
				Handoff:     filepath.Join(runDir, "round-3b-handoff.md"),
				Gate:        filepath.Join(runDir, "round-3b-gate.md"),
				Triage:      filepath.Join(runDir, "round-3b-triage.md"),
				Seed:        filepath.Join(runDir, "round-3b-seed.md"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artifactPaths(runDir, tt.round, tt.attempt)
			if got != tt.want {
				t.Errorf("artifactPaths(%q, %d, %d) = %+v; want %+v", runDir, tt.round, tt.attempt, got, tt.want)
			}
		})
	}
}
