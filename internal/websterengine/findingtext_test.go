// findingtext_test.go pins the findings clause and the way forward of the board-model findings as golden text, and the reason a path cannot be checked.
// Untagged, with a shuttlefake.Engine: no git, no subprocess spawns, so the cases that need git (a git-ignored path, a tracked path) are covered by the integration tests of the sites.

package websterengine

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// TestPendingFindingsText pins the findings clause and way forward as golden text: the board-model
// findings take the reset route, a contract file a fork wrote last takes the delete-and-accept route,
// one Master wrote last takes the accept route, and a pathless finding takes the reset route.
func TestPendingFindingsText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// fixture builds the engine and findings over geom and returns the clause wanted, "" to skip it.
		fixture func(t *testing.T, geom Geometry) (engine shuttleengine.Engine, items []findingItem, wantClause, wantWay string)
		reentry string
	}{
		{
			name:    "board-model findings golden",
			reentry: "lyx webster run",
			fixture: func(t *testing.T, geom Geometry) (shuttleengine.Engine, []findingItem, string, string) {
				outcome := writeContractFile(t, geom)
				summary := summaryparser.Path(geom.WebsterDir)
				state := filepath.Join(geom.WebsterDir, "state.json")
				engine := writesEngine(
					[]shuttleengine.WriteEvent{succeeded(outcome, 0)},
					[]shuttleengine.WriteEvent{succeeded(outcome, time.Minute), succeeded(summary, time.Minute)},
				)
				items := []findingItem{
					{Class: "fork-contract-write", Detail: fmt.Sprintf("fork wrote %q — a contract file", outcome), Paths: []string{outcome}},
					{Class: "fork-contract-write", Detail: fmt.Sprintf("fork wrote %q — a contract file", summary), Paths: []string{summary}},
					{Class: "parent-write", Detail: fmt.Sprintf("Master wrote %q — under the run state", state), Paths: []string{state}},
				}
				clause := fmt.Sprintf("3 correctness finding(s): "+
					"1) fork-contract-write: fork wrote %q — a contract file (%s: a fork wrote it after Master's last write); "+
					"2) fork-contract-write: fork wrote %q — a contract file (%s: cleared: absent, or Master wrote it last); "+
					"3) parent-write: Master wrote %q — under the run state (%s: cannot be checked: under `%s`, outside the task worktree's tracked tree)",
					outcome, outcome, summary, summary, state, state, lyxdirs.LyxDirName)
				return engine, items, clause, "way forward: 1) lyx webster reset --to start; 2) lyx webster run --fresh"
			},
		},
		{
			name:    "a contract file a fork wrote last ends in one re-entry step after its delete",
			reentry: "re-step the loom row",
			fixture: func(t *testing.T, geom Geometry) (shuttleengine.Engine, []findingItem, string, string) {
				outcome := writeContractFile(t, geom)
				engine := writesEngine(nil, []shuttleengine.WriteEvent{succeeded(outcome, time.Minute)})
				items := []findingItem{{Class: "fork-contract-write", Detail: fmt.Sprintf("fork wrote %q", outcome), Paths: []string{outcome}}}
				return engine, items, "", fmt.Sprintf("way forward: 1) rm %s; 2) lyx webster accept-audit; 3) re-step the loom row", outcome)
			},
		},
		{
			name:    "a contract file Master wrote last needs no delete",
			reentry: "lyx webster run",
			fixture: func(t *testing.T, geom Geometry) (shuttleengine.Engine, []findingItem, string, string) {
				outcome := writeContractFile(t, geom)
				engine := writesEngine([]shuttleengine.WriteEvent{succeeded(outcome, 2*time.Minute)}, []shuttleengine.WriteEvent{succeeded(outcome, time.Minute)})
				items := []findingItem{{Class: "fork-contract-write", Detail: fmt.Sprintf("fork wrote %q", outcome), Paths: []string{outcome}}}
				return engine, items, "", "way forward: 1) lyx webster accept-audit; 2) lyx webster run"
			},
		},
		{
			name:    "a pathless finding takes the reset route",
			reentry: "lyx webster run",
			fixture: func(t *testing.T, geom Geometry) (shuttleengine.Engine, []findingItem, string, string) {
				items := []findingItem{{Class: "fabric-reference", Detail: "ran lyx fabric"}}
				return nil, items,
					"1 correctness finding(s): 1) fabric-reference: ran lyx fabric (the finding names no path)",
					"way forward: 1) lyx webster reset --to start; 2) lyx webster run --fresh"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			geom := evidenceGeom(t)
			engine, items, wantClause, wantWay := tt.fixture(t, geom)
			clause, wayForward, err := pendingFindingsText(engine, &State{MasterSessionID: "s1"}, geom, items, tt.reentry)
			if err != nil {
				t.Fatalf("pendingFindingsText: %v", err)
			}
			if wantClause != "" && clause != wantClause {
				t.Errorf("clause =\n%s\nwant\n%s", clause, wantClause)
			}
			if wayForward != wantWay {
				t.Errorf("way forward = %q, want %q", wayForward, wantWay)
			}
		})
	}
}

// TestFindingsClause pins the path parenthetical a finding with no note gets: its path when the
// detail lacks it, and nothing when the detail already names it.
//
//testtiming:keep pins the bare-path suffix and its omission when the detail names the path; every pending-findings case carries a note per path
func TestFindingsClause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail string
		want   string
	}{
		{name: "the detail lacks the path", detail: "Master wrote a file", want: "1 correctness finding(s): 1) parent-write: Master wrote a file (a.go)"},
		{name: "the detail names the path", detail: "Master wrote a.go", want: "1 correctness finding(s): 1) parent-write: Master wrote a.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := findingsClause([]findingItem{{Class: "parent-write", Detail: tt.detail, Paths: []string{"a.go"}}}, nil)
			if got != tt.want {
				t.Errorf("clause = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUncheckableReason(t *testing.T) {
	geom := evidenceGeom(t)
	outside := filepath.Join(t.TempDir(), "elsewhere", "f.txt")
	tests := []struct {
		name        string
		st          *State
		path        string
		wantReason  string
		wantUncheck bool
	}{
		{name: "no path", st: &State{}, path: "", wantReason: "the finding names no path", wantUncheck: true},
		{name: "plan file with no recorded plan", st: &State{}, path: filepath.Join(geom.PlanDir, "01-a.md"), wantReason: "no plan copy recorded", wantUncheck: true},
		{name: "plan file with a recorded plan", st: &State{PlanFileHashes: map[string]string{"01-a.md": "x"}}, path: filepath.Join(geom.PlanDir, "01-a.md")},
		{name: "outside the worktree", st: &State{}, path: outside, wantReason: "outside the task worktree", wantUncheck: true},
		{
			name:        "under the lyx directory",
			st:          &State{},
			path:        filepath.Join(geom.WorktreeRoot, lyxdirs.LyxDirName, "board", "x.json"),
			wantReason:  fmt.Sprintf("under `%s`, outside the task worktree's tracked tree", lyxdirs.LyxDirName),
			wantUncheck: true,
		},
		{name: "under webster's scratch directory", st: &State{}, path: filepath.Join(geom.ScratchDir, "pause"), wantReason: "under webster's scratch directory", wantUncheck: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, uncheckable, err := uncheckableReason(geom, tt.st, tt.path)
			if err != nil {
				t.Fatalf("uncheckableReason: %v", err)
			}
			if reason != tt.wantReason || uncheckable != tt.wantUncheck {
				t.Errorf("uncheckableReason = (%q, %v), want (%q, %v)", reason, uncheckable, tt.wantReason, tt.wantUncheck)
			}
		})
	}
}
