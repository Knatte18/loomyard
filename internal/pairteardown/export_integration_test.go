//go:build integration && !windows

// export_integration_test.go re-exports the seams teardown_integration_test.go drives:
// internal/pairteardown sits inside internal/fabriccli's dependency set, so its hubforge-using test cannot live in-package without closing a compile cycle through internal/hubforge — the standard Go export_test.go idiom.

package pairteardown

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReedEngineForTest builds the reed engine for the pair's present task worktree.
func (t *Teardown) ReedEngineForTest(slug string) (*reedengine.Engine, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return nil, err
	}
	return t.reedEngine(task)
}

// SetIntervalForTest sets the quiet-wait poll interval.
func (t *Teardown) SetIntervalForTest(d time.Duration) {
	t.interval = d
}

// SleepForTest returns the quiet wait's current sleep.
func (t *Teardown) SleepForTest() func(ctx context.Context, d time.Duration) error {
	return t.sleep
}

// SetSleepForTest replaces the quiet wait's sleep.
func (t *Teardown) SetSleepForTest(sleep func(ctx context.Context, d time.Duration) error) {
	t.sleep = sleep
}
