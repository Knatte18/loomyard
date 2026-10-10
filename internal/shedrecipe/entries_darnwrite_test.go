// entries_darnwrite_test.go covers darnWriteEntry: its construction-time validation over Config, the injected DarnSpec and Shuttle and every Env.Darn seam, plus the happy path.

package shedrecipe

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// darnTestEnv returns newTestEnv(t) with DarnSpec and every Darn seam filled.
func darnTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.DescriptionPath = filepath.Join(dir, "summary.md")
	env.DarnSpec = func(loomshed.DarnTold) (shuttleengine.Spec, error) {
		return shuttleengine.Spec{Prompt: "darn prompt", OutputFiles: []string{filepath.Join(dir, "description.md")}}, nil
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
		{"NilDarnSpec", "DarnSpec", gated, func(e *Env) { e.DarnSpec = nil }},
		{"NilShuttle", "Shuttle", gated, func(e *Env) { e.Shuttle = nil }},
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

func TestDarnWriteEntry_HappyPath(t *testing.T) {
	cfg := Config{"gates": []any{
		map[string]any{"name": "verify", "attempts": 2},
		map[string]any{"name": "description", "attempts": 2},
	}}
	producer, err := darnWriteEntry("Row", cfg, darnTestEnv(t))
	if err != nil {
		t.Fatalf("darnWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("darnWriteEntry() = nil producer; want non-nil")
	}
}
