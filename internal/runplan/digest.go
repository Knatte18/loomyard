// digest.go defines the batch Digest, the snapshot of one batch's outcome.
// The run records each Digest in its state file and the resume trail.
// Distill derives a terminal Digest from a batch's Report.

package runplan

import "fmt"

// The six legal Digest.Status values.
// DigestStatusFailed marks a batch whose report the implementer rejected on its merits.
// DigestStatusWaiting marks a batch stopped on a step or an answer only the run's parent can give;
// it is in flight, never terminal.
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

// Digest is the batch-outcome snapshot the implementer records in its state file and the resume trail.
// Running snapshots carry only Batch, Status, and ElapsedS;
// the other fields populate once the batch is terminal.
type Digest struct {
	Batch string `json:"batch"`
	// Status is DigestStatusRunning, Done, Stuck, Dead, Failed, or Waiting.
	Status  string `json:"status"`
	HeadSHA string `json:"head_sha,omitempty"`
	// Deviations are the scope guard's files changed outside the batch's targets; informational only, never affects Status.
	Deviations []string `json:"deviations,omitempty"`
	// Departures are the Pattern-Departure trailers of the batch's commits as "<subject>: <value>";
	// informational only, never affects Status.
	Departures []string `json:"departures,omitempty"`
	// Renames are the glyph renames the batch's report lists; the post-batch pass applies them to the cards not yet begun.
	Renames []ReportRename `json:"renames,omitempty"`
	// DeadReason is set only when Status is Dead.
	DeadReason string `json:"dead_reason,omitempty"`
	// LastMessage is the final message, on one line, of a strand that ended its turn without a report.
	// It is set only when DeadReason is DeadReasonAsking and the turn end carried a message.
	LastMessage string `json:"last_message,omitempty"`
	// Reasons is set only when Status is Failed.
	Reasons []string `json:"reasons,omitempty"`
	// Need is the step only the parent can take or the question only it can answer; set only when Status is Waiting.
	Need string `json:"need,omitempty"`
	// ElapsedS is the seconds since spawn, for running snapshots only.
	ElapsedS int `json:"elapsed_s,omitempty"`
}

// BatchDepartures returns the departure lines of every non-merge commit a batch made, from startSHA to headSHA.
// An empty startSHA, from a record written before the field existed, yields none.
// A commit before startSHA, or one a parent merge-in brought in, is never attributed to the batch.
// A failed read is returned as an error with a transient way forward, before any batch is marked terminal.
func BatchDepartures(geom Geometry, startSHA, headSHA string) ([]string, error) {
	if startSHA == "" {
		return nil, nil
	}
	entries, err := geom.Git.CommitLog(geom.WorktreeRoot, startSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("runplan: read the commits of %s..%s for the digest's departures: %w; way forward: transient, re-run the verb", startSHA, headSHA, err)
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.DepartureLines()...)
	}
	return lines, nil
}

// Distill computes the digest for a batch report.
// An OK report maps to Done, a FAILED report to Stuck, and a NEEDS_PARENT report to Waiting with its Need.
// HeadSHA and Renames carry through.
// The caller sets Batch, and the scope guard fills Deviations afterwards.
func Distill(r *Report) Digest {
	switch r.Status {
	case ReportStatusOK:
		return Digest{Status: DigestStatusDone, HeadSHA: r.HeadSHA, Renames: r.Renames}
	case ReportStatusNeedsParent:
		return Digest{Status: DigestStatusWaiting, HeadSHA: r.HeadSHA, Renames: r.Renames, Need: r.Need}
	default:
		return Digest{Status: DigestStatusStuck, HeadSHA: r.HeadSHA, Renames: r.Renames}
	}
}
