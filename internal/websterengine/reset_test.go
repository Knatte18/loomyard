// reset_test.go pins wayForwardSteps and the PlanReset refusals that fire before any git probe: the held run lock and the missing state.
// Tier 1: no git, only t.TempDir() and a file lock.

package websterengine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestWayForwardSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		steps []string
		want  string
	}{
		{"none", nil, ""},
		{"one", []string{"a"}, "way forward: a"},
		{"two", []string{"a", "b"}, "way forward: 1) a; 2) b"},
		{"three", []string{"a", "b", "c"}, "way forward: 1) a; 2) b; 3) c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := wayForwardSteps(tt.steps...); got != tt.want {
				t.Errorf("wayForwardSteps(%v) = %q, want %q", tt.steps, got, tt.want)
			}
		})
	}
}

func TestPlanReset_HeldRunLockIsTransientBusy(t *testing.T) {
	t.Parallel()

	scratch := t.TempDir()
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratch, runLockName))
	if err != nil || !locked {
		t.Fatalf("hold run lock: locked=%v err=%v", locked, err)
	}
	defer held.Release()

	_, err = PlanReset(ResetDeps{Geom: Geometry{ScratchDir: scratch}, State: &State{}}, ResetToStart)
	if !errors.Is(err, ErrRunBusy) {
		t.Fatalf("PlanReset error = %v, want ErrRunBusy", err)
	}
	if !strings.Contains(err.Error(), "wait for it to finish, or check `lyx webster status`") {
		t.Errorf("error = %v, want the wait way forward", err)
	}
}

func TestPlanReset_NoStateNamesRun(t *testing.T) {
	t.Parallel()

	_, err := PlanReset(ResetDeps{Geom: Geometry{ScratchDir: t.TempDir()}}, ResetToPreFix)
	if err == nil {
		t.Fatal("PlanReset error = nil, want the no-state refusal")
	}
	if !strings.Contains(err.Error(), "way forward: run `lyx webster run` first") {
		t.Errorf("error = %v, want the run-first way forward", err)
	}
}
