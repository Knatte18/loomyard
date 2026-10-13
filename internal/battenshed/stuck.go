// stuck.go declares reportStuck and reportWait, the helpers all four producers in this package route every
// stuck verdict through, and writeStuckReason, the reason file half they share.
// A stuck verdict takes its log level from its cause: reportStuck logs a failed producer step at Warn, and reportWait logs an ordinary wait or move at Info.
//
// The producer seam (shedengine.ShedProducer) returns a verdict, an output pointer, and an error.
// Each producer also hands its reason to shedengine on OutputPointer.Reason, and the engine
// persists it as the run's error when a stuck verdict has no OnStuck target, which is the case for
// Worktree-Create, Seed-Child and Worktree-Teardown.
// Run-Shed self-bounces and ends blocked through shedengine's budget arm, whose persisted error is
// the fixed bounce-budget literal, so the one-line reason file this helper writes is the only
// durable carrier of its reason there.
// The structured warning log line scrolls away in an unattended run, so the file is also what the
// tests assert two stuck causes are distinguishable against.

package battenshed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
)

// stuckFileSuffix is the fixed suffix every producer's reason file carries, joined onto the
// producer's own name.
const stuckFileSuffix = "-stuck.md"

// StuckReasonFile returns the path reportStuck writes producer's one-line reason to, under
// scratchDir.
// It is exported so the file's reader and writer share one declarer of its name: the file is the
// only durable carrier of the reason when Run-Shed blocks through the budget arm, so a CLI
// reporting why a run is blocked reads it.
func StuckReasonFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+stuckFileSuffix)
}

// stuckMessage is the log line every stuck verdict carries, whatever its level.
const stuckMessage = "battenshed: producer stuck"

// reportStuck emits a structured warning through the shared logger, then writes the one-line reason file through writeStuckReason.
// The warning carries a producer field and a reason field alongside fields' own key-value pairs.
// It is the route of a failed producer step; a wait or move that is no failure goes through reportWait.
func reportStuck(producer, reason, scratchDir string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn(stuckMessage, logFields...)
	writeStuckReason(producer, reason, scratchDir)
}

// reportWait is reportStuck at Info, the route of an ordinary wait or move that is no failed step.
// Such a wait ends on a state change, a requested pause or a Publish-retry action.
func reportWait(producer, reason, scratchDir string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Info(stuckMessage, logFields...)
	writeStuckReason(producer, reason, scratchDir)
}

// writeStuckReason writes a one-line reason file at <scratchDir>/<producer>-stuck.md, overwritten each attempt.
//
// It creates scratchDir with os.MkdirAll first, on every write path -- a told directory may not
// exist yet, and making a told path usable is legal where deriving one would not be. A write
// failure is logged rather than swallowed, and never replaces the caller's own verdict: failing to
// record why something is stuck must not change whether it is stuck, so this function returns
// nothing for a caller to act on.
func writeStuckReason(producer, reason, scratchDir string) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for stuck-reason file failed", "producer", producer, "scratchDir", scratchDir, "error", err)
		return
	}

	path := StuckReasonFile(scratchDir, producer)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%s\n", reason)), 0o644); err != nil {
		logger.Warn("battenshed: write stuck-reason file failed", "producer", producer, "path", path, "error", err)
	}
}
