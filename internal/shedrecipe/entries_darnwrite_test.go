// entries_darnwrite_test.go covers darnWriteEntry: its construction-time validation over Config, the injected DarnTable and Seats and every Env.Darn seam, plus the happy path that runs one table through the seat runner.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// darnTestEnv returns newTestEnv(t) with DarnTable and every Darn seam filled, over a decision record and a support log that exist.
func darnTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "decision-record.md")
	supportLogPath := filepath.Join(dir, "support-log.md")
	for _, path := range []string{recordPath, supportLogPath} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	env.DescriptionPath = filepath.Join(dir, "summary.md")
	env.DarnTable = func(loomshed.DarnTold) (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "darn",
			Segment:    segmentcolor.Darn,
			Seats:      []seatengine.Seat{{Stencil: "darn-template-write", Outputs: []string{filepath.Join(dir, "description.md")}}},
		}, nil
	}
	env.Darn = loomshed.DarnDeps{
		RecordPath:      recordPath,
		SupportLogPath:  supportLogPath,
		StopMarkerPath:  filepath.Join(dir, "stop-marker"),
		CommittedRecord: func() (bool, error) { return false, nil },
		Commit:          func() error { return nil },
		LatestOutcome:   func() (loomshed.DarnOutcome, bool, error) { return loomshed.DarnOutcome{}, false, nil },
		CheckRecord:     func() ([]discussionparser.Finding, error) { return nil, nil },

		ConditionStopMarkerPath: filepath.Join(dir, "condition-stop-marker"),
	}
	return env
}

func TestDarnWriteEntry_ConstructionFailures(t *testing.T) {
	tiergate.Require(t, 0)
	gated := gatesCfg("description", 3)
	tests := []struct {
		name  string
		field string
		cfg   Config
		edit  func(*Env)
	}{
		{"NilDarnTable", "DarnTable", gated, func(e *Env) { e.DarnTable = nil }},
		{"NilSeats", "Seats", gated, func(e *Env) { e.Seats = nil }},
		{"NilCommit", "Darn.Commit", gated, func(e *Env) { e.Darn.Commit = nil }},
		{"NilLatestOutcome", "Darn.LatestOutcome", gated, func(e *Env) { e.Darn.LatestOutcome = nil }},
		{"NilCheckRecord", "Darn.CheckRecord", gated, func(e *Env) { e.Darn.CheckRecord = nil }},
		{"NilCommittedRecord", "Darn.CommittedRecord", gated, func(e *Env) { e.Darn.CommittedRecord = nil }},
		{"RelativeRecordPath", "Darn.RecordPath", gated, func(e *Env) { e.Darn.RecordPath = "decision-record.md" }},
		{"RelativeSupportLogPath", "Darn.SupportLogPath", gated, func(e *Env) { e.Darn.SupportLogPath = "support-log.md" }},
		{"RelativeStopMarkerPath", "Darn.StopMarkerPath", gated, func(e *Env) { e.Darn.StopMarkerPath = "stop-marker" }},
		{"RelativeConditionStopMarkerPath", "Darn.ConditionStopMarkerPath", gated, func(e *Env) { e.Darn.ConditionStopMarkerPath = "condition-stop-marker" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := darnTestEnv(t)
			tt.edit(&env)
			_, err := darnWriteEntry(RowIdentity{Name: "Row"}, tt.cfg, env)
			if err == nil {
				t.Fatalf("darnWriteEntry() error = nil; want non-nil")
			}
			assertErrContains(t, err, tt.field)
		})
	}
}

// TestDarnWriteEntry_HappyPath asserts a fully-filled Env constructs a producer, whose one Call runs the told table through the seat runner under the row's resolved gates.
func TestDarnWriteEntry_HappyPath(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := Config{"gates": []any{
		map[string]any{"name": "verify", "attempts": 2},
		map[string]any{"name": "description", "attempts": 2},
	}}
	env := darnTestEnv(t)
	output := filepath.Join(t.TempDir(), "description.md")
	env.DarnTable = func(loomshed.DarnTold) (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "darn",
			Segment:    segmentcolor.Darn,
			Seats:      []seatengine.Seat{{Stencil: "darn-template-write", Outputs: []string{output}}},
		}, nil
	}
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
	env.Seats = fake
	producer, err := darnWriteEntry(RowIdentity{Name: "Row"}, cfg, env)
	if err != nil {
		t.Fatalf("darnWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("darnWriteEntry() = nil producer; want non-nil")
	}

	// The run's outcome depends on the writer's committed state, not on the gates under test.
	_, _, _ = producer.Call(context.Background())

	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	gate := fake.GotTables[0].Gate
	if len(gate) != 2 || gate[0].Name != "verify" || gate[1].Name != "description" || gate[0].Attempts != 2 || gate[1].Attempts != 2 {
		t.Errorf("table gate = %+v; want the verify and description entries with 2 attempts each", gate)
	}
	for _, entry := range gate {
		if entry.Reprompt == nil {
			t.Errorf("gate entry %q Reprompt = nil; want the darn line", entry.Name)
			continue
		}
		line := entry.Reprompt("/run/findings.md")
		if !strings.Contains(line, "/run/findings.md") || !strings.Contains(line, env.DecisionRecordPath) || !strings.Contains(line, "Added after Discussion") || strings.Contains(line, "\n") {
			t.Errorf("gate entry %q Reprompt() = %q; want one line naming the findings file, %s and the Added after Discussion entries", entry.Name, line, env.DecisionRecordPath)
		}
	}

	// An exhausted gate's reason names the row's findings file, so an entry that drops the chain fails here.
	withGateFindingsFile(&env, t.TempDir())
	env.Seats = &shedfake.SeatRunner{Results: []seatengine.Result{failedGateResult(output)}}
	producer, err = darnWriteEntry(RowIdentity{Name: "Row"}, cfg, env)
	if err != nil {
		t.Fatalf("darnWriteEntry() error = %v; want nil", err)
	}
	_, pointer, _ := producer.Call(context.Background())
	requireNamesFindingsFile(t, env, "Row", pointer.Reason)

	// A fixer element hands the seam the finished spec, re-prompt lines included, and names verify.
	fixerCfg := Config{"gates": []any{
		map[string]any{"name": "verify", "attempts": 2, "fixer": "darn.gate_fixer"},
		map[string]any{"name": "description", "attempts": 2},
	}}
	env.Setting = fakeSetting(map[string]any{"darn.gate_fixer": "opus"})
	calls := recordGateFixer(&env)
	if _, err := darnWriteEntry(RowIdentity{Name: "Row"}, fixerCfg, env); err != nil {
		t.Fatalf("darnWriteEntry() error = %v; want nil", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("GateFixer called %d times; want 1", len(*calls))
	}
	req := (*calls)[0]
	if req.Row != "Row" || req.Entry != "verify" || req.Model.Alias != "opus" || len(req.Spec) != 2 {
		t.Fatalf("request = %+v; want row Row, entry verify, alias opus and both gate entries", req)
	}
	for _, entry := range req.Spec {
		if entry.Reprompt == nil {
			t.Errorf("request spec entry %q Reprompt = nil; want the darn line", entry.Name)
		}
	}
}
