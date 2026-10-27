//go:build tier0

// entries_drift_test.go covers the three Drift entries: their config keys, their required Env seams and the producers they build.

package shedrecipe

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// driftTestEnv returns newTestEnv(t) with every Env.Drift seam filled by a no-op.
func driftTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	env.Drift = driftengine.Deps{
		Config:        func() (driftengine.Config, error) { return driftengine.Config{}, nil },
		Status:        func() (shedengine.Status, error) { return shedengine.Status{}, nil },
		Head:          func() (string, error) { return "head", nil },
		BranchPoint:   func() (string, error) { return "branch-point", nil },
		DirtyPaths:    func() ([]string, error) { return nil, nil },
		Commit:        func([]string) error { return nil },
		BaselineTable: func(string) (seatengine.Table, error) { return seatengine.Table{}, nil },
		StartCapture:  func() error { return nil },
		RunCapture: func(context.Context) (driftengine.CaptureRecord, error) {
			return driftengine.CaptureRecord{}, nil
		},
		Sleep:       func(context.Context, time.Duration) error { return nil },
		VerifyTable: func(driftengine.VerifyTold) (seatengine.Table, error) { return seatengine.Table{}, nil },
		Compare: func(context.Context) (driftengine.CompareResult, error) {
			return driftengine.CompareResult{}, nil
		},
	}
	return env
}

// TestDriftEntries asserts each Drift entry builds its producer with the kit's Env, and refuses an unknown key, a bad subject, a wrong gates list and a nil seam, each naming the way forward.
func TestDriftEntries(t *testing.T) {
	t.Parallel()

	verifyCfg := func(gates ...any) Config {
		return Config{"subject": "decision-record", "gate_key": "drift-verify", "gates": gates}
	}
	verifyGate := map[string]any{"name": "verify", "attempts": 3}
	tests := []struct {
		name  string
		entry Constructor
		cfg   Config
		edit  func(*Env)
		// wantType is the producer type a build must return; wantErr is what a refusal must mention.
		wantType reflect.Type
		wantErr  string
	}{
		{"BaselineDecisionRecord", driftBaselineEntry, Config{"subject": "decision-record"}, nil, reflect.TypeOf(&driftengine.DriftBaseline{}), ""},
		{"BaselineBoardEntry", driftBaselineEntry, Config{"subject": "board-entry"}, nil, reflect.TypeOf(&driftengine.DriftBaseline{}), ""},
		{"BaselineCaptureKey", driftBaselineEntry, Config{"subject": "board-entry", "capture": "wait"}, nil, nil, "entry_gate"},
		{"BaselineMissingSubject", driftBaselineEntry, Config{}, nil, nil, "way forward: set subject"},
		{"BaselineUnknownSubject", driftBaselineEntry, Config{"subject": "issue"}, nil, nil, "way forward: set subject"},
		{"BaselineNilSeats", driftBaselineEntry, Config{"subject": "decision-record"}, func(e *Env) { e.Seats = nil }, nil, "Seats"},
		{"BaselineNilStartCapture", driftBaselineEntry, Config{"subject": "decision-record"}, func(e *Env) { e.Drift.StartCapture = nil }, nil, "Drift.StartCapture"},
		{"AwaitBuilds", driftAwaitEntry, Config{}, nil, reflect.TypeOf(&driftengine.DriftAwait{}), ""},
		{"AwaitUnknownKey", driftAwaitEntry, Config{"subject": "decision-record"}, nil, nil, "entry_gate"},
		{"AwaitNilSleep", driftAwaitEntry, Config{}, func(e *Env) { e.Drift.Sleep = nil }, nil, "Drift.Sleep"},
		{"VerifyBuilds", driftVerifyEntry, verifyCfg(verifyGate), nil, reflect.TypeOf(&driftengine.DriftVerify{}), ""},
		{"VerifyUnknownKey", driftVerifyEntry, Config{"subject": "decision-record", "gates": []any{verifyGate}, "enabled": true}, nil, nil, "entry_gate"},
		{"VerifyMissingSubject", driftVerifyEntry, Config{"gates": []any{verifyGate}}, nil, nil, "way forward: set subject"},
		{"VerifyNoGates", driftVerifyEntry, Config{"subject": "decision-record"}, nil, nil, "way forward: set gates"},
		{"VerifySecondGate", driftVerifyEntry, verifyCfg(verifyGate, map[string]any{"name": "description", "attempts": 3}), nil, nil, "way forward: set gates"},
		{"VerifyOnlyOtherGate", driftVerifyEntry, verifyCfg(map[string]any{"name": "description", "attempts": 3}), nil, nil, "way forward: set gates"},
		{"VerifyNilCompare", driftVerifyEntry, verifyCfg(verifyGate), func(e *Env) { e.Drift.Compare = nil }, nil, "Drift.Compare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := driftTestEnv(t)
			if tt.edit != nil {
				tt.edit(&env)
			}

			producer, err := tt.entry(RowIdentity{Name: "Drift-Row"}, tt.cfg, env)

			if tt.wantErr != "" {
				assertErrContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("entry() error = %v; want nil", err)
			}
			if got := reflect.TypeOf(producer); got != tt.wantType {
				t.Errorf("entry() producer type = %v; want %v", got, tt.wantType)
			}
		})
	}
}
