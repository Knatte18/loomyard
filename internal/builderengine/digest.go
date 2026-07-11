// digest.go implements Digest and Distill, the pinned terse contract
// poll's terminal classification returns to the orchestrator (the
// discussion's "Digest contract" decision): exactly the decision fields
// plus what Go cannot cheaply compute itself — no prose, no file lists
// beyond the drift paths — the mill-go-bloat lesson made structural.
// Distill handles the report-present (done/stuck) branch; the
// running/dead branches poll's own terminal-classification logic builds
// separately are out of this batch's scope.

package builderengine

import (
	"sort"
	"strings"
)

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

// Digest is poll's terminal-state output to the orchestrator: the pinned
// terse field set. A "running" snapshot carries only {batch, status,
// elapsed_s}; the remaining fields are populated only once a batch reaches
// a terminal classification.
type Digest struct {
	// Batch is the batch's NN-<batch-slug> identifier.
	Batch string `json:"batch"`
	// Status is one of DigestStatusRunning, DigestStatusDone,
	// DigestStatusStuck, or DigestStatusDead.
	Status string `json:"status"`
	// Tests is the report's green/red/skipped verdict; empty for a running
	// or dead snapshot.
	Tests string `json:"tests,omitempty"`
	// StuckReason is the report's stuck_reason, verbatim, when Status is
	// DigestStatusStuck.
	StuckReason string `json:"stuck_reason,omitempty"`
	// OutOfScope is the report's out_of_scope list, verbatim (path + one-
	// line why) — read from the report, never recomputed.
	OutOfScope []OutOfScopeEntry `json:"out_of_scope,omitempty"`
	// DriftUnreported is every changed file that falls outside every scope
	// entry AND is not named by any OutOfScope entry: the rot signal.
	// Paths only, sorted — never a full changed-file list.
	DriftUnreported []string `json:"drift_unreported,omitempty"`
	// FilesChanged is the count of files git reports changed since the
	// batch's start SHA — a count, never a list, so it does not scale with
	// batch size.
	FilesChanged int `json:"files_changed"`
	// Dirty reports whether the worktree had uncommitted or untracked
	// changes at terminal classification — a half-done-work signal.
	Dirty bool `json:"dirty"`
	// DeadReason is set only when Status is DigestStatusDead: one of
	// DeadReasonAsking, DeadReasonTimeout, or DeadReasonDied.
	DeadReason string `json:"dead_reason,omitempty"`
	// ElapsedS is the number of seconds since spawn, populated only on a
	// running snapshot.
	ElapsedS int `json:"elapsed_s,omitempty"`
}

// Distill computes the terminal digest for a batch whose batch-report has
// landed. report's decision fields (Status, Tests, StuckReason,
// OutOfScope) pass straight through — OutOfScope is read from
// report.OutOfScope, never a separate parameter. changed is compared
// against scope (prefix semantics, "/"-separated path boundaries — a
// directory scope entry covers everything under it, but "internal/foo"
// never covers "internal/foobar") to classify every changed file as
// in-scope, justified out-of-scope (named by report.OutOfScope), or
// unreported drift; DriftUnreported carries the sorted unreported set.
// FilesChanged is len(changed); dirty passes straight through to Dirty.
func Distill(report *Report, changed []string, scope []string, dirty bool) Digest {
	justified := make(map[string]bool, len(report.OutOfScope))
	for _, e := range report.OutOfScope {
		justified[e.Path] = true
	}

	var drift []string
	for _, f := range changed {
		if inScope(f, scope) || justified[f] {
			continue
		}
		drift = append(drift, f)
	}
	sort.Strings(drift)

	return Digest{
		Batch:           report.Batch,
		Status:          report.Status,
		Tests:           report.Tests,
		StuckReason:     report.StuckReason,
		OutOfScope:      report.OutOfScope,
		DriftUnreported: drift,
		FilesChanged:    len(changed),
		Dirty:           dirty,
	}
}

// inScope reports whether file falls under any of scope's prefix entries.
func inScope(file string, scope []string) bool {
	for _, entry := range scope {
		if pathCovers(entry, file) {
			return true
		}
	}
	return false
}

// pathCovers reports whether prefix covers path under "/"-separated
// boundary semantics: an exact match, or path continuing with
// prefix + "/". A raw string-prefix comparison would wrongly let
// "internal/foo" cover "internal/foobar"; this does not.
func pathCovers(prefix, path string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}
