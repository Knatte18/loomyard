// stuck.go declares reportStuck, the one helper both producers route every stuck verdict through.
//
// The producer seam (shedengine.ShedProducer) returns only a verdict, an output pointer, and an
// error. The driving engine persists a fixed reason string of its own for every stuck verdict
// regardless of what the producer knows, and the output pointer is never persisted anywhere -- so a
// producer-supplied reason has nowhere else to go. Two carriers exist rather than one returned
// string: a structured warning log line, which scrolls away in an unattended run, and a one-line
// reason file, which is what the tests assert two stuck causes are distinguishable against, since
// the engine's own persisted reason is identical in all of them.

package landingshed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
)

// stuckFileSuffix is the fixed suffix every producer's reason file carries, joined onto the
// producer's own name.
const stuckFileSuffix = "-stuck.md"

// reportStuck emits a structured warning through the shared logger carrying at minimum a producer
// field and a reason field alongside fields' own key-value pairs, and writes a one-line reason file
// at <scratchDir>/<producer>-stuck.md, overwritten each attempt.
//
// It creates scratchDir with os.MkdirAll first, on every write path -- a told directory may not
// exist yet, and making a told path usable is legal where deriving one would not be. A write
// failure is logged rather than swallowed, and never replaces the caller's own verdict: failing to
// record why something is stuck must not change whether it is stuck, so this function returns
// nothing for a caller to act on.
func reportStuck(producer, reason, scratchDir string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn("landingshed: producer stuck", logFields...)

	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("landingshed: create scratch directory for stuck-reason file failed", "producer", producer, "scratchDir", scratchDir, "error", err)
		return
	}

	path := filepath.Join(scratchDir, producer+stuckFileSuffix)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%s\n", reason)), 0o644); err != nil {
		logger.Warn("landingshed: write stuck-reason file failed", "producer", producer, "path", path, "error", err)
	}
}
