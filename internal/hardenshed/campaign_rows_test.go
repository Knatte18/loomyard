// campaign_rows_test.go covers the facts a Campaign renders for a round, its live-red answer and its halt.

package hardenshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeLiveReport writes the live report of its round into runDir.
func writeLiveReport(t *testing.T, runDir string, report LiveReport) {
	t.Helper()
	path := liveReportPath(runDir, report.Round)
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), report); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignRoundFacts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const fixerReport = "---\ndispositions:\n  - id: f1\n    disposition: deferred\n    reason: needs a refactor outside the module\n  - id: f2\n    disposition: fixed\nspawned: true\n---\nbody\n"
	tests := []struct {
		name  string
		setup func(t *testing.T, runDir string)
		want  []string
	}{
		{
			name:  "AFullRound",
			setup: fullRound,
			want: []string{
				"- Models: reviewer `opus`, fixer `fable`.",
				"- Live report: green. The `harden-live` run passed and started 2 test binaries.",
				"- Copies run: failed and started 2 test binaries.",
				"- Packages outside the stray-state cover: pkg/b.",
				"- Stray process 41 (sleep 1000) found by row Harden-Review at start, left by an interrupted row.",
				"- Stray server of hub /sandbox/hub found by row Harden-Review at end.",
				"- Fix commits: c1, c2.",
				"- Deferred by the fixer: `f1`: needs a refactor outside the module",
				"- Deferred by the red live report: none.",
			},
		},
		{
			name: "ARedLiveReportListsItsDeferredFailuresBesideTheFixersDeferral",
			setup: func(t *testing.T, runDir string) {
				writeLiveReport(t, runDir, LiveReport{Round: 1, Mark: MarkRed, Live: LiveRunReport{Started: 1}, Deferred: []string{"--- FAIL: TestSlow"}})
				if err := os.WriteFile(filepath.Join(runDir, "round-1-fixer-report.md"), []byte(fixerReport), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{
				"- Live report: red. The `harden-live` run failed and started 1 test binaries.",
				"- Deferred by the fixer: `f1`: needs a refactor outside the module",
				"- Deferred by the red live report: --- FAIL: TestSlow",
			},
		},
		{
			name: "AnAbsentLiveReportBesideTheCommitsFileIsRed",
			setup: func(t *testing.T, runDir string) {
				if err := writeRoundRecord(runDir, "commits", 1, RoundCommits{Round: 1, Head: "h"}); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"- Live report: absent, read as red because the round's fix row ran and wrote none.", "- Fix commits: none."},
		},
		{
			name: "ARefusedCopiesRunRendersItsRefusal",
			setup: func(t *testing.T, runDir string) {
				writeLiveReport(t, runDir, LiveReport{Round: 1, Mark: MarkGreen, Live: LiveRunReport{Passed: true, Started: 1}, Copies: &LiveRunReport{Refused: "the key names two tiers"}})
			},
			want: []string{"- Copies run: refused: the key names two tiers"},
		},
		{
			name: "AnUnparseableFixerReportIsNamedAndFailsNothing",
			setup: func(t *testing.T, runDir string) {
				if err := os.WriteFile(filepath.Join(runDir, "round-1-fixer-report.md"), []byte("no frontmatter\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"- Fixer report: unreadable: "},
		},
		{
			name:  "ARoundWithNoFiles",
			setup: func(*testing.T, string) {},
			want: []string{
				"- Models: absent.",
				"- Live report: absent.",
				"- Strays: the stray report is absent.",
				"- Fix commits: absent.",
				"- Fixer report: absent.",
				"- Deferred by the red live report: none.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			tt.setup(t, runDir)

			got, err := (&Campaign{}).RoundFacts(runDir, 1)
			if err != nil {
				t.Fatalf("RoundFacts() error = %v; want nil", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("RoundFacts() = %q; want it to hold the line %q", got, want)
				}
			}
		})
	}
}

// fullRound writes every file one round of a campaign leaves in runDir.
func fullRound(t *testing.T, runDir string) {
	t.Helper()
	if err := writeRoundRecord(runDir, "models", 1, RoundModels{Round: 1, Review: "opus", Fix: "fable"}); err != nil {
		t.Fatal(err)
	}
	writeLiveReport(t, runDir, LiveReport{
		Round: 1, Mark: MarkGreen,
		Live:              LiveRunReport{Passed: true, Started: 2},
		Copies:            &LiveRunReport{Started: 2},
		OutsideStrayCover: []string{"pkg/b"},
	})
	strays := []StrayEntry{
		{Row: "Harden-Review", Point: ReapStart, Kind: StrayProcess, PID: 41, Argv: []string{"sleep", "1000"}, Interrupted: true},
		{Row: "Harden-Review", Point: ReapEnd, Kind: StrayServer, Hub: "/sandbox/hub"},
	}
	if err := appendStrays(runDir, 1, strays); err != nil {
		t.Fatal(err)
	}
	if err := writeRoundRecord(runDir, "commits", 1, RoundCommits{Round: 1, Head: "h", Commits: []string{"c1", "c2"}}); err != nil {
		t.Fatal(err)
	}
	report := "---\ndispositions:\n  - id: f1\n    disposition: deferred\n    reason: needs a refactor outside the module\nspawned: true\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(runDir, "round-1-fixer-report.md"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignLiveRed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, runDir string)
		want  bool
	}{
		{"ARedReport", func(t *testing.T, runDir string) { writeLiveReport(t, runDir, LiveReport{Round: 1, Mark: MarkRed}) }, true},
		{"AGreenReport", func(t *testing.T, runDir string) { writeLiveReport(t, runDir, LiveReport{Round: 1, Mark: MarkGreen}) }, false},
		{"AnAbsentReportBesideTheCommitsFile", func(t *testing.T, runDir string) {
			if err := writeRoundRecord(runDir, "commits", 1, RoundCommits{Round: 1}); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"AnAbsentReportWithNoCommitsFile", func(*testing.T, string) {}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			tt.setup(t, runDir)
			if got, err := (&Campaign{}).LiveRed(runDir, 1); err != nil || got != tt.want {
				t.Errorf("LiveRed(round 1) = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

func TestCampaignHalt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	runDir := t.TempDir()
	for _, name := range []string{"round-1-review.md", "round-2-review.md"} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("review"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reapErr := errors.New("reap refused")
	campaign := &Campaign{RunDir: runDir, Reap: func(row string, point ReapPoint) ([]StrayEntry, error) {
		return []StrayEntry{{Row: row, Point: point, Kind: StrayServer, Hub: "hub"}}, reapErr
	}}

	if err := campaign.Halt("Harden-Bouncer"); !errors.Is(err, reapErr) {
		t.Errorf("Halt() error = %v; want it to wrap %v", err, reapErr)
	}
	report, found, err := readRoundRecord[StrayReport](runDir, "strays", 2)
	if err != nil || !found || len(report.Entries) != 1 || report.Entries[0].Point != ReapHalt || report.Entries[0].Row != "Harden-Bouncer" {
		t.Errorf("round 2 stray report = %+v, %v, %v; want the halt's entry for Harden-Bouncer", report, found, err)
	}
}
