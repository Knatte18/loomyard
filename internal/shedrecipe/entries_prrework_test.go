// entries_prrework_test.go covers prReworkEntry: its construction-time validation over Config, the injected ReworkTable and Seats, every Env.Rework seam and its two told directories,
// and the Calls proving the told value reaches the table source and the row's gate reaches the seat runner.

package shedrecipe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// reworkTestEnv returns newTestEnv(t) with ReworkTable and every Rework seam filled.
func reworkTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.ReworkTable = func(loomshed.ReworkTold) (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "rework",
			Segment:    segmentcolor.Webster,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-rework", Outputs: []string{filepath.Join(dir, "coverage.md")}}},
		}, nil
	}
	env.Rework = loomshed.PRReworkDeps{
		PlanDir:          filepath.Join(dir, "plan"),
		ReworkDir:        filepath.Join(dir, "rework"),
		ReworkDirRel:     "_lyx/loom/rework",
		ReviewsDir:       filepath.Join(dir, "reviews"),
		ReviewRunSubdirs: []string{"plan", "webster"},
		ReadCommitted:    func(string) ([]byte, bool, error) { return nil, false, nil },
		ReadRejection:    func() (loomshed.PendingRejection, bool, error) { return loomshed.PendingRejection{}, false, nil },
		ClearRejection:   func() error { return nil },
		ArchiveWebster:   func(string) error { return nil },
		Commit:           func() error { return nil },
	}
	return env
}

// withPendingRework returns env after planting a committed one-card plan and a pending rejection, so the producer computes FirstCard = 2 and starts a session.
func withPendingRework(t *testing.T, env Env) Env {
	t.Helper()
	planDir := env.Rework.PlanDir
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := plankit.Render(plankit.Plan{
		Approved: true,
		Language: "none",
		Framing:  "Framing.",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "first-card",
			Summary: "placeholder",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/x/new.go"}}},
			Intent:  "placeholder.",
		}},
	})
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env.Rework.ReadCommitted = func(rel string) ([]byte, bool, error) {
		body, ok := files[filepath.Base(rel)]
		return body, ok, nil
	}
	env.Rework.ReadRejection = func() (loomshed.PendingRejection, bool, error) {
		return loomshed.PendingRejection{PRNumber: 1, HeadSHA: "h", RejectedAt: "t"}, true, nil
	}
	return env
}

func TestPRReworkEntry_HappyPath(t *testing.T) {
	producer, err := prReworkEntry("Row", Config{}, reworkTestEnv(t))
	if err != nil {
		t.Fatalf("prReworkEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("prReworkEntry() = nil producer; want non-nil")
	}
}

func TestPRReworkEntry_EvaluatesReworkTableWithToldValue(t *testing.T) {
	var got []loomshed.ReworkTold
	env := withPendingRework(t, reworkTestEnv(t))
	env.ReworkTable = func(told loomshed.ReworkTold) (seatengine.Table, error) {
		got = append(got, told)
		return seatengine.Table{}, errors.New("stop here")
	}
	producer, err := prReworkEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("prReworkEntry() error = %v; want nil", err)
	}
	// The session's table failure surfaces as an error or a Stuck; either way the table source ran.
	_, _, _ = producer.Call(context.Background())
	if len(got) == 0 || got[0].FirstCard != 2 {
		t.Errorf("ReworkTable told %+v; want first call FirstCard 2", got)
	}
}

func TestPRReworkEntry_ConstructionFailures(t *testing.T) {
	tests := []struct {
		name  string
		field string
		edit  func(*Env)
	}{
		{"NilReworkTable", "ReworkTable", func(e *Env) { e.ReworkTable = nil }},
		{"NilSeats", "Seats", func(e *Env) { e.Seats = nil }},
		{"NilReadCommitted", "Rework.ReadCommitted", func(e *Env) { e.Rework.ReadCommitted = nil }},
		{"NilReadRejection", "Rework.ReadRejection", func(e *Env) { e.Rework.ReadRejection = nil }},
		{"NilClearRejection", "Rework.ClearRejection", func(e *Env) { e.Rework.ClearRejection = nil }},
		{"NilCommit", "Rework.Commit", func(e *Env) { e.Rework.Commit = nil }},
		{"NilArchiveWebster", "Rework.ArchiveWebster", func(e *Env) { e.Rework.ArchiveWebster = nil }},
		{"EmptyReviewsDir", "Rework.ReviewsDir", func(e *Env) { e.Rework.ReviewsDir = "" }},
		{"EmptyPlanDir", "Rework.PlanDir", func(e *Env) { e.Rework.PlanDir = "" }},
		{"EmptyReworkDir", "Rework.ReworkDir", func(e *Env) { e.Rework.ReworkDir = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := reworkTestEnv(t)
			tt.edit(&env)
			_, err := prReworkEntry("Row", Config{}, env)
			if err == nil {
				t.Fatalf("prReworkEntry() error = nil; want non-nil")
			}
			assertErrContains(t, err, "PRRework")
			assertErrContains(t, err, tt.field)
		})
	}
}

func TestPRReworkEntry_Config(t *testing.T) {
	t.Run("ReworkPlanGate", func(t *testing.T) {
		gateSpec, err := resolveGateSpec("PRRework", gatesCfg("rework-plan", 3), reworkTestEnv(t))
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v; want nil", err)
		}
		if len(gateSpec) != 1 || gateSpec[0].Gate == nil || gateSpec[0].Attempts != 3 {
			t.Errorf("resolveGateSpec() = %+v; want the rework plan closure with 3 attempts", gateSpec)
		}
	})

	t.Run("ReworkPlanGateWithoutReadCommitted", func(t *testing.T) {
		env := reworkTestEnv(t)
		env.Rework.ReadCommitted = nil
		_, err := resolveGateSpec("PRRework", gatesCfg("rework-plan", 3), env)
		if err == nil {
			t.Fatalf("resolveGateSpec() error = nil; want non-nil for a rework-plan gate with no ReadCommitted seam")
		}
		assertErrContains(t, err, "Rework.ReadCommitted")
	})

	t.Run("TheRowsGateReachesTheSeatRunner", func(t *testing.T) {
		env := withPendingRework(t, reworkTestEnv(t))
		fake := &shedfake.SeatRunner{}
		env.Seats = fake
		producer, err := prReworkEntry("Row", gatesCfg("rework-plan", 3), env)
		if err != nil {
			t.Fatalf("prReworkEntry() error = %v; want nil", err)
		}
		// The run's outcome depends on the planted plan, not on the gate under test.
		_, _, _ = producer.Call(context.Background())

		if len(fake.GotTables) != 1 {
			t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
		}
		gate := fake.GotTables[0].Gate
		if len(gate) != 1 || gate[0].Name != "rework-plan" || gate[0].Attempts != 3 {
			t.Errorf("table gate = %+v; want one \"rework-plan\" entry with 3 attempts", gate)
		}
	})
}
