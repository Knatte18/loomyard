// rungate.go is the in-process entry point to one gate key, for a caller that runs a gate itself and hears each tier as it starts and ends.

package gatecli

import (
	"context"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TierObserver hears the tiers of one gate run.
// A nil func is skipped, so the zero TierObserver observes nothing.
type TierObserver struct {
	// Started is called with the tier's number once its slots are held, or at once for a run that takes none, before its command runs.
	Started func(tier int)
	// Ended is called with the tier's run record when the tier ends.
	// A tier carried as already green, or blocked by a failed build, ends with no start of its own.
	Ended func(record gateslot.RunRecord)
}

// RunGate runs the tiers of the gate.yaml key gate in dir as `lym gate test --wait --gate <gate>` does, calling observer for each tier, and returns the call's exit code.
// The envelope goes to out and the summary lines to stderr.
func RunGate(ctx context.Context, dir, gate string, out, stderr io.Writer, observer TierObserver) int {
	request := testRequest{
		goBinary: defaultGoBinary, samplePeriod: defaultSamplePeriod, slotWait: shuttleengine.DefaultAwaitCap, preTierCeiling: defaultPreTierCeiling, timeoutUnit: time.Minute, shortageRetryDelay: defaultShortageRetryDelay,
		stderr: stderr, gate: gate, wait: true, observer: observer,
	}
	return runTest(lymcwd.WithCwd(ctx, dir), out, request)
}
