// halt.go holds loom's halt friction: the Go-authored note a `blocked` or `failed` halt leaves, and the two hooks that write it and reflect, loomAfterStep under step and loomPostRun under run.
//
// Two paths file a halt and they split it: Tier 1 anomaly filing (detectAndFileAnomalies), which both hooks run first, files the halt event under its stable anomaly title, and the reflection files only the lyx problems behind it.
// With the selfreport knob on the halt note says so, so one halt is not filed twice.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// haltNote describes one `blocked` or `failed` halt.
type haltNote struct {
	Producer   string
	State      shedengine.State
	Reason     string
	HistoryLen int
	TraceFile  string
	// TierOneFiled is set when Tier 1 anomaly filing is on, which files the halt event itself.
	TierOneFiled bool
}

// writeHaltNote records n as a friction note named `loom-halt`.
// It is a no-op when frictionDir is empty (Tier 2 off) or when friction.NotePath rejects the id.
func writeHaltNote(frictionDir string, n haltNote) error {
	if frictionDir == "" {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "loom-halt")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("loom halted " + string(n.State) + " at " + n.Producer + "\n\n")
	b.WriteString("```\n" + n.Reason + "\n```\n\n")
	if n.TierOneFiled {
		b.WriteString("Tier 1 anomaly filing files the halt event itself; file only the lyx problems behind it.\n\n")
	}
	b.WriteString("producer: " + n.Producer + "\n")
	b.WriteString("state: " + string(n.State) + "\n")
	b.WriteString("history_entries: " + strconv.Itoa(n.HistoryLen) + "\n")
	b.WriteString("trace_file: " + n.TraceFile + "\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("loom: write halt note %s: %w", path, err)
	}
	return nil
}

// reflectHalt writes n as a halt note and runs the non-waiting reflection, returning the envelope's "friction" status.
// A note write failure only logs.
// A held reflection lock skips the reflection, as reflectFriction(false) does.
func (c *loomCLI) reflectHalt(n haltNote) string {
	if c.frictionDir == "" {
		return frictionengine.StatusSkipped
	}
	n.TierOneFiled = c.cfg.Selfreport
	if err := writeHaltNote(c.frictionDir, n); err != nil {
		logger.Warn("loom: could not write the halt note", "dir", c.frictionDir, "error", err)
	}
	return c.reflectFriction(false)
}

// failedHalt reports the halt behind err when it is a `failed` one.
// It is never one for shedengine.ErrShedBusy,
// and otherwise only when the status file reads `failed` with a persisted error equal to err.Error(), which is how shedengine persists the producer-error and unrecognised-outcome arms.
// A stale `failed` status left by an earlier invocation therefore never writes a second note.
func (c *loomCLI) failedHalt(err error) (haltNote, bool) {
	if err == nil || errors.Is(err, shedengine.ErrShedBusy) {
		return haltNote{}, false
	}
	st, found, readErr := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if readErr != nil || !found {
		return haltNote{}, false
	}
	if st.State != shedengine.StateFailed || st.Error != err.Error() {
		return haltNote{}, false
	}
	return haltNote{
		Producer:   st.CurrentProducer,
		State:      shedengine.StateFailed,
		Reason:     st.Error,
		HistoryLen: len(st.History),
		TraceFile:  logger.TraceFile(),
	}, true
}

// loomAfterStep is loom's AfterStep hook and returns the envelope's "friction" key.
// It first runs Tier 1 anomaly filing on every path, ahead of the halt note and reflection, with the skips detectAndFileAnomalies owns: the knob off, a busy step and a cancelled context each file nothing more.
// A `failed` halt behind stepErr and a `blocked` result each write a halt note and reflect.
// `done` reports what the Friction-Reflect row recorded, or skipped when the row did not run in this process.
// Every other state, awaiting and paused included, writes nothing and reports skipped.
func (c *loomCLI) loomAfterStep(ctx context.Context, res shedengine.StepResult, stepErr error) string {
	detectAndFileAnomalies(c.anomalyDeps(ctx, stepErr))

	if stepErr != nil {
		if n, ok := c.failedHalt(stepErr); ok {
			return c.reflectHalt(n)
		}
		return frictionengine.StatusSkipped
	}
	switch res.State {
	case shedengine.StateBlocked:
		return c.reflectHalt(haltNote{
			Producer:   res.Producer,
			State:      shedengine.StateBlocked,
			Reason:     res.Reason,
			HistoryLen: len(res.History),
			TraceFile:  logger.TraceFile(),
		})
	case shedengine.StateDone:
		if c.rowFrictionStatus != "" {
			return c.rowFrictionStatus
		}
	}
	return frictionengine.StatusSkipped
}
