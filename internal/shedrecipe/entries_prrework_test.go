// entries_prrework_test.go covers prReworkEntry: its construction-time validation over Config, the
// injected ReworkSpec and Shuttle, every Env.Rework seam and its two told directories.

package shedrecipe

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// reworkTestEnv returns newTestEnv(t) with ReworkSpec and every Rework seam filled.
func reworkTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.ReworkSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{Prompt: "rework prompt", OutputFiles: []string{filepath.Join(dir, "coverage.md")}}, nil
	}
	env.Rework = loomshed.PRReworkDeps{
		PlanDir:        filepath.Join(dir, "plan"),
		ReworkDir:      filepath.Join(dir, "rework"),
		ReworkDirRel:   "_lyx/loom/rework",
		ReadCommitted:  func(string) ([]byte, bool, error) { return nil, false, nil },
		ReadRejection:  func() (loomshed.PendingRejection, bool, error) { return loomshed.PendingRejection{}, false, nil },
		ClearRejection: func() error { return nil },
		Commit:         func() error { return nil },
		Rebaseline:     func() error { return nil },
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

func TestPRReworkEntry_ConstructionFailures(t *testing.T) {
	tests := []struct {
		name  string
		field string
		edit  func(*Env)
	}{
		{"NilReworkSpec", "ReworkSpec", func(e *Env) { e.ReworkSpec = nil }},
		{"NilShuttle", "Shuttle", func(e *Env) { e.Shuttle = nil }},
		{"NilReadCommitted", "Rework.ReadCommitted", func(e *Env) { e.Rework.ReadCommitted = nil }},
		{"NilReadRejection", "Rework.ReadRejection", func(e *Env) { e.Rework.ReadRejection = nil }},
		{"NilClearRejection", "Rework.ClearRejection", func(e *Env) { e.Rework.ClearRejection = nil }},
		{"NilCommit", "Rework.Commit", func(e *Env) { e.Rework.Commit = nil }},
		{"NilRebaseline", "Rework.Rebaseline", func(e *Env) { e.Rework.Rebaseline = nil }},
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
	t.Run("UnknownKey", func(t *testing.T) {
		_, err := prReworkEntry("Row", Config{"bogus_key": "x"}, reworkTestEnv(t))
		if err == nil {
			t.Fatalf("prReworkEntry() error = nil; want non-nil for an unrecognised config key")
		}
		assertErrContains(t, err, "bogus_key")
	})

	t.Run("GateAttemptsWithoutGate", func(t *testing.T) {
		_, err := prReworkEntry("Row", Config{"gate_attempts": 3}, reworkTestEnv(t))
		if err == nil {
			t.Fatalf("prReworkEntry() error = nil; want non-nil for gate_attempts with no gate")
		}
		assertErrContains(t, err, "gate_attempts")
	})
}
