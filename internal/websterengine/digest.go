// digest.go implements webster's own state Digest — the batch-outcome
// snapshot internal/webstercli persists into state.json and the resume
// trail (consumed there via a digestFields adapter, wired in batch 9) — and
// distill, which computes a terminal Done/Stuck digest from a fork's
// Report. This replaces builderengine.Digest/Distill entirely: webster
// carries the fork-return facts (status, head SHA, informational
// deviation list) plus recovery metadata, and drops builder's
// out_of_scope/DriftUnreported/scope-drift model wholesale, since the flat
// plan format has no `## Scope` for a changed file to drift against.

package websterengine

// The four legal Digest.Status values.
const (
	DigestStatusRunning = "running"
	DigestStatusDone    = "done"
	DigestStatusStuck   = "stuck"
	DigestStatusDead    = "dead"
)

// The three legal Digest.DeadReason values, set only when Status is
// DigestStatusDead.
const (
	DeadReasonAsking  = "asking"
	DeadReasonTimeout = "timeout"
	DeadReasonDied    = "died"
)

// Digest is webster's batch-outcome snapshot: the fork-return facts
// (status, head SHA, the informational deviation list) plus recovery
// metadata, recorded into state.json and the resume trail. A "running"
// snapshot carries only Batch, Status, and ElapsedS; the remaining fields
// populate only once a batch reaches a terminal classification.
type Digest struct {
	// Batch is the batch's NN-<batch-slug> identifier.
	Batch string `json:"batch"`
	// Status is one of DigestStatusRunning, DigestStatusDone,
	// DigestStatusStuck, or DigestStatusDead.
	Status string `json:"status"`
	// HeadSHA is the fork's reported Report.HeadSHA, carried through
	// verbatim; empty for a running or dead snapshot.
	HeadSHA string `json:"head_sha,omitempty"`
	// Deviations is the fork's reported Report.Deviations, carried through
	// verbatim. It is always informational — see the deviation-list-is-
	// informational Shared Decision — and never affects Status.
	Deviations []string `json:"deviations,omitempty"`
	// DeadReason is set only when Status is DigestStatusDead: one of
	// DeadReasonAsking, DeadReasonTimeout, or DeadReasonDied.
	DeadReason string `json:"dead_reason,omitempty"`
	// ElapsedS is the number of seconds since spawn, populated only on a
	// running snapshot.
	ElapsedS int `json:"elapsed_s,omitempty"`
}

// distill computes the terminal digest for a batch whose fork-return report
// has landed: r.Status maps OK -> DigestStatusDone / FAILED ->
// DigestStatusStuck, r.HeadSHA carries straight through, and r.Deviations
// (the fork-reported, informational deviation list) carries straight
// through as Deviations. changed is changedFiles' optional Master
// cross-check of the same deviation signal — per the deviation-list-is-
// informational Shared Decision it is recorded nowhere on the digest and
// never inspected here beyond being accepted as a parameter, since the
// digest's own Deviations field is always the fork's own report, not a
// recomputation; a caller wanting a cross-check reads changed itself,
// before or after calling distill. distill never sets Batch — the caller
// (Classify) composes and assigns the NN-<batch-slug> identifier, since a
// fork Report carries no batch field of its own.
func distill(r *Report, changed []string) Digest {
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
