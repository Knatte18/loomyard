//go:build tier0

// entries_parts_test.go covers partsWriteEntry and partsPlanWriteEntry: their construction-time validation over Config and the Env fields they read,
// and the Calls proving each commits through its own seam, keeps a failed gate's findings, and hands the table source the hooks it builds.

package shedrecipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// partsMaxSetting answers max for tapestry.parts_max and an error for any other setting.
func partsMaxSetting(max int) func(module, key string) (any, error) {
	return func(module, key string) (any, error) {
		if module == "tapestry" && key == "parts_max" {
			return max, nil
		}
		return nil, fmt.Errorf("unexpected setting %s.%s", module, key)
	}
}

// partsWriteTestEnv returns a newTestEnv with the PartsWrite entry's seams filled, its table writing the parts file under the anchor.
func partsWriteTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	output := partsparser.File(env.AnchorPath)
	env.PartsTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "parts",
			Segment:    segmentcolor.Plan,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-parts", Outputs: []string{output}}},
		}, nil
	}
	env.CommitParts = func() error { return nil }
	return env
}

// partsPlanWriteTestEnv returns a newTestEnv with the PartsPlanWrite entry's seams filled, its table a chair writing the join record and one writer seat for the part "whole".
// The returned slice records every PartSeats the table source was handed.
func partsPlanWriteTestEnv(t *testing.T) (Env, *[]PartSeats) {
	t.Helper()
	env := newTestEnv(t)
	handed := new([]PartSeats)
	env.PartsPlanTable = func(seats PartSeats) (seatengine.Table, error) {
		*handed = append(*handed, seats)
		return seatengine.Table{
			RolePrefix: "parts-plan",
			Segment:    segmentcolor.Plan,
			Seats: []seatengine.Seat{
				{Stencil: "loom-template-parts-chair", Outputs: []string{partsparser.JoinRecord(env.AnchorPath)}},
				{Name: "whole", Stencil: "loom-template-part-writer", Outputs: []string{partsparser.PartOverview(env.AnchorPath, "whole")}, Writer: true},
			},
		}, nil
	}
	env.CommitPartsPlan = func() error { return nil }
	return env, handed
}

// partsPlanGatesCfg is a PartsPlanWrite row Config guarding the chair with the plan gate and each writer with the plan-part gate.
func partsPlanGatesCfg() Config {
	return Config{
		"gates":      []any{map[string]any{"name": "plan", "attempts": 2}},
		"part_gates": []any{map[string]any{"name": "plan-part", "attempts": 3}},
	}
}

func TestPartsWriteEntry(t *testing.T) {
	t.Parallel()

	t.Run("ConstructionFailures", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			mutate func(*Env)
			cfg    Config
			want   string
		}{
			{"NilPartsTable", func(e *Env) { e.PartsTable = nil }, Config{}, "PartsTable"},
			{"NilCommitParts", func(e *Env) { e.CommitParts = nil }, Config{}, "CommitParts"},
			{"NilSeats", func(e *Env) { e.Seats = nil }, Config{}, "Seats"},
			{"EmptyAnchorPath", func(e *Env) { e.AnchorPath = "" }, Config{}, "AnchorPath"},
			{"UnknownKey", func(*Env) {}, Config{"bogus": true}, "bogus"},
			{"PartGatesKey", func(*Env) {}, Config{"part_gates": []any{}}, "part_gates"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				env := partsWriteTestEnv(t)
				tt.mutate(&env)
				_, err := partsWriteEntry(RowIdentity{Name: "Row"}, tt.cfg, env)
				assertErrContains(t, err, tt.want)
			})
		}
	})

	t.Run("DoneCommitsOnceAndStampsTheGate", func(t *testing.T) {
		t.Parallel()
		env := partsWriteTestEnv(t)
		env.Setting = partsMaxSetting(6)
		output := partsparser.File(env.AnchorPath)
		commits := 0
		env.CommitParts = func() error { commits++; return nil }
		fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
		env.Seats = fake

		producer, err := partsWriteEntry(RowIdentity{Name: "Row"}, gatesCfg("parts", 2), env)
		if err != nil {
			t.Fatalf("partsWriteEntry() error = %v; want nil", err)
		}
		pointer := shedfake.RequireOutcome(t, producer, shedengine.Done)

		if pointer.Path != output {
			t.Errorf("Call() OutputPointer.Path = %q; want the lead's output %q", pointer.Path, output)
		}
		if commits != 1 {
			t.Errorf("CommitParts invoked %d times; want 1", commits)
		}
		if len(fake.GotTables) != 1 {
			t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
		}
		if gate := fake.GotTables[0].Gate; len(gate) != 1 || gate[0].Name != "parts" || gate[0].Attempts != 2 {
			t.Errorf("table gate = %+v; want one \"parts\" entry with 2 attempts", gate)
		}
	})

	t.Run("GateFailedStuckCommitsAndKeepsTheFindings", func(t *testing.T) {
		t.Parallel()
		env := partsWriteTestEnv(t)
		output := partsparser.File(env.AnchorPath)
		withGateFindingsFile(&env, t.TempDir())
		commits := 0
		env.CommitParts = func() error { commits++; return nil }
		env.Seats = &shedfake.SeatRunner{Results: []seatengine.Result{failedGateResult(output)}}

		producer, err := partsWriteEntry(RowIdentity{Name: "Row"}, Config{}, env)
		if err != nil {
			t.Fatalf("partsWriteEntry() error = %v; want nil", err)
		}
		pointer := shedfake.RequireOutcome(t, producer, shedengine.Stuck)

		requireNamesFindingsFile(t, env, "Row", pointer.Reason)
		kept, err := os.ReadFile(env.GateFindingsFile("Row"))
		if err != nil || !strings.Contains(string(kept), "the gate's findings") {
			t.Errorf("findings file = %q, %v; want it to hold the gate's findings", kept, err)
		}
		if commits != 1 {
			t.Errorf("CommitParts invoked %d times; want 1", commits)
		}
	})

	t.Run("FreshSpawnArchivesTheStalePartsBeforeTheRun", func(t *testing.T) {
		t.Parallel()
		env := partsWriteTestEnv(t)
		stale := partsparser.File(env.AnchorPath)
		if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
		staleAtRun := true
		env.Seats = &shedfake.SeatRunner{RunFn: func(seatengine.Table) (seatengine.Result, error) {
			_, err := os.Stat(stale)
			staleAtRun = err == nil
			return leadDoneResult(stale), nil
		}}

		producer, err := partsWriteEntry(RowIdentity{Name: "Row"}, Config{}, env)
		if err != nil {
			t.Fatalf("partsWriteEntry() error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if staleAtRun {
			t.Errorf("the stale parts file was still in place when the seat ran; want it archived first")
		}
		archives, err := filepath.Glob(filepath.Join(partsparser.Dir(env.AnchorPath), partsparser.ArchivePrefix+"*"))
		if err != nil || len(archives) != 1 {
			t.Errorf("archive directories = %v, %v; want exactly one", archives, err)
		}
	})
}

func TestPartsPlanWriteEntry(t *testing.T) {
	t.Parallel()

	t.Run("ConstructionFailures", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			mutate func(*Env)
			cfg    Config
			want   string
		}{
			{"NilPartsPlanTable", func(e *Env) { e.PartsPlanTable = nil }, Config{}, "PartsPlanTable"},
			{"NilCommitPartsPlan", func(e *Env) { e.CommitPartsPlan = nil }, Config{}, "CommitPartsPlan"},
			{"NilSeats", func(e *Env) { e.Seats = nil }, Config{}, "Seats"},
			{"NilPlanIndex", func(e *Env) { e.PlanIndex = nil }, Config{}, "PlanIndex"},
			{"NilArchiveWebster", func(e *Env) { e.Rework.ArchiveWebster = nil }, Config{}, "Rework.ArchiveWebster"},
			{"EmptyAnchorPath", func(e *Env) { e.AnchorPath = "" }, Config{}, "AnchorPath"},
			{"EmptyWorktreeRoot", func(e *Env) { e.WorktreeRoot = "" }, Config{}, "WorktreeRoot"},
			{"EmptyStencilsDir", func(e *Env) { e.StencilsDir = "" }, Config{}, "StencilsDir"},
			{"UnknownKey", func(*Env) {}, Config{"bogus": true}, "bogus"},
			{"PartGateOfAnotherName", func(*Env) {}, Config{"part_gates": []any{map[string]any{"name": "plan", "attempts": 1}}}, "plan-part"},
			{"EmptyPartGates", func(*Env) {}, Config{"part_gates": []any{}}, "part_gates"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				env, _ := partsPlanWriteTestEnv(t)
				tt.mutate(&env)
				_, err := partsPlanWriteEntry(RowIdentity{Name: "Row"}, tt.cfg, env)
				assertErrContains(t, err, tt.want)
			})
		}
	})

	t.Run("DoneCommitsOnceAndHandsTheTableSourceItsHooks", func(t *testing.T) {
		t.Parallel()
		env, handed := partsPlanWriteTestEnv(t)
		withGateFindingsFile(&env, t.TempDir())
		join := partsparser.JoinRecord(env.AnchorPath)
		commits := 0
		env.CommitPartsPlan = func() error { commits++; return nil }
		fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(join)}}
		env.Seats = fake

		producer, err := partsPlanWriteEntry(RowIdentity{Name: "Row"}, partsPlanGatesCfg(), env)
		if err != nil {
			t.Fatalf("partsPlanWriteEntry() error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if commits != 1 {
			t.Errorf("CommitPartsPlan invoked %d times; want 1", commits)
		}
		if len(fake.GotTables) != 1 {
			t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
		}
		if gate := fake.GotTables[0].Gate; len(gate) != 1 || gate[0].Name != "plan" || gate[0].Attempts != 2 {
			t.Errorf("chair gate = %+v; want one \"plan\" entry with 2 attempts", gate)
		}
		if len(*handed) != 1 {
			t.Fatalf("table source evaluated %d times; want 1", len(*handed))
		}
		seats := (*handed)[0]
		spec, err := seats.Gates("whole")
		if err != nil {
			t.Fatalf("Gates(whole) error = %v; want nil", err)
		}
		if len(spec) != 1 || spec[0].Name != "plan-part" || spec[0].Attempts != 3 || spec[0].Gate == nil {
			t.Errorf("Gates(whole) = %+v; want one \"plan-part\" entry with 3 attempts and a gate", spec)
		}
		if got, want := seats.FindingsPath("whole"), env.GateFindingsFile("Row-whole"); got != want {
			t.Errorf("FindingsPath(whole) = %q; want %q", got, want)
		}
		if seats.Accept("whole") == nil {
			t.Errorf("Accept(whole) = nil; want the part's Accept hook")
		}
	})

	t.Run("WriterStuckCommitsOnce", func(t *testing.T) {
		t.Parallel()
		env, _ := partsPlanWriteTestEnv(t)
		overview := partsparser.PartOverview(env.AnchorPath, "whole")
		commits := 0
		env.CommitPartsPlan = func() error { commits++; return nil }
		env.Seats = &shedfake.SeatRunner{Results: []seatengine.Result{{
			Chair:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			Advisors: []seatengine.SeatResult{{Name: "whole", Writer: true, EndedStep: true, Outcome: shuttleengine.OutcomeTimeout, Outputs: []string{overview}}},
		}}}

		producer, err := partsPlanWriteEntry(RowIdentity{Name: "Row"}, Config{}, env)
		if err != nil {
			t.Fatalf("partsPlanWriteEntry() error = %v; want nil", err)
		}
		pointer, outcome := callProducer(t, producer)

		if outcome != shedengine.Stuck || pointer.Path != overview {
			t.Errorf("Call() = (%v, %q); want Stuck pointing at the writer's overview %q", outcome, pointer.Path, overview)
		}
		if commits != 1 {
			t.Errorf("CommitPartsPlan invoked %d times; want 1", commits)
		}
	})
}

// callProducer calls producer once and returns its pointer and outcome, failing t on an error.
func callProducer(t *testing.T, producer shedengine.ShedProducer) (shedengine.OutputPointer, shedengine.Outcome) {
	t.Helper()
	outcome, pointer, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	return pointer, outcome
}
