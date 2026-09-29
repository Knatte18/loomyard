// stuck.go declares reportStuck, the one helper both producers route every stuck verdict through.
//
// The producer seam (shedengine.ShedProducer) carries a producer's reason on
// OutputPointer.Reason, which the engine persists as the blocked error when the row has no OnStuck
// target -- every landingshed row has none. This helper is the log-line carrier beside it: a
// structured warning that stays in the trace after the persisted error is overwritten.

package landingshed

import "github.com/Knatte18/loomyard/internal/logger"

// reportStuck emits a structured warning through the shared logger carrying at minimum a producer
// field and a reason field alongside fields' own key-value pairs.
func reportStuck(producer, reason string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn("landingshed: producer stuck", logFields...)
}
