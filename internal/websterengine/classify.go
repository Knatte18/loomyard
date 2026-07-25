// classify.go implements webster's own recovery-classification decision
// logic: a webster-local retarget of builderengine.Classify/ClassifyInputs
// (builderengine/poll.go) onto webster's Report/Digest types. The v2
// Scope field is dropped entirely — the flat plan format carries no
// `## Scope`, so there is nothing for a changed file to be judged against
// beyond the fork's own informational deviation list. The pinned decision
// order (report present wins outright; otherwise turn-ended, then
// timeout, then dead-pane, then running) is preserved byte-for-byte from
// builder's own Classify, since that ordering is a frozen, cross-checked
// invariant, not a webster-specific choice.

package websterengine

import (
	"fmt"
	"time"
)

// ClassifyInputs carries every signal Classify's decision needs.
type ClassifyInputs struct {
	// BatchNumber and BatchSlug together name the batch being classified,
	// used to compose the NN-<batch-slug> identifier Classify assigns to
	// every Digest it returns, terminal or not — a fork Report carries no
	// batch field of its own for distill to read.
	BatchNumber int
	BatchSlug   string

	// ReportPath is the fork-return report file path Classify's caller
	// checked for; it is informational only (Classify itself branches on
	// Report, never re-reads ReportPath), kept here so a caller building
	// ClassifyInputs has a natural place to record what it looked at.
	ReportPath string
	// Report is the parsed fork-return report when the caller found one,
	// nil when it did not. A non-nil Report always wins classification,
	// regardless of every other field's value.
	Report *Report

	// TurnEnded reports whether the fork's turn has ended (a Stop event
	// was observed) — see the package-level TurnEnded gatherer.
	TurnEnded bool
	// StrandLive reports whether the fork's mux strand is still live — see
	// the package-level StrandLive gatherer.
	StrandLive bool

	// Elapsed is the wall-clock duration since this batch's fork was
	// spawned.
	Elapsed time.Duration
	// BatchTimeout is the configured ceiling Elapsed is compared against.
	BatchTimeout time.Duration
}

// Classify decides a batch's classification from ins, in the pinned
// decision order, returning the Digest to report and whether that
// classification is terminal:
//
//  1. Report present: terminal, via distill(in.Report) with the composed
//     NN-<batch-slug> identifier assigned — status is done or stuck per
//     the report's own OK/FAILED status.
//  2. No report, TurnEnded: terminal dead, DeadReasonAsking — the fork
//     ended its turn without ever satisfying the file contract, which is
//     recovery material, same as a crash.
//  3. No report, Elapsed > BatchTimeout: terminal dead, DeadReasonTimeout.
//  4. No report, turn still in progress (TurnEnded false), strand pane
//     gone (StrandLive false): terminal dead, DeadReasonDied.
//  5. Otherwise: non-terminal running snapshot carrying only batch,
//     status, and ElapsedS.
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
