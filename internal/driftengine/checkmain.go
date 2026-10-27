// checkmain.go — the precondition of a main capture, checked by the capture child under its lock and by the baseline row before it starts one.

package driftengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// The rows a main capture is admitted at.
const (
	captureRowBaseline = "Drift-Baseline"
	captureRowAwait    = "Drift-Await"
)

// CheckMainCapture checks that a main capture may run.
// The status's current row must be Drift-Baseline, or Drift-Await while the status is blocked, HEAD must equal the branch point and the tracked tree must be clean.
// A blocked status at Drift-Baseline is admitted only when its error opens with CaptureHaltPrefix.
// Each refusal opens "drift capture refused:" and names its way forward.
func CheckMainCapture(status shedengine.Status, head, branchPoint string, dirty []string) error {
	atBaseline := status.CurrentProducer == captureRowBaseline
	atAwaitHalt := status.CurrentProducer == captureRowAwait && status.State == shedengine.StateBlocked
	atBaselineHalt := atBaseline && status.State == shedengine.StateBlocked

	switch {
	case atBaselineHalt && !strings.HasPrefix(status.Error, CaptureHaltPrefix):
		return fmt.Errorf("drift capture refused: %s is blocked on something other than a capture halt; way forward: resolve the halt lym loom status shows, then lym loom resume", captureRowBaseline)
	case !atBaseline && !atAwaitHalt:
		return fmt.Errorf("drift capture refused: the run is at %s (%s), and a main capture runs only at %s or at a %s halt; way forward: run lym drift capture at a %s halt or at a %s capture halt, which lym loom status shows", orNone(status.CurrentProducer), status.State, captureRowBaseline, captureRowAwait, captureRowAwait, captureRowBaseline)
	}

	if head != branchPoint {
		return fmt.Errorf("drift capture refused: HEAD %s is not the branch point %s, so the capture would not record the task's start; way forward: lym drift defer, then lym loom resume", head, branchPoint)
	}

	if len(dirty) > 0 {
		next := "lym loom resume, which captures again"
		if atAwaitHalt {
			next = "lym drift capture again, then lym loom resume, since the resumed " + captureRowAwait + " only reads the records"
		}
		return fmt.Errorf("drift capture refused: the tracked tree is dirty (%s); way forward: restore those paths to HEAD or stash them (committing them would move HEAD off the branch point), then %s; or lym drift defer, then lym loom resume", strings.Join(dirty, ", "), next)
	}
	return nil
}

// orNone returns row, or "no row" when it is empty.
func orNone(row string) string {
	if row == "" {
		return "no row"
	}
	return row
}
