// digest.go implements webster's own state Digest — the batch-outcome snapshot internal/webstercli
// persists into state.json and the resume trail (consumed there via a digestFields adapter, wired
// in batch 9) — and distill, which computes a terminal Done/Stuck digest from a fork's Report.
// This replaces builderengine.Digest/Distill entirely: webster carries the fork-return facts
// (status, head SHA, informational deviation list) plus recovery metadata, and drops builder's
// out_of_scope/DriftUnreported/scope-drift model wholesale, since the flat plan format has no `##
// Scope` for a changed file to drift against.

package websterengine

// The four legal Digest.Status values.
const (
	DigestStatusRunning = "running"
	DigestStatusDone    = "done"
	DigestStatusStuck   = "stuck"
	DigestStatusDead    = "dead"
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
	Status     string   `json:"status"` // DigestStatusRunning, Done, Stuck, or Dead
	HeadSHA    string   `json:"head_sha,omitempty"`
	Deviations []string `json:"deviations,omitempty"`  // Informational only, never affects Status
	DeadReason string   `json:"dead_reason,omitempty"` // Set only when Status is Dead
	ElapsedS   int      `json:"elapsed_s,omitempty"`   // Seconds since spawn, running snapshots only
}

// distill computes the terminal digest for a fork-return report:
// OK → Done, FAILED → Stuck. HeadSHA and Deviations carry through.
// Batch is not set; the caller (Classify) assigns it.
func distill(r *Report) Digest {
	status := DigestStatusStuck
	if r.Status == ReportStatusOK {
		status = DigestStatusDone
	}

	return Digest{
		Status:     status,
		HeadSHA:    r.HeadSHA,
		Deviations: r.Deviations,
	}
}
