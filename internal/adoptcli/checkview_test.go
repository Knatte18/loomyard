// checkview_test.go pins the adopt check's view in `lym beam status`:
// what each state shows, which paths its lines name, and which subjects' views the hub's scratch directory yields.

package adoptcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCheckView(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := filepath.Join("hub", fabricengine.BoltDirName, ".lym", "adopt", "alpha")
	logPath, outPath := filepath.Join(dir, suiteLogName), filepath.Join(dir, checkOutName)
	one, two := 1, 2
	ended := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	done := []CheckTier{{Tier: 0, Outcome: checkGreen}, {Tier: 1, Outcome: checkFailed}}

	tests := []struct {
		name      string
		record    CheckRecord
		alive     bool
		wantState string
		wantLines []string
		// wantRunning is the tier the view holds in flight, nil for none.
		wantRunning *int
	}{
		{
			name:      "RunningAliveShowsTheTierInFlightAndTheTiersDone",
			record:    CheckRecord{Subject: "alpha", State: checkRunning, Running: &two, Tiers: done[:1]},
			alive:     true,
			wantState: "running", wantRunning: &two,
			wantLines: []string{"adopt check of alpha: running", "tier2 running", "done: tier0 green", logPath},
		},
		{
			name:      "GreenShowsItsEndTime",
			record:    CheckRecord{Subject: "alpha", State: checkGreen, EndedAt: ended, Tiers: done[:1]},
			wantState: "green",
			wantLines: []string{"adopt check of alpha: green at 2026-10-27T12:30:00Z", "done: tier0 green", logPath},
		},
		{
			name:      "FailedShowsItsEndTimeAndFailingTier",
			record:    CheckRecord{Subject: "alpha", State: checkFailed, EndedAt: ended, Tiers: done, FailedTier: &one},
			wantState: "failed",
			wantLines: []string{"adopt check of alpha: failed at 2026-10-27T12:30:00Z", "tier1 failed", logPath},
		},
		{
			name:      "RunningReadDeadShowsEndedWithoutAVerdictNamingTheSubjectsAdopt",
			record:    CheckRecord{Subject: "alpha", State: checkRunning, Running: &one, Tiers: done[:1]},
			wantState: "ended without a verdict",
			wantLines: []string{"adopt check of alpha: ended without a verdict", outPath, logPath, "re-run lym adopt alpha", "next lym adopt with an owner write", "starts a fresh check"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkView(tt.record, tt.alive, dir)

			if got.Subject != "alpha" || got.State != tt.wantState || (got.Running == nil) != (tt.wantRunning == nil) || (tt.wantRunning != nil && *got.Running != *tt.wantRunning) {
				t.Errorf("checkView() subject = %q, state = %q, running = %v; want alpha, %q and %v", got.Subject, got.State, got.Running, tt.wantState, tt.wantRunning)
			}
			if got.SuiteLog != logPath || got.CheckOut != outPath {
				t.Errorf("checkView() paths = %q, %q; want %q, %q", got.SuiteLog, got.CheckOut, logPath, outPath)
			}
			text := strings.Join(got.Lines, "\n")
			for _, want := range tt.wantLines {
				if !strings.Contains(text, want) {
					t.Errorf("checkView() lines = %q; want them to hold %q", text, want)
				}
			}
			if strings.Contains(text, "lym gate test --gate adopt") {
				t.Errorf("checkView() lines = %q; want no re-run of lym gate test --gate adopt", text)
			}
		})
	}
}

func TestReadCheckViews(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NoScratchDirectoryYieldsNoneAndCreatesNothing", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		views, err := ReadCheckViews(hub)
		if err != nil || len(views) != 0 {
			t.Errorf("ReadCheckViews() = %+v, %v; want none", views, err)
		}
		if _, err := os.Stat(fabricengine.AdoptScratchDir(hub)); !os.IsNotExist(err) {
			t.Errorf("Stat(adopt scratch directory) error = %v; want it still absent", err)
		}
	})

	t.Run("OneViewPerSubjectSortedAndATopLevelRecordIsNotRead", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		adoptDir := fabricengine.AdoptScratchDir(hub)
		for subject, record := range map[string]CheckRecord{
			"root":  {Subject: "root", State: checkGreen},
			"alpha": {Subject: "alpha", State: checkFailed},
		} {
			if err := writeCheck(filepath.Join(adoptDir, subject, checkRecordName), record); err != nil {
				t.Fatalf("write the record of %s: %v", subject, err)
			}
		}
		if err := writeCheck(filepath.Join(adoptDir, checkRecordName), CheckRecord{Subject: "old", State: checkGreen}); err != nil {
			t.Fatalf("write the top-level record: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(adoptDir, "empty"), 0o755); err != nil {
			t.Fatal(err)
		}

		views, err := ReadCheckViews(hub)
		if err != nil {
			t.Fatalf("ReadCheckViews() error = %v", err)
		}
		var subjects []string
		for _, view := range views {
			subjects = append(subjects, view.Subject)
		}
		if !slices.Equal(subjects, []string{"alpha", "root"}) {
			t.Errorf("ReadCheckViews() subjects = %v; want [alpha root], the top-level record and the empty directory skipped", subjects)
		}
	})
}
