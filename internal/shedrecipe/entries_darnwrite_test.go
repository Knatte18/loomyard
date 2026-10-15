// entries_darnwrite_test.go covers darnWriteEntry: its construction-time validation over Config, the injected DarnTable and Seats and every Env.Darn seam, plus the happy path that runs one table through the seat runner.

package shedrecipe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// darnTestEnv returns newTestEnv(t) with DarnTable and every Darn seam filled.
func darnTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.DescriptionPath = filepath.Join(dir, "summary.md")
	env.DarnTable = func(loomshed.DarnTold) (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "darn",
			Segment:    segmentcolor.Darn,
			Seats:      []seatengine.Seat{{Stencil: "darn-template-write", Outputs: []string{filepath.Join(dir, "description.md")}}},
		}, nil
	}
	env.Darn = loomshed.DarnDeps{
		ReadRejection:  func() (loomshed.PendingRejection, bool, error) { return loomshed.PendingRejection{}, false, nil },
		ClearRejection: func() error { return nil },
		Commit:         func() error { return nil },
		LatestOutcome:  func() (loomshed.DarnOutcome, bool, error) { return loomshed.DarnOutcome{}, false, nil },
		PublishFailure: func() (string, bool, error) { return "", false, nil },
	}
	return env
}

func TestDarnWriteEntry_ConstructionFailures(t *testing.T) {
	gated := gatesCfg("description", 3)
	tests := []struct {
		name  string
		field string
		cfg   Config
		edit  func(*Env)
	}{
		{"NilDarnTable", "DarnTable", gated, func(e *Env) { e.DarnTable = nil }},
		{"NilSeats", "Seats", gated, func(e *Env) { e.Seats = nil }},
		{"NilReadRejection", "Darn.ReadRejection", gated, func(e *Env) { e.Darn.ReadRejection = nil }},
		{"NilClearRejection", "Darn.ClearRejection", gated, func(e *Env) { e.Darn.ClearRejection = nil }},
		{"NilCommit", "Darn.Commit", gated, func(e *Env) { e.Darn.Commit = nil }},
		{"NilLatestOutcome", "Darn.LatestOutcome", gated, func(e *Env) { e.Darn.LatestOutcome = nil }},
		{"NilPublishFailure", "Darn.PublishFailure", gated, func(e *Env) { e.Darn.PublishFailure = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := darnTestEnv(t)
			tt.edit(&env)
			_, err := darnWriteEntry("Row", tt.cfg, env)
			if err == nil {
				t.Fatalf("darnWriteEntry() error = nil; want non-nil")
			}
			assertErrContains(t, err, tt.field)
		})
	}
}

// TestDarnWriteEntry_HappyPath asserts a fully-filled Env constructs a producer, whose one Call runs the told table through the seat runner under the row's resolved gates.
func TestDarnWriteEntry_HappyPath(t *testing.T) {
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
	producer, err := darnWriteEntry("Row", cfg, env)
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
}
