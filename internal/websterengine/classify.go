// classify.go implements webster's own recovery-classification decision logic, deciding a batch's
// outcome from webster's Report/Digest types.
// The flat plan format carries no `## Scope`, so there is nothing for a changed file to be judged
// against beyond the fork's own informational deviation list.
// The pinned decision order (report present wins outright; otherwise turn-ended, then timeout, then
// dead-pane, then running) is a frozen, cross-checked invariant that Classify preserves exactly,
// not a webster-specific choice.

package websterengine

import (
	"fmt"
	"time"
)

// ClassifyInputs carries every signal Classify's decision needs.
type ClassifyInputs struct {
	BatchNumber  int
	BatchSlug    string
	ReportPath   string
	Report       *Report // Non-nil always wins; nil when caller found none
	TurnEnded    bool    // True if Stop event was observed
	StrandLive   bool    // True if fork's reed strand is still live
	Elapsed      time.Duration
	BatchTimeout time.Duration
}

// Classify decides a batch's classification from ins in pinned order:
// 1. Report present → terminal (done/stuck per OK/FAILED).
// 2. No report, TurnEnded → terminal dead (DeadReasonAsking).
// 3. No report, Elapsed > BatchTimeout → terminal dead (DeadReasonTimeout).
// 4. No report, StrandLive false → terminal dead (DeadReasonDied).
// 5. Otherwise → non-terminal running snapshot.
func Classify(in ClassifyInputs) (Digest, bool) {
	batch := fmt.Sprintf("%02d-%s", in.BatchNumber, in.BatchSlug)

	if in.Report != nil {
		d := distill(in.Report)
		d.Batch = batch
		return d, true
	}

	if in.TurnEnded {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonAsking}, true
	}

	if in.Elapsed > in.BatchTimeout {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonTimeout}, true
	}

	if !in.StrandLive {
		return Digest{Batch: batch, Status: DigestStatusDead, DeadReason: DeadReasonDied}, true
	}

	return Digest{Batch: batch, Status: DigestStatusRunning, ElapsedS: int(in.Elapsed.Seconds())}, false
}
