// audit.go implements webster's own fail-loud policy over the provider-invariant
// shuttleengine.ForkAudit/ForkReport fact shapes: the violation classes a forked
// implementer or Master's own parent session can trigger, and the weft-reference
// matcher both checks share. Unlike burlerengine's read-only cluster-round policy
// (a fork reviewer must never mutate anything), webster's forks are implementers —
// Write/Edit and host-repo git are the whole point of a batch, so CheckFork only
// bans nesting (Agent calls) and weft references, never writes. Master's own parent
// transcript is the mirror image: writes are banned everywhere EXCEPT the run's two
// contract files, since a Master that "helpfully" implements a batch itself or
// hand-writes a batch report defeats the fork-audit design as silently as a named
// spawn does. Every function here is pure — facts in, verdict out — per the
// discussion's TDD-centre framing; all transcript reading stays in claudeengine, per
// the Shuttle Provider-Seam Invariant.

package websterengine

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AuditViolationClass discriminates the fail-loud violation classes CheckFork and
// CheckParent can report. It exists so a caller (e.g. record-batch's error message,
// or a future metrics hook) can branch on WHAT kind of violation occurred without
// parsing Detail's free-text prose.
type AuditViolationClass string

// The set of hard violation classes CheckFork and CheckParent can report.
const (
	// ClassNestedAgent means a fork's own transcript attempted an Agent tool call —
	// forks cannot nest, even when Claude Code denied the attempt.
	ClassNestedAgent AuditViolationClass = "nested-agent"
	// ClassWeftReference means a Bash command (fork or parent) invoked lyx weft/lyx
	// warp, or referenced the weft worktree path — agents never touch weft directly.
	ClassWeftReference AuditViolationClass = "weft-reference"
	// ClassNamedSpawn means Master's parent transcript recorded one or more Agent
	// calls carrying a name parameter — named forks silently lose inherited context.
	ClassNamedSpawn AuditViolationClass = "named-spawn"
	// ClassParentWrite means Master's parent transcript wrote a file other than the
	// run's two contract files (outcome.yaml, summary.md) — a Master implementing
	// batches itself or hand-writing a batch report.
	ClassParentWrite AuditViolationClass = "parent-write"
)

// AuditViolation is one hard fork-audit policy violation observed in either a
// fork's own transcript or Master's parent transcript. Every hard violation is an
// error — the fail-loud posture treats a detected violation as build-breaking, not
// merely logged — so AuditViolation implements the error interface directly and a
// caller can return a []AuditViolation entry (or an aggregate wrapping them) as an
// ordinary Go error. TranscriptPath is the fork's TranscriptPath for a CheckFork
// violation, or "" for a CheckParent violation (ForkAudit carries no path for
// Master's own parent transcript — webster tracks Master's session ID separately,
// in State.MasterSessionID).
type AuditViolation struct {
	Class          AuditViolationClass
	TranscriptPath string
	Detail         string
}

// Error implements the error interface, formatting the violation as a single-line,
// webster-prefixed message a caller can wrap or surface verbatim.
func (v AuditViolation) Error() string {
	if v.TranscriptPath == "" {
		return fmt.Sprintf("webster: %s violation: %s", v.Class, v.Detail)
	}
	return fmt.Sprintf("webster: %s violation in %q: %s", v.Class, v.TranscriptPath, v.Detail)
}

// weftReferencePattern builds the regexp CheckFork and CheckParent use to detect a
// Bash command that touches weft: an invocation of `lyx weft` or `lyx warp`, or any
// command referencing the weft worktree path (e.g. `git -C <weft-worktree> add`,
// `cd <weft-worktree> && ...`). It is built at runtime from layout.WeftWorktree()
// (this run's own weft sibling path) and the exported hubgeometry.WeftSuffix
// constant (so any OTHER weft-suffixed path an agent might reference — not just this
// run's own — is caught too), NEVER from a "-weft" string literal in this package:
// a literal here would trip TestEnforcement_GeometryLiterals, which bans every
// geometry-path token outside internal/hubgeometry.
func weftReferencePattern(layout *hubgeometry.Layout) *regexp.Regexp {
	weftPath := regexp.QuoteMeta(layout.WeftWorktree())
	weftSuffix := regexp.QuoteMeta(hubgeometry.WeftSuffix)
	pattern := fmt.Sprintf(
		`lyx\s+weft\b|lyx\s+warp\b|%s|\S*%s\b`,
		weftPath, weftSuffix,
	)
	return regexp.MustCompile(pattern)
}

// CheckFork evaluates one fork's transcript facts against webster's implementer
// policy: Write/Edit and host-repo git are explicitly ALLOWED (a batch's per-card
// commits are the whole implementer contract — the opposite of burlerengine's
// read-only cluster-reviewer policy, which hard-bans any fork write). Only two hard
// violations remain for a fork: any attempted Agent call (forks cannot nest, even a
// denied attempt — same posture as burler's nested-Agent ban) and any Bash command
// matching weftRef (an implementer fork must never touch weft; weft sync is
// webstercli's own in-process job, per the Weft Git Invariant).
func CheckFork(f shuttleengine.ForkReport, weftRef *regexp.Regexp) []AuditViolation {
	var violations []AuditViolation

	if f.AgentCalls > 0 {
		violations = append(violations, AuditViolation{
			Class:          ClassNestedAgent,
			TranscriptPath: f.TranscriptPath,
			Detail: fmt.Sprintf(
				"attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied",
				f.AgentCalls,
			),
		})
	}

	for _, cmd := range f.BashCommands {
		if weftRef.MatchString(cmd) {
			violations = append(violations, AuditViolation{
				Class:          ClassWeftReference,
				TranscriptPath: f.TranscriptPath,
				Detail:         fmt.Sprintf("ran a weft-referencing command (%q) — an implementer fork must never touch weft directly", cmd),
			})
		}
	}

	return violations
}

// CheckParent evaluates Master's own parent-session facts against webster's Master
// policy: the mirror image of CheckFork's write posture. A fork MUST write to
// implement its batch; Master must NOT — except for the run's two contract files,
// outcomePath and summaryPath (_lyx/webster/outcome.yaml and _lyx/webster/
// summary.md), since a blanket write ban would break the outcome/summary contract
// itself. Paths are compared after filepath.Clean on both sides, so a
// slash/separator or "./" difference between a's raw ParentWrites entry and the
// caller-supplied contract paths never false-positives. Three hard violations:
// any named spawn (silent context loss — same posture as burlerengine's
// NamedSpawns check), any parent write outside the two contract files (Master
// implementing a batch itself, or hand-writing a batch report — the same
// silent-quality-degradation class as a named spawn), and any parent Bash command
// matching weftRef (Master never drives weft directly; weftengine.Commit/Push run
// in-process inside webstercli's verbs, per the Weft Git Invariant).
func CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath string, weftRef *regexp.Regexp) []AuditViolation {
	var violations []AuditViolation

	if a.NamedSpawns > 0 {
		violations = append(violations, AuditViolation{
			Class: ClassNamedSpawn,
			Detail: fmt.Sprintf(
				"%d fork(s) were spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory",
				a.NamedSpawns,
			),
		})
	}

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	for _, w := range a.ParentWrites {
		cw := filepath.Clean(w)
		if cw != cleanOutcome && cw != cleanSummary {
			violations = append(violations, AuditViolation{
				Class:  ClassParentWrite,
				Detail: fmt.Sprintf("Master wrote %q — Master may write only its two contract files (outcome.yaml and summary.md); any other write means Master implemented a batch itself or hand-wrote a batch report", w),
			})
		}
	}

	for _, cmd := range a.ParentBashCommands {
		if weftRef.MatchString(cmd) {
			violations = append(violations, AuditViolation{
				Class:  ClassWeftReference,
				Detail: fmt.Sprintf("ran a weft-referencing command (%q) — Master must never touch weft directly; weft sync is webstercli's own in-process job", cmd),
			})
		}
	}

	return violations
}

// ForkWarnings evaluates f for webster's warning-only (never round-failing)
// classes: a fork that never returned a final report is sloppiness no mechanism
// can prevent in advance (the fork ran clean but never delivered its findings), so
// it is collected as a warning rather than a hard violation — the same posture
// burlerengine's auditClusterRound takes for its own ReportReturned == false case.
func ForkWarnings(f shuttleengine.ForkReport) []string {
	if !f.ReportReturned {
		return []string{fmt.Sprintf("fork %q never returned a final report", f.TranscriptPath)}
	}
	return nil
}
