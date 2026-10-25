// digest.go implements webster's own state Digest — the batch-outcome snapshot internal/webstercli
// persists into state.json and the resume trail (consumed there via a digestFields adapter, wired
// in batch 9) — and distill, which computes a terminal Done/Stuck digest from a fork's Report.
// webster's Digest carries the fork-return facts
// (status, head SHA) and the scope guard's informational deviation list, plus recovery metadata, and drops any
// out_of_scope/DriftUnreported/scope-drift model wholesale, since the flat plan format has no `##
// Scope` for a changed file to drift against.

package websterengine

import "fmt"

// The six legal Digest.Status values.
// failed is a batch whose report webster rejected on its merits.
// waiting is a batch stopped on a step only the run's parent can take; it is in flight, never terminal.
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

// Digest is webster's batch-outcome snapshot recorded into state.json and resume trail.
// Running snapshots carry only Batch, Status, and ElapsedS;
// other fields populate at terminal.
type Digest struct {
	Batch      string   `json:"batch"`
	Status     string   `json:"status"` // DigestStatusRunning, Done, Stuck, Dead, Failed, or Waiting
	HeadSHA    string   `json:"head_sha,omitempty"`
	Deviations []string `json:"deviations,omitempty"` // The scope guard's files changed outside the batch's targets; informational only, never affects Status
	Departures []string `json:"departures,omitempty"` // The Pattern-Departure trailers of the batch's commits as "<subject>: <value>"; informational only, never affects Status
	// Renames are the glyph renames the batch's report lists, which the post-batch pass applies to the cards not yet begun.
	Renames    []ReportRename `json:"renames,omitempty"`
	DeadReason string         `json:"dead_reason,omitempty"` // Set only when Status is Dead
	Reasons    []string       `json:"reasons,omitempty"`     // Set only when Status is Failed
	Need       string         `json:"need,omitempty"`        // The step only the parent can take; set only when Status is Waiting
	ElapsedS   int            `json:"elapsed_s,omitempty"`   // Seconds since spawn, running snapshots only
}

// batchDepartures returns the departure lines of every non-merge commit a batch made, from startSHA to headSHA.
// An empty startSHA, a record from before the field, yields none.
// A commit before startSHA, or one a parent merge-in brought in, is never attributed to the batch.
// A failed read is returned with a transient way forward, before any batch is marked terminal.
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

// distill computes the digest for a fork-return report:
// OK → Done, FAILED → Stuck, NEEDS_PARENT → Waiting with its Need. HeadSHA and Renames carry through.
// Batch is not set; the caller (Classify) assigns it.
// Deviations is not set either: record-batch fills it from the scope guard.
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
