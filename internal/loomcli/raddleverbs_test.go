// raddleverbs_test.go exercises the raddle-propose and raddle-approve verbs over injected seams and a proposal, record and state in a temp directory.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// raddleTestRow is the name the injected row seam answers for the recipe's RaddleWrite row.
	raddleTestRow = "Raddle-Write"
	// raddleTestRecord is a decision record of one decision with one criterion, which the one-part tapestry proposal below builds.
	raddleTestRecord = "## Decisions\n\n### D1 — First\n\nThe first decision.\n\n## Acceptance criteria\n\n- D1: first holds.\n"
)

// raddleTestProposal returns the bytes of a tapestry proposal that passes every check against raddleTestRecord and raddleTestBoard.
// checked lists the board entries the proposal weighed, each with the revised stamp it read, and costLine changes the file without changing its validity.
func raddleTestProposal(t *testing.T, costLine string, checked ...raddleparser.BoardCheck) []byte {
	t.Helper()
	data, err := json.Marshal(raddleparser.Proposal{
		Sketch:       "the-sketch",
		Shape:        raddleparser.ShapeTapestry,
		BoardChecked: checked,
		Parts: []raddleparser.Part{{
			Slug: "part-a", Title: "Part A", Type: "enhancement", Recipe: raddleparser.RecipeTapestry, Profile: "standard",
			Decisions: []string{"D1"}, Packages: []raddleparser.Package{{Path: "internal/a", Size: 10}},
			Criteria: []string{"D1: first holds."}, CostLine: "one tapestry part",
		}},
		Cost: raddleparser.Cost{Line: costLine, NextBest: raddleparser.NextBest{Split: "two parts", Line: "two tapestry parts"}},
	})
	if err != nil {
		t.Fatalf("marshal the proposal: %v", err)
	}
	return data
}

// raddleTestBoard returns the board context raddleTestProposal passes under: the Sketch is the run's own entry and "other" is one open entry.
func raddleTestBoard() raddleparser.BoardContext {
	return raddleparser.BoardContext{
		Sketch: "the-sketch",
		Entry:  "the-sketch",
		Entries: []raddleparser.BoardEntry{
			{Slug: "the-sketch", Kind: raddleparser.KindTask, Labels: []string{"sketch"}, Open: true},
			{Slug: "other", Kind: raddleparser.KindTask, Labels: []string{"bug"}, Open: true, Revised: "2026-11-01T10:00:00Z"},
		},
		TypeLabels: []string{"bug", "enhancement", "sketch"},
		Profiles:   map[string][]string{raddleparser.RecipeTapestry: {"standard"}},
	}
}

// raddleVerbFixture is a raddle run's files in a temp directory with recording seams.
type raddleVerbFixture struct {
	deps      raddleVerbDeps
	hasRow    bool
	current   string
	board     raddleparser.BoardContext
	notices   []string
	notifyErr error
}

// newRaddleVerbFixture writes proposal and the decision record into a temp directory and returns the fixture whose run stands at the Raddle-Write row.
func newRaddleVerbFixture(t *testing.T, proposal []byte) *raddleVerbFixture {
	t.Helper()
	dir := t.TempDir()
	f := &raddleVerbFixture{hasRow: true, current: raddleTestRow, board: raddleTestBoard()}
	f.deps = raddleVerbDeps{
		row:        func() (string, bool, error) { return raddleTestRow, f.hasRow, nil },
		currentRow: func() (string, error) { return f.current, nil },
		check: loomshed.RaddleCheck{
			ProposalPath: filepath.Join(dir, "proposal.yaml"),
			RecordPath:   filepath.Join(dir, "decision.md"),
			Board:        func(bool) (raddleparser.BoardContext, error) { return f.board, nil },
		},
		statePath:     filepath.Join(dir, "state.json"),
		stateLockPath: filepath.Join(dir, "state.json.lock"),
		coldPath:      filepath.Join(dir, "cold-proposal.yaml"),
		approvalPath:  filepath.Join(dir, "approval.json"),
		sketch:        "the-sketch",
		notify: func(line string) error {
			if f.notifyErr != nil {
				return f.notifyErr
			}
			f.notices = append(f.notices, line)
			return nil
		},
	}
	f.writeProposal(t, proposal)
	if err := os.WriteFile(f.deps.check.RecordPath, []byte(raddleTestRecord), 0o644); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	return f
}

func (f *raddleVerbFixture) writeProposal(t *testing.T, proposal []byte) {
	t.Helper()
	if err := os.WriteFile(f.deps.check.ProposalPath, proposal, 0o644); err != nil {
		t.Fatalf("write the proposal: %v", err)
	}
}

// updateState applies mutate to the run's state record.
func (f *raddleVerbFixture) updateState(t *testing.T, mutate func(*loomshed.RaddleState)) {
	t.Helper()
	if _, err := loomshed.UpdateRaddleState(f.deps.statePath, f.deps.stateLockPath, mutate); err != nil {
		t.Fatalf("UpdateRaddleState() = %v; want nil", err)
	}
}

// readState returns the run's state record.
func (f *raddleVerbFixture) readState(t *testing.T) loomshed.RaddleState {
	t.Helper()
	got, err := loomshed.ReadRaddleState(f.deps.statePath, f.deps.stateLockPath)
	if err != nil {
		t.Fatalf("ReadRaddleState() = %v; want nil", err)
	}
	return got
}

// run calls verb over the fixture's deps and returns its exit code and output.
func (f *raddleVerbFixture) run(verb func(io.Writer, raddleVerbDeps) int) (int, string) {
	var out bytes.Buffer
	code := verb(&out, f.deps)
	return code, out.String()
}

func TestRaddleProposeVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	refusals := []struct {
		name       string
		hasRow     bool
		current    string
		proposal   []byte
		wantOutput []string
	}{
		{name: "ARecipeWithNoRaddleWriteRowIsRefused", hasRow: false, current: "Describe", proposal: raddleTestProposal(t, "one tapestry part"),
			wantOutput: []string{"has no Raddle-Write row", "way forward: none is needed"}},
		{name: "ARunAtAnotherRowIsRefused", hasRow: true, current: "Raddle-File", proposal: raddleTestProposal(t, "one tapestry part"),
			wantOutput: []string{"current row is Raddle-File", "way forward: run lym loom raddle-propose while the run stands at Raddle-Write"}},
		{name: "FindingsArePrintedAndNothingIsStored", hasRow: true, current: raddleTestRow, proposal: []byte("not json"),
			wantOutput: []string{"proposal-unparsable", "way forward: fix each finding"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRaddleVerbFixture(t, tt.proposal)
			f.hasRow, f.current = tt.hasRow, tt.current
			f.updateState(t, func(stored *loomshed.RaddleState) { stored.Session = 1 })

			code, out := f.run(raddleProposeVerb)
			if code != 1 {
				t.Errorf("raddleProposeVerb() = %d, %s; want exit 1", code, out)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out, want) {
					t.Errorf("raddleProposeVerb() output = %s; want it to contain %q", out, want)
				}
			}
			if got := f.readState(t); got.ColdStored || got.ProposedSession != 0 || got.NoticeSession != 0 {
				t.Errorf("state after the refusal = %+v; want it unchanged", got)
			}
			if _, err := os.Stat(f.deps.coldPath); !os.IsNotExist(err) {
				t.Errorf("cold proposal stat = %v; want not-exist", err)
			}
			if len(f.notices) != 0 {
				t.Errorf("notices = %q; want none", f.notices)
			}
		})
	}

	t.Run("PassesStoreTheColdProposalOnceAndNotifyOncePerSession", func(t *testing.T) {
		t.Parallel()
		first := raddleTestProposal(t, "one tapestry part")
		f := newRaddleVerbFixture(t, first)
		f.updateState(t, func(stored *loomshed.RaddleState) { stored.Session = 1 })

		if code, out := f.run(raddleProposeVerb); code != 0 {
			t.Fatalf("first raddleProposeVerb() = %d, %s; want exit 0", code, out)
		}
		if cold, err := os.ReadFile(f.deps.coldPath); err != nil || !bytes.Equal(cold, first) {
			t.Errorf("cold proposal = %q, %v; want the first proposal", cold, err)
		}
		if got := f.readState(t); !got.ColdStored || got.ProposedSession != 1 || got.NoticeSession != 1 || len(f.notices) != 1 {
			t.Errorf("state after the first pass = %+v with %d notices; want cold stored, proposed 1, notice 1 and one notice", got, len(f.notices))
		}

		revised := raddleTestProposal(t, "one tapestry part, revised")
		f.writeProposal(t, revised)
		if code, out := f.run(raddleProposeVerb); code != 0 {
			t.Fatalf("second raddleProposeVerb() = %d, %s; want exit 0", code, out)
		}
		if len(f.notices) != 1 {
			t.Errorf("notices after a second pass of the session = %d; want 1", len(f.notices))
		}

		f.updateState(t, func(stored *loomshed.RaddleState) { stored.Session = 2 })
		if code, out := f.run(raddleProposeVerb); code != 0 {
			t.Fatalf("later-session raddleProposeVerb() = %d, %s; want exit 0", code, out)
		}
		if got := f.readState(t); got.ProposedSession != 2 || got.NoticeSession != 2 || len(f.notices) != 2 {
			t.Errorf("state after the later session's pass = %+v with %d notices; want proposed 2, notice 2 and two notices", got, len(f.notices))
		}
		if cold, err := os.ReadFile(f.deps.coldPath); err != nil || !bytes.Equal(cold, first) {
			t.Errorf("cold proposal after later passes = %q, %v; want the first proposal kept", cold, err)
		}
	})

	t.Run("AFailedNoticeIsPostedOnTheNextPass", func(t *testing.T) {
		t.Parallel()
		f := newRaddleVerbFixture(t, raddleTestProposal(t, "one tapestry part"))
		f.updateState(t, func(stored *loomshed.RaddleState) { stored.Session = 1 })
		f.notifyErr = errors.New("creel unavailable")

		code, out := f.run(raddleProposeVerb)
		if code != 1 || !strings.Contains(out, "creel unavailable") || !strings.Contains(out, "run lym loom raddle-propose again") {
			t.Fatalf("raddleProposeVerb() = %d, %s; want exit 1 naming the fault and a re-run", code, out)
		}
		if got := f.readState(t); got.ProposedSession != 1 || got.NoticeSession != 0 {
			t.Errorf("state after the failed notice = %+v; want proposed 1 and no notice recorded", got)
		}

		f.notifyErr = nil
		if code, out := f.run(raddleProposeVerb); code != 0 || len(f.notices) != 1 {
			t.Errorf("raddleProposeVerb() = %d, %s with %d notices; want exit 0 and one notice", code, out, len(f.notices))
		}
	})
}

func TestRaddleApproveVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	moved := raddleparser.BoardCheck{Slug: "other", Revised: "2026-11-01T10:00:00Z"}
	gone := raddleparser.BoardCheck{Slug: "gone", Revised: "2026-11-01T08:00:00Z"}
	tests := []struct {
		name         string
		current      string
		proposal     []byte
		state        loomshed.RaddleState
		boardMoved   bool
		wantCode     int
		wantOutput   []string
		wantApproved bool
	}{
		{name: "OutsideRaddleWriteIsRefused", current: "Raddle-File", proposal: raddleTestProposal(t, "line"),
			state: loomshed.RaddleState{Session: 1, ProposedSession: 1, NoticeSession: 1}, wantCode: 1,
			wantOutput: []string{"current row is Raddle-File", "way forward: run lym loom raddle-approve while the run stands at Raddle-Write"}},
		{name: "NoPassingProposeThisSessionIsRefusedNamingPropose", current: raddleTestRow, proposal: raddleTestProposal(t, "line"),
			state: loomshed.RaddleState{Session: 1}, wantCode: 1,
			wantOutput: []string{"no proposal has passed", "lym loom raddle-propose"}},
		{name: "ProposeInAnEarlierSessionIsRefusedNamingPropose", current: raddleTestRow, proposal: raddleTestProposal(t, "line"),
			state: loomshed.RaddleState{Session: 2, ProposedSession: 1, NoticeSession: 1}, wantCode: 1,
			wantOutput: []string{"session 2", "lym loom raddle-propose"}},
		{name: "ProposeWhoseNoticeWasNotPostedIsRefusedNamingPropose", current: raddleTestRow, proposal: raddleTestProposal(t, "line"),
			state: loomshed.RaddleState{Session: 1, ProposedSession: 1}, wantCode: 1,
			wantOutput: []string{"waiting notice", "was not posted", "run lym loom raddle-propose again"}},
		{name: "AnInvalidProposalIsRefusedWithItsFindings", current: raddleTestRow, proposal: []byte("not json"),
			state: loomshed.RaddleState{Session: 1, ProposedSession: 1, NoticeSession: 1}, wantCode: 1,
			wantOutput: []string{"proposal-unparsable", "the proposal is not valid"}},
		{name: "AMovedBoardIsRefusedNamingTheEntry", current: raddleTestRow, proposal: raddleTestProposal(t, "line", moved),
			state: loomshed.RaddleState{Session: 1, ProposedSession: 1, NoticeSession: 1}, boardMoved: true, wantCode: 1,
			wantOutput: []string{"the board moved", raddleparser.CheckRevisedMoved, `\"other\"`}},
		{name: "SuccessRecordsTheApprovalAndNamesLandedEntries", current: raddleTestRow, proposal: raddleTestProposal(t, "line", moved, gone),
			state: loomshed.RaddleState{Session: 1, ProposedSession: 1, NoticeSession: 1}, wantCode: 0,
			wantOutput: []string{`"approved":true`, `"landed":["gone"]`}, wantApproved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRaddleVerbFixture(t, tt.proposal)
			f.current = tt.current
			f.updateState(t, func(stored *loomshed.RaddleState) { *stored = tt.state })
			if tt.boardMoved {
				f.board.Entries[1].Revised = "2026-11-02T10:00:00Z"
			}

			code, out := f.run(raddleApproveVerb)
			if code != tt.wantCode {
				t.Errorf("raddleApproveVerb() = %d, %s; want exit %d", code, out, tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out, want) {
					t.Errorf("raddleApproveVerb() output = %s; want it to contain %q", out, want)
				}
			}

			record, recordErr := os.ReadFile(f.deps.approvalPath)
			got := f.readState(t)
			if !tt.wantApproved {
				if !os.IsNotExist(recordErr) || got.Approval != nil {
					t.Errorf("approval record = %q, %v with state approval %+v; want neither", record, recordErr, got.Approval)
				}
				return
			}
			want := loomshed.RaddleApprovalRecord{Hash: raddleparser.Hash(tt.proposal), Session: tt.state.Session}
			var written loomshed.RaddleApprovalRecord
			if recordErr != nil || json.Unmarshal(record, &written) != nil || written != want {
				t.Errorf("approval record = %q, %v; want %+v", record, recordErr, want)
			}
			if got.Approval == nil || !reflect.DeepEqual(*got.Approval, want) {
				t.Errorf("state approval = %+v; want %+v", got.Approval, want)
			}
		})
	}
}
