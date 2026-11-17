// classify.go implements webster's recovery classification, deciding a batch's outcome from Report and Digest.
// The flat plan format has no `## Scope` section.
// A changed file therefore has nothing to be judged against beyond the scope guard's informational deviation list.
// Classify's decision order is a frozen, cross-checked invariant, not a webster-specific choice.

package websterengine

import (
	"fmt"
	"time"
)

// ClassifyInputs carries every signal that decides a batch's classification.
type ClassifyInputs struct {
	BatchNumber int
	BatchSlug   string
	ReportPath  string
	Report      *Report // Non-nil always wins; nil when caller found none
	// TurnEnded reports whether the strand's newest turn end counts.
	TurnEnded    bool
	StrandLive   bool // True if fork's reed strand is still live
	Elapsed      time.Duration
	BatchTimeout time.Duration
}

// Classify decides a batch's classification from ins, checking the signals in this order:
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
