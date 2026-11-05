// entries_drift_test.go covers the Drift entries: their config keys, their required Env seams and the producers they build.

package shedrecipe

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// driftTestEnv returns newTestEnv(t) with every Env.Drift seam filled by a no-op.
func driftTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	env.Drift = driftengine.Deps{
		Config:      func() (driftengine.Config, error) { return driftengine.Config{}, nil },
		Status:      func() (shedengine.Status, error) { return shedengine.Status{}, nil },
		Head:        func() (string, error) { return "head", nil },
		BranchPoint: func() (string, error) { return "branch-point", nil },
		DirtyPaths:  func() ([]string, error) { return nil, nil },
		Commit:      func([]string) error { return nil },
		CommitWithTrailers: func([]string, []fabricengine.Trailer) error {
			return nil
		},
		TrailerCommit: func(string, bool) (fabricengine.TrailerCommit, bool, error) {
			return fabricengine.TrailerCommit{}, false, nil
		},
		FileAt:       func(string, string) ([]byte, bool, error) { return nil, false, nil },
		CommitsSince: func(string) ([]string, error) { return nil, nil },
		ExpectTable:  func(string) (seatengine.Table, error) { return seatengine.Table{}, nil },
		StartCapture: func() error { return nil },
		JudgeTable:   func(string) (seatengine.Table, error) { return seatengine.Table{}, nil },
		Checkouts: func() (fabricengine.CheckoutState, fabricengine.CheckoutState, error) {
			return fabricengine.CheckoutState{}, fabricengine.CheckoutState{}, nil
		},
		RecordsChangedSince: func(string) ([]string, error) { return nil, nil },
		EndRoles:            func([]string) error { return nil },
		CaptureCurrent: func(context.Context) (driftengine.CurrentCapture, error) {
			return driftengine.CurrentCapture{}, nil
		},
		RunCapture: func(context.Context) (driftengine.CaptureRecord, error) {
			return driftengine.CaptureRecord{}, nil
		},
		Sleep: func(context.Context, time.Duration) error { return nil },
		Compare: func(context.Context) (driftengine.CompareResult, error) {
			return driftengine.CompareResult{}, nil
		},
	}
	return env
}

// TestDriftEntries asserts each Drift entry builds its producer with the kit's Env, and refuses an unknown key, a bad subject, a wrong gates list and a nil seam, each naming the way forward.
func TestDriftEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	verifyCfg := func(gates ...any) Config {
		return Config{"subject": "decision-record", "gates": gates}
	}
	verifyGate := map[string]any{"name": "judgement", "attempts": 3}
	expectGate := map[string]any{"name": "expectation", "attempts": 3}
	expectCfg := Config{"subject": "decision-record", "gates": []any{expectGate}}
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
		{"BaselineNilEndRoles", driftBaselineEntry, Config{"subject": "decision-record"}, func(e *Env) { e.Drift.EndRoles = nil }, nil, "Drift.EndRoles"},
		{"BaselineNilStartCapture", driftBaselineEntry, Config{"subject": "decision-record"}, func(e *Env) { e.Drift.StartCapture = nil }, nil, "Drift.StartCapture"},
		{"ExpectBuilds", driftExpectEntry, expectCfg, nil, reflect.TypeOf(&driftengine.DriftExpect{}), ""},
		{"ExpectUnknownKey", driftExpectEntry, Config{"subject": "decision-record", "gates": []any{expectGate}, "gate_key": "x"}, nil, nil, "entry_gate"},
		{"ExpectMissingSubject", driftExpectEntry, Config{"gates": []any{expectGate}}, nil, nil, "way forward: set subject"},
		{"ExpectNoGates", driftExpectEntry, Config{"subject": "decision-record"}, nil, nil, "way forward: set gates"},
		{"ExpectOtherGate", driftExpectEntry, Config{"subject": "decision-record", "gates": []any{map[string]any{"name": "description", "attempts": 3}}}, nil, nil, "way forward: set gates"},
		{"ExpectSecondGate", driftExpectEntry, Config{"subject": "decision-record", "gates": []any{expectGate, map[string]any{"name": "description", "attempts": 3}}}, nil, nil, "way forward: set gates"},
		{"ExpectNilSeats", driftExpectEntry, expectCfg, func(e *Env) { e.Seats = nil }, nil, "Seats"},
		{"ExpectNilExpectTable", driftExpectEntry, expectCfg, func(e *Env) { e.Drift.ExpectTable = nil }, nil, "Drift.ExpectTable"},
		{"ExpectNilCommitsSince", driftExpectEntry, expectCfg, func(e *Env) { e.Drift.CommitsSince = nil }, nil, "Drift.CommitsSince"},
		{"ExpectNilFileAt", driftExpectEntry, expectCfg, func(e *Env) { e.Drift.FileAt = nil }, nil, "Drift.FileAt"},
		{"AwaitBuilds", driftAwaitEntry, Config{}, nil, reflect.TypeOf(&driftengine.DriftAwait{}), ""},
		{"AwaitUnknownKey", driftAwaitEntry, Config{"subject": "decision-record"}, nil, nil, "entry_gate"},
		{"AwaitNilSleep", driftAwaitEntry, Config{}, func(e *Env) { e.Drift.Sleep = nil }, nil, "Drift.Sleep"},
		{"AwaitNilConfig", driftAwaitEntry, Config{}, func(e *Env) { e.Drift.Config = nil }, nil, "Drift.Config"},
		{"VerifyBuilds", driftVerifyEntry, verifyCfg(verifyGate), nil, reflect.TypeOf(&driftengine.DriftVerify{}), ""},
		{"VerifyUnknownKey", driftVerifyEntry, Config{"subject": "decision-record", "gates": []any{verifyGate}, "enabled": true}, nil, nil, "entry_gate"},
		{"VerifyRefusesGateKey", driftVerifyEntry, Config{"subject": "decision-record", "gates": []any{verifyGate}, "gate_key": "drift-verify"}, nil, nil, "entry_gate"},
		{"VerifyMissingSubject", driftVerifyEntry, Config{"gates": []any{verifyGate}}, nil, nil, "way forward: set subject"},
		{"VerifyNoGates", driftVerifyEntry, Config{"subject": "decision-record"}, nil, nil, "way forward: set gates"},
		{"VerifySecondGate", driftVerifyEntry, verifyCfg(verifyGate, map[string]any{"name": "description", "attempts": 3}), nil, nil, "way forward: set gates"},
		{"VerifyOnlyOtherGate", driftVerifyEntry, verifyCfg(map[string]any{"name": "verify", "attempts": 3}), nil, nil, "way forward: set gates"},
		{"VerifyNilCompare", driftVerifyEntry, verifyCfg(verifyGate), func(e *Env) { e.Drift.Compare = nil }, nil, "Drift.Compare"},
		{"VerifyNilRecordsChangedSince", driftVerifyEntry, verifyCfg(verifyGate), func(e *Env) { e.Drift.RecordsChangedSince = nil }, nil, "Drift.RecordsChangedSince"},
		{"VerifyNilEndRoles", driftVerifyEntry, verifyCfg(verifyGate), func(e *Env) { e.Drift.EndRoles = nil }, nil, "Drift.EndRoles"},
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
