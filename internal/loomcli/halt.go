// halt.go holds loom's halt friction: the Go-authored note a `blocked` or `failed` halt leaves, and the two hooks that write it and reflect, loomAfterStep under step and loomPostRun under run.
//
// A halt has one path: the halt note, then the reflection, which files only the lyx problems behind the halt.
// An escalation halt writes its note and skips the reflection.

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
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// haltNote describes one `blocked` or `failed` halt.
type haltNote struct {
	Producer  string
	State     shedengine.State
	Reason    string
	History   []shedengine.HistoryEntry
	TraceFile string
}

// writeHistoryRows appends the history section of a friction note: one `- <producer> / <outcome> / <at>` row per entry, or a single line saying there are none.
func writeHistoryRows(b *strings.Builder, history []shedengine.HistoryEntry) {
	b.WriteString("\nhistory:\n")
	if len(history) == 0 {
		b.WriteString("no history entries\n")
		return
	}
	for _, h := range history {
		b.WriteString("- " + h.Producer + " / " + string(h.Outcome) + " / " + h.At + "\n")
	}
}

// writeHaltNote records n as a friction note named `loom-halt`, carrying the anomaly kind the halt classifies as and the history behind it.
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
	b.WriteString("producer: " + n.Producer + "\n")
	b.WriteString("state: " + string(n.State) + "\n")
	if kind, ok := loomengine.ClassifyHalt(n.State, n.Reason); ok {
		b.WriteString("anomaly: " + string(kind) + "\n")
	}
	b.WriteString("history_entries: " + strconv.Itoa(len(n.History)) + "\n")
	b.WriteString("trace_file: " + n.TraceFile + "\n")
	writeHistoryRows(&b, n.History)

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("loom: write halt note %s: %w", path, err)
	}
	return nil
}

// reflectHalt writes n as a halt note and runs the non-waiting reflection, returning the envelope's "friction" status.
// A note write failure only logs.
// A held reflection lock skips the reflection, as reflectFriction(false) does.
// An `escalation-to-human` halt writes its note and skips the reflection: the producer's own session holds the open question,
// so a second session reflecting on it finds only a process signal.
// Only that kind skips,
// and the skipped reflection is the only thing lost: the note stays on disk unarchived until a later reflection covers it.
func (c *loomCLI) reflectHalt(n haltNote) string {
	if c.frictionDir == "" {
		return frictionengine.StatusSkipped
	}
	if err := writeHaltNote(c.frictionDir, n); err != nil {
		logger.Warn("loom: could not write the halt note", "dir", c.frictionDir, "error", err)
	}
	if kind, ok := loomengine.ClassifyHalt(n.State, n.Reason); ok && kind == loomengine.AnomalyEscalation {
		logger.Info("loom: reflection skipped for an escalation halt", "producer", n.Producer)
		return frictionengine.StatusSkipped
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
		Producer:  st.CurrentProducer,
		State:     shedengine.StateFailed,
		Reason:    st.Error,
		History:   st.History,
		TraceFile: logger.TraceFile(),
	}, true
}

// loomAfterStep is loom's AfterStep hook and returns the envelope's "friction" key.
// A `failed` halt behind stepErr and a `blocked` result each write a halt note and reflect.
// `done` reports what the Friction-Reflect row recorded, or skipped when the row did not run in this process.
// Every other state, awaiting and paused included, writes nothing and reports skipped.
// Every step that returns, with or without an error, vouches for what it left on disk: a step error records the handoff voucher from the status file before the halt check, except a busy refusal, whose driver vouches on its own exit.
func (c *loomCLI) loomAfterStep(ctx context.Context, res shedengine.StepResult, stepErr error) string {
	if stepErr != nil {
		if !errors.Is(stepErr, shedengine.ErrShedBusy) {
			recordHandoffVoucherFromStatus(loomengine.LoomHandoffVoucher(c.location), loomengine.LoomHandoffVoucherLock(c.location), c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
		}
		if n, ok := c.failedHalt(stepErr); ok {
			return c.reflectHalt(n)
		}
		return frictionengine.StatusSkipped
	}
	switch res.State {
	case shedengine.StateBlocked:
		return c.reflectHalt(haltNote{
			Producer:  res.Producer,
			State:     shedengine.StateBlocked,
			Reason:    res.Reason,
			History:   res.History,
			TraceFile: logger.TraceFile(),
		})
	case shedengine.StateDone:
		if c.rowFrictionStatus != "" {
			return c.rowFrictionStatus
		}
	}
	return frictionengine.StatusSkipped
}
