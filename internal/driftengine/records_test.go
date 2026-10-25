//go:build tier0

// records_test.go — tier 0 tests for the counted-capture predicate and the Drift-Verify run counter.

package driftengine

import (
	"errors"
	"testing"
)

// TestCountedCapture covers a found and an absent capture, each state, awaited_at set or not, and a loom (await row) and a darn recipe.
func TestCountedCapture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    string
		found    bool
		awaited  bool
		awaitRow bool
		want     bool
	}{
		{"absent", CapturePassed, false, true, true, false},
		{"running", CaptureRunning, true, true, true, false},
		{"failed", CaptureFailed, true, true, true, false},
		{"passed and awaited under an await row", CapturePassed, true, true, true, true},
		{"passed but not awaited under an await row", CapturePassed, true, false, true, false},
		{"passed and not awaited without an await row", CapturePassed, true, false, false, true},
		{"failed without an await row", CaptureFailed, true, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scope := ScopeRecord{}
			if tt.awaited {
				scope.AwaitedAt = "2026-01-01T00:00:00Z"
			}
			got := CountedCapture(scope, CaptureRecord{State: tt.state}, tt.found, tt.awaitRow)
			if got != tt.want {
				t.Errorf("CountedCapture() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestTakeRun covers taking runs up to the budget, the refusal wrapping ErrRunsSpent leaving the count, ResetRuns admitting again, AttemptHead and the agent-session mark.
func TestTakeRun(t *testing.T) {
	t.Parallel()
	p := NewPaths(t.TempDir())

	if _, found, err := AttemptHead(p); err != nil || found {
		t.Fatalf("AttemptHead() with no record = _, %v, %v; want found false", found, err)
	}
	if open, err := AgentSessionOpen(p); err != nil || open {
		t.Fatalf("AgentSessionOpen() with no record = %v, %v; want false", open, err)
	}
	if err := CloseAgentSession(p); err != nil {
		t.Fatalf("CloseAgentSession() with no record = %v; want nil", err)
	}
	if _, found, _ := AttemptHead(p); found {
		t.Fatalf("CloseAgentSession() with no record wrote one")
	}

	if err := ResetRuns(p, "abc"); err != nil {
		t.Fatalf("ResetRuns() = %v; want nil", err)
	}
	for want := 1; want <= 2; want++ {
		if got, err := TakeRun(p, 2); err != nil || got != want {
			t.Fatalf("TakeRun() = %d, %v; want %d, nil", got, err, want)
		}
	}
	if _, err := TakeRun(p, 2); !errors.Is(err, ErrRunsSpent) {
		t.Fatalf("TakeRun() past the budget = %v; want it to wrap ErrRunsSpent", err)
	}
	if got, err := TakeRun(p, 3); err != nil || got != 3 {
		t.Errorf("TakeRun() after a refusal = %d, %v; want 3, nil (the refusal left the count at 2)", got, err)
	}

	if err := CloseAgentSession(p); err != nil {
		t.Fatalf("CloseAgentSession() = %v; want nil", err)
	}
	if open, _ := AgentSessionOpen(p); open {
		t.Errorf("AgentSessionOpen() after CloseAgentSession = true; want false")
	}
	if head, found, _ := AttemptHead(p); !found || head != "abc" {
		t.Errorf("AttemptHead() after CloseAgentSession = %q, %v; want abc, true", head, found)
	}
	if _, err := TakeRun(p, 3); !errors.Is(err, ErrRunsSpent) {
		t.Errorf("TakeRun() after CloseAgentSession = %v; want the count kept, refusing at 3", err)
	}

	if err := ResetRuns(p, "def"); err != nil {
		t.Fatalf("ResetRuns() = %v; want nil", err)
	}
	if got, err := TakeRun(p, 3); err != nil || got != 1 {
		t.Errorf("TakeRun() after ResetRuns = %d, %v; want 1, nil", got, err)
	}
	if head, _, _ := AttemptHead(p); head != "def" {
		t.Errorf("AttemptHead() after ResetRuns = %q; want def", head)
	}
	if open, _ := AgentSessionOpen(p); !open {
		t.Errorf("AgentSessionOpen() after ResetRuns = false; want true")
	}
}
