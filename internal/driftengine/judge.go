// judge.go holds the judge's seat table and the request Drift-Verify hands it;
// the `judgement` gate entry checks what the judge writes.

package driftengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// The judge's role and the stencil that opens it.
const (
	judgeRole        = "drift-judge"
	judgeStencilName = "drift-template-judge"
)

// judgeTimeout is the judge session's timeout, since drift.yaml declares no timeout key.
const judgeTimeout = 60 * time.Minute

// The judge stencil's mode text, autonomous and with the operator in the pane.
const (
	autonomousJudgeMode = "No operator is present, so make best-judgment rulings and never block on a dialog."
	operatorJudgeMode   = "The operator is present at your pane and decides each ruling: you present each cell or case, discuss it and record the operator's ruling.\n" +
		"Ask in plain text in the pane and wait for the operator's typed reply; never call the `AskUserQuestion` tool, whose modal dialog the run cannot drive."
)

// judgeRequestLockName names the lock of the judge request file.
const judgeRequestLockName = "judge-request.json"

// JudgeIdentity names the line of a judged cell, with the keys a reader of the request file expects.
type JudgeIdentity struct {
	Key  string `json:"key,omitempty"`
	N    int    `json:"n,omitempty"`
	Line int    `json:"line,omitempty"`
}

// JudgeCell is one cell the judge rules on.
type JudgeCell struct {
	// ID is the cell id the judgement must rule on.
	ID       string        `json:"id"`
	Case     string        `json:"case"`
	Identity JudgeIdentity `json:"identity"`
	Kind     CellKind      `json:"kind"`
	Old      string        `json:"old"`
	New      string        `json:"new"`
	// Why is the covering expectation entry's why.
	Why string `json:"why"`
	// OpenRange reports that the covering entry is an open range.
	OpenRange bool `json:"open_range,omitempty"`
	// File is the absolute path of the case's file: scratch directory if added, baseline directory if removed, else empty.
	File string `json:"file,omitempty"`
}

// JudgeOpenRange is an open range of the expectation that covers a judged cell.
type JudgeOpenRange struct {
	Case string `json:"case"`
	// Range is the range as the expectation spells it.
	Range string `json:"range"`
	// Why is the entry's why.
	Why string `json:"why"`
}

// JudgeRequest is what one judge spawn is handed, written before the spawn.
// Under DiffLines it holds Cells and OpenRanges, and under DiffCommand CompareLog.
type JudgeRequest struct {
	// SpawnID names the spawn; the judgement must carry it back.
	SpawnID string `json:"spawn_id"`
	// Mode is DiffLines or DiffCommand.
	Mode string `json:"mode"`
	// Cells are the cells the judge rules on, under DiffLines.
	Cells []JudgeCell `json:"cells,omitempty"`
	// OpenRanges are the open ranges covering a judged cell, under DiffLines.
	OpenRanges []JudgeOpenRange `json:"open_ranges,omitempty"`
	// CompareLog is the absolute path of the failing compare's log, under DiffCommand.
	CompareLog string `json:"compare_log,omitempty"`
	// Expectation is the sealed expectation as text.
	Expectation string `json:"expectation"`
	// Amendments are the parent's amendments as text, and empty when there is none.
	Amendments string `json:"amendments,omitempty"`
}

// WriteJudgeRequest writes the judge request to p's request file through internal/state.
func WriteJudgeRequest(p Paths, req JudgeRequest) error {
	lock, err := recordLock(p, judgeRequestLockName)
	if err != nil {
		return err
	}
	if err := state.WriteJSON(p.JudgeRequest, lock, req); err != nil {
		return fmt.Errorf("drift: write the judge request: %w", err)
	}
	return nil
}

// readJudgeRequest reads the judge request, answering found false when none was written.
func readJudgeRequest(p Paths) (JudgeRequest, bool, error) {
	lock, err := recordLock(p, judgeRequestLockName)
	if err != nil {
		return JudgeRequest{}, false, err
	}
	req, found, err := state.ReadJSON[JudgeRequest](p.JudgeRequest, lock)
	if err != nil {
		return JudgeRequest{}, false, fmt.Errorf("drift: read the judge request: %w", err)
	}
	return req, found, nil
}

// want returns what the judgement must answer in full: every cell and open range of the request.
func (r JudgeRequest) want() JudgementWant {
	want := JudgementWant{Mode: r.Mode, SpawnID: r.SpawnID, CompareFailed: r.CompareLog != ""}
	for _, cell := range r.Cells {
		want.CellIDs = append(want.CellIDs, cell.ID)
	}
	for _, open := range r.OpenRanges {
		want.OpenRanges = append(want.OpenRanges, OpenRangeNote{Case: open.Case, Range: open.Range})
	}
	return want
}

// JudgeInputs holds the values one judge spawn is composed from.
type JudgeInputs struct {
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
	// Slug is the board entry's slug; required for SubjectBoardEntry.
	Slug string
	// DecisionRecordPath is the absolute path of the task's decision record.
	DecisionRecordPath string
	// RequestPath is the absolute path of the judge request file.
	RequestPath string
	// JudgementPath is the absolute path the judge writes judgement.yaml to.
	JudgementPath string
}

// JudgeTable builds the seatengine.Table of one judge spawn: one lead on the review segment.
// The lead's only output is judgement.yaml.
// An empty required field of in is refused by name, and cfg.JudgeModel is resolved against reg.
// With cfg.JudgeInteractive the lead is interactive,
// and its prompt has it present each cell or case to the operator and record the operator's ruling.
// The judge changes nothing but its output, so the table carries no PATTERN directive,
// and it carries no gate; the producer sets it.
func JudgeTable(in JudgeInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	err := requireTold("JudgeTable", []struct{ field, value string }{
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"RequestPath", in.RequestPath},
		{"JudgementPath", in.JudgementPath},
	})
	if err != nil {
		return seatengine.Table{}, err
	}
	reading, err := subjectFields{in.Subject, in.Slug, in.DecisionRecordPath}.reading()
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("drift: JudgeTable: %w", err)
	}
	choice, err := resolveModel("JudgeTable", cfg.JudgeModel, reg)
	if err != nil {
		return seatengine.Table{}, err
	}
	mode := autonomousJudgeMode
	if cfg.JudgeInteractive {
		mode = operatorJudgeMode
	}

	return seatengine.Table{
		RolePrefix:  judgeRole,
		Segment:     segmentcolor.Review,
		Timeout:     judgeTimeout,
		Interactive: cfg.JudgeInteractive,
		Values: map[string]string{
			"mode_rules":      mode,
			"subject_reading": reading,
			"request_path":    in.RequestPath,
			"judgement_path":  in.JudgementPath,
		},
		Seats: []seatengine.Seat{{
			Stencil:     judgeStencilName,
			ModelChoice: choice,
			Outputs:     []string{in.JudgementPath},
			ConfigRole:  agentconfig.RoleDriftJudge,
		}},
	}, nil
}

// NewJudgementGate returns the gate of the `judgement` entry of the judge's table.
// It reads the judge request and parses judgement.yaml with ParseJudgement against the JudgementWant the request implies.
// A parse failure fails the gate with the parser's message, which re-prompts the judge.
// A missing file fails the gate asking for it.
func NewJudgementGate(d Deps) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		request, found, err := readJudgeRequest(d.Paths)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: the judgement gate: %w", err)
		}
		if !found {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: the judgement gate: no judge request at %s", d.Paths.JudgeRequest)
		}
		data, present, err := readRecordFile(d.Paths.Judgement)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: the judgement gate: %w", err)
		}
		if !present {
			return shuttleengine.GateResult{Findings: fmt.Sprintf("judgement.yaml is missing; write the rulings to %s and end your turn again.", d.Paths.Judgement)}, nil
		}
		if _, err := ParseJudgement(data, request.want()); err != nil {
			return shuttleengine.GateResult{Findings: err.Error() + "\nCorrect judgement.yaml and end your turn again."}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}
