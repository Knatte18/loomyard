// digest.go defines webster's batch Digest, the snapshot of one batch's outcome.
// The run records each Digest in state.json and the resume trail.
// distill derives a terminal Digest from a fork's Report.

package websterengine

import "fmt"

// The six legal Digest.Status values.
// DigestStatusFailed marks a batch whose report webster rejected on its merits.
// DigestStatusWaiting marks a batch stopped on a step only the run's parent can take; it is in flight, never terminal.
const (
	DigestStatusRunning = "running"
	DigestStatusDone    = "done"
	DigestStatusStuck   = "stuck"
	DigestStatusDead    = "dead"
	DigestStatusFailed  = "failed"
	DigestStatusWaiting = "waiting"
)

// The three legal Digest.DeadReason values, set only when Status is DigestStatusDead.
const (
	DeadReasonAsking  = "asking"
	DeadReasonTimeout = "timeout"
	DeadReasonDied    = "died"
)

// Digest is the batch-outcome snapshot that webster records in state.json and the resume trail.
// Running snapshots carry only Batch, Status, and ElapsedS;
// the other fields populate once the batch is terminal.
type Digest struct {
	Batch      string   `json:"batch"`
	Status     string   `json:"status"` // DigestStatusRunning, Done, Stuck, Dead, Failed, or Waiting
	HeadSHA    string   `json:"head_sha,omitempty"`
	Deviations []string `json:"deviations,omitempty"` // The scope guard's files changed outside the batch's targets; informational only, never affects Status
	Departures []string `json:"departures,omitempty"` // The Pattern-Departure trailers of the batch's commits as "<subject>: <value>"; informational only, never affects Status
	// Renames are the glyph renames the batch's report lists; the post-batch pass applies them to the cards not yet begun.
	Renames    []ReportRename `json:"renames,omitempty"`
	DeadReason string         `json:"dead_reason,omitempty"` // Set only when Status is Dead
	Reasons    []string       `json:"reasons,omitempty"`     // Set only when Status is Failed
	Need       string         `json:"need,omitempty"`        // The step only the parent can take; set only when Status is Waiting
	ElapsedS   int            `json:"elapsed_s,omitempty"`   // Seconds since spawn, running snapshots only
}

// batchDepartures returns the departure lines of every non-merge commit a batch made, from startSHA to headSHA.
// An empty startSHA, from a record written before the field existed, yields none.
// A commit before startSHA, or one a parent merge-in brought in, is never attributed to the batch.
// A failed read is returned as an error with a transient way forward, before any batch is marked terminal.
func batchDepartures(geom Geometry, startSHA, headSHA string) ([]string, error) {
	if startSHA == "" {
		return nil, nil
	}
	entries, err := geom.git().CommitLog(geom.WorktreeRoot, startSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("webster: read the commits of %s..%s for the digest's departures: %w; way forward: transient, re-run the verb", startSHA, headSHA, err)
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.DepartureLines()...)
	}
	return lines, nil
}

// distill computes the digest for a fork-return report.
// An OK report maps to Done, a FAILED report to Stuck, and a NEEDS_PARENT report to Waiting with its Need.
// HeadSHA and Renames carry through.
// The caller sets Batch, and the scope guard fills Deviations afterwards.
func distill(r *Report) Digest {
	switch r.Status {
	case ReportStatusOK:
		return Digest{Status: DigestStatusDone, HeadSHA: r.HeadSHA, Renames: r.Renames}
	case ReportStatusNeedsParent:
		return Digest{Status: DigestStatusWaiting, HeadSHA: r.HeadSHA, Renames: r.Renames, Need: r.Need}
	default:
		return Digest{Status: DigestStatusStuck, HeadSHA: r.HeadSHA, Renames: r.Renames}
	}
}
