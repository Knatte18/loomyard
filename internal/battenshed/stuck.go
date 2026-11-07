// stuck.go declares the helpers the producers in this package route their stuck verdicts through.
// reportStuck and reportWait log a verdict and write its reason file.
// writeStuckReason is the reason file half they share.
// A stuck verdict takes its log level from its cause.
// reportStuck logs a failed producer step at Warn, and reportWait logs an ordinary wait or move at Info.
//
// The producer seam (shedengine.ShedProducer) returns a verdict, an output pointer, and an error.
// Each producer also hands its reason to shedengine on OutputPointer.Reason,
// and the engine persists it as the run's error when a stuck verdict has no OnStuck target.
// Run-Shed self-bounces and ends blocked through shedengine's budget arm.
// Its persisted error is a fixed bounce-budget literal.
// So the one-line reason file this helper writes is the only durable carrier of its reason.
// The structured warning log line scrolls away in an unattended run,
// so the file is also what the tests assert two stuck causes are distinguishable against.

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

// StuckReasonFile returns the path of the one-line reason file for producer under scratchDir.
// It is exported so a CLI reporting why a run is blocked can read the file the producers write.
func StuckReasonFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+stuckFileSuffix)
}

// stuckMessage is the log line every stuck verdict carries, whatever its level.
const stuckMessage = "battenshed: producer stuck"

// reportStuck logs a structured warning through the shared logger, then writes the one-line reason file.
// The warning carries a producer field and a reason field alongside fields' own key-value pairs.
// It is the route for a failed producer step.
func reportStuck(producer, reason, scratchDir string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn(stuckMessage, logFields...)
	writeStuckReason(producer, reason, scratchDir)
}

// reportWait logs a stuck verdict at Info, for an ordinary wait or move that is no failed step.
// Such a wait ends on a state change, or a requested pause.
func reportWait(producer, reason, scratchDir string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Info(stuckMessage, logFields...)
	writeStuckReason(producer, reason, scratchDir)
}

// writeStuckReason writes a one-line reason file at <scratchDir>/<producer>-stuck.md, overwritten each attempt.
//
// scratchDir may not exist yet, so the function creates it with os.MkdirAll first.
// A failed create or write is logged, not returned, and never changes the caller's verdict:
// failing to record why something is stuck must not change whether it is stuck.
// For that reason the function returns nothing for a caller to act on.
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
