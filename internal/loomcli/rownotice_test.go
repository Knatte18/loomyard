// rownotice_test.go covers watchRows.
// The parent hears of each visit of a watched row once, and an unknown row is refused.
// Every test reads and writes a status file and record in its own temporary directory and spawns nothing.

package loomcli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// countingProducer is a stub producer that counts its calls and returns outcome, pointer and err.
type countingProducer struct {
	calls   *int
	outcome shedengine.Outcome
	pointer shedengine.OutputPointer
	err     error
}

func (p countingProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	*p.calls++
	return p.outcome, p.pointer, p.err
}

// rowNoticeFixture builds a two-row shed, rows Darn-Write and Drift-Verify, over a status file in a temporary directory.
// It returns the shed, the paths and a function that writes the status with the told history.
func rowNoticeFixture(t *testing.T) (*shedengine.Shed, rowNoticePaths, func(history []shedengine.HistoryEntry), *int) {
	t.Helper()
	dir := t.TempDir()
	paths := rowNoticePaths{
		status:     filepath.Join(dir, "status.json"),
		statusLock: filepath.Join(dir, "status.json.lock"),
		record:     filepath.Join(dir, "scratch", "row-notice.json"),
	}
	calls := new(int)
	shed := &shedengine.Shed{Producers: []shedengine.ProducerDef{
		{Name: "Darn-Write", Producer: countingProducer{calls: calls, outcome: shedengine.Done}},
		{Name: "Drift-Verify", Producer: countingProducer{calls: calls, outcome: shedengine.Yield}},
	}}
	writeStatus := func(history []shedengine.HistoryEntry) {
		t.Helper()
		if err := state.WriteJSON(paths.status, paths.statusLock, shedengine.Status{CurrentProducer: "Drift-Verify", State: shedengine.StateRunning, History: history}); err != nil {
			t.Fatalf("state.WriteJSON = %v; want nil error", err)
		}
	}
	return shed, paths, writeStatus, calls
}

// TestWatchRows_TellsParentOncePerVisit covers the notice count over a sequence of calls:
// a call within the same visit tells nothing more, and a new verdict or a folded repeat opens a new visit.
func TestWatchRows_TellsParentOncePerVisit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	shed, paths, writeStatus, calls := rowNoticeFixture(t)
	var lines []string
	notify := func(line string) error {
		lines = append(lines, line)
		return nil
	}
	if err := watchRows(shed, []string{"Drift-Verify"}, "darn", paths, notify); err != nil {
		t.Fatalf("watchRows = %v; want nil error", err)
	}
	darn := []shedengine.HistoryEntry{{Producer: "Darn-Write", Outcome: shedengine.Done}}
	stuck := shedengine.HistoryEntry{Producer: "Drift-Verify", Outcome: shedengine.Stuck}
	folded := stuck
	folded.Repeats = 1
	steps := []struct {
		name      string
		row       int
		history   []shedengine.HistoryEntry
		wantLines int
	}{
		{"FirstCallOfTheRow", 1, darn, 1},
		{"CallAfterAYield", 1, darn, 1},
		{"UnwatchedRow", 0, darn, 1},
		{"CallAfterANewVerdict", 1, append(darn, stuck), 2},
		{"CallAfterAFoldedRepeat", 1, append(darn, folded), 3},
	}
	for i, step := range steps {
		writeStatus(step.history)
		if _, _, err := shed.Producers[step.row].Producer.Call(context.Background()); err != nil {
			t.Fatalf("%s: Call = %v; want nil error", step.name, err)
		}
		if len(lines) != step.wantLines {
			t.Errorf("%s: notices = %d; want %d", step.name, len(lines), step.wantLines)
		}
		if *calls != i+1 {
			t.Errorf("%s: producer calls = %d; want %d", step.name, *calls, i+1)
		}
	}
	const prefix = "[row reached] the run reached Drift-Verify "
	if len(lines) == 0 {
		t.Fatal("notices = none; want at least one")
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			t.Errorf("notice = %q; want it to open %q", line, prefix)
		}
	}
}

// TestWatchRows_NoticeNeverStopsTheCall covers a refused post and an unreadable status:
// the notice goes out before the row's call, and a failed post is not retried.
// The row's own outcome, pointer and error stand.
func TestWatchRows_NoticeNeverStopsTheCall(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name          string
		statusPresent bool
		wantPosts     int
	}{
		{"RefusedPost", true, 1},
		{"MissingStatus", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, paths, writeStatus, calls := rowNoticeFixture(t)
			wantPath := "artifact.md"
			wantErr := errors.New("row failed")
			shed.Producers[0].Producer = countingProducer{
				calls: calls, outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: wantPath}, err: wantErr,
			}
			posts, callsAtPost := 0, -1
			notify := func(string) error {
				posts++
				callsAtPost = *calls
				return errors.New("refused")
			}
			if err := watchRows(shed, []string{"Darn-Write"}, "darn", paths, notify); err != nil {
				t.Fatalf("watchRows = %v; want nil error", err)
			}
			if tt.statusPresent {
				writeStatus(nil)
			}
			for attempt := 1; attempt <= 2; attempt++ {
				outcome, pointer, err := shed.Producers[0].Producer.Call(context.Background())
				if outcome != shedengine.Done || pointer.Path != wantPath || !errors.Is(err, wantErr) {
					t.Errorf("Call %d = %q, %q, %v; want %q, %q, %v", attempt, outcome, pointer.Path, err, shedengine.Done, wantPath, wantErr)
				}
				if *calls != attempt {
					t.Errorf("producer calls after Call %d = %d; want %d", attempt, *calls, attempt)
				}
			}
			if posts != tt.wantPosts {
				t.Errorf("posts over two calls = %d; want %d", posts, tt.wantPosts)
			}
			if posts > 0 && callsAtPost != 0 {
				t.Errorf("producer calls when the notice went out = %d; want 0, the notice before the call", callsAtPost)
			}
		})
	}
}

// TestNewShed_WrapsTheRowsNotifyParentOnNames covers the activation from loom.yaml's notify_parent_on:
// an empty list wraps nothing, a recipe row's producer is wrapped over the run's status and row-notice paths,
// and a name that is no row of the recipe is refused with no shed.
func TestNewShed_WrapsTheRowsNotifyParentOnNames(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		pick       func(rows []string) []string
		wantWrap   bool
		wantRefuse bool
	}{
		{"EmptyList", func([]string) []string { return nil }, false, false},
		{"RecipeRow", func(rows []string) []string { return rows[:1] }, true, false},
		{"UnknownRow", func([]string) []string { return []string{"No-Such-Row"} }, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			c := &loomCLI{runID: shedrun.SelfRunID}
			if err := c.wire(loc, loc.AnchorPath()); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}
			// The landing deps are filled at arm time, after wire(); the Publish and Finalize rows refuse to build without them.
			c.env.Landing = envkit.LandingDeps(t, t.TempDir())
			plain, err := c.newShed()
			if err != nil {
				t.Fatalf("newShed() with no notify_parent_on = %v; want nil", err)
			}
			rows := make([]string, len(plain.Producers))
			for i, def := range plain.Producers {
				rows[i] = def.Name
			}
			watched := tt.pick(rows)
			c.cfg.NotifyParentOn = watched

			shed, err := c.newShed()
			if tt.wantRefuse {
				if err == nil || shed != nil {
					t.Fatalf("newShed() = %v, %v; want a nil shed and a refusal", shed, err)
				}
				for _, want := range []string{watched[0], "way forward", rows[0]} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("newShed error = %q; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("newShed() = %v; want nil", err)
			}
			wrapped := 0
			for _, def := range shed.Producers {
				notice, isNotice := def.Producer.(rowNoticeProducer)
				if isNotice != (tt.wantWrap && def.Name == watched[0]) {
					t.Errorf("row %q wrapped = %v; want %v", def.Name, isNotice, !isNotice)
				}
				if !isNotice {
					continue
				}
				wrapped++
				if want := shedrun.RowNoticeRecord(loc, c.runID); notice.paths.record != want {
					t.Errorf("row %q record path = %q; want %q", def.Name, notice.paths.record, want)
				}
				if notice.paths.status != c.shedPaths.StatusPath || notice.paths.statusLock != c.shedPaths.StatusLockPath {
					t.Errorf("row %q status paths = %q, %q; want %q, %q",
						def.Name, notice.paths.status, notice.paths.statusLock, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
				}
			}
			if wantWrapped := len(watched); wrapped != wantWrapped {
				t.Errorf("wrapped rows = %d; want %d", wrapped, wantWrapped)
			}
		})
	}
}

// TestWatchRows_UnknownRowNamesTheRecipeRows covers a name that is no row: the refusal names it, the recipe and its rows.
func TestWatchRows_UnknownRowNamesTheRecipeRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	shed, paths, _, _ := rowNoticeFixture(t)
	err := watchRows(shed, []string{"Drift-Verify", "Drift-Judge"}, "darn", paths, func(string) error { return nil })
	if err == nil {
		t.Fatal("watchRows = nil; want a refusal")
	}
	for _, want := range []string{`"loom.notify_parent_on"`, `"Drift-Judge"`, "darn recipe", "way forward", "Darn-Write, Drift-Verify"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("watchRows error = %q; want it to contain %q", err, want)
		}
	}
}
