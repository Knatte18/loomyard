// audit.go implements webster's own fail-loud policy over the provider-invariant
// shuttleengine.ForkAudit/ForkReport fact shapes: the violation classes a forked
// implementer or Master's own parent session can trigger, and the weft-reference
// matcher both checks share. Unlike burlerengine's read-only cluster-round policy
// (a fork reviewer must never mutate anything), webster's forks are implementers —
// Write/Edit and host-repo git are the whole point of a batch, so CheckFork bans
// only nesting (Agent calls), weft references, and writes to the run's two contract
// files (outcome.yaml/summary.md — Master's alone), never batch writes. Master's own parent
// transcript is the mirror image: writes are banned everywhere EXCEPT the run's two
// contract files, since a Master that "helpfully" implements a batch itself or
// hand-writes a batch report defeats the fork-audit design as silently as a named
// spawn does. Every function here is pure — facts in, verdict out — per the
// discussion's TDD-centre framing; all transcript reading stays in claudeengine, per
// the Shuttle Provider-Seam Invariant.

package websterengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

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
	// ClassForkContractWrite means a fork's own transcript wrote one of the run's
	// two contract files (outcome.yaml, summary.md) — the exact mirror-image hole
	// of ClassParentWrite: those files are MASTER's only permitted writes, and a
	// fork writing them forges the run's own terminal judgment (observed live in
	// round fable-r3: a misidentifying fork overwrote outcome.yaml with a forged
	// "stuck" mid-run).
	ClassForkContractWrite AuditViolationClass = "fork-contract-write"
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
// read-only cluster-reviewer policy, which hard-bans any fork write). Three hard
// violations remain for a fork: any attempted Agent call (forks cannot nest, even a
// denied attempt — same posture as burler's nested-Agent ban), any write landing on
// one of the run's two contract files, outcomePath or summaryPath (those are
// MASTER's only permitted writes — a fork writing them forges the run's own
// terminal judgment; each WritePaths entry is canonicalized via resolveWritePath
// against workdir, exactly as CheckParent canonicalizes ParentWrites), and any Bash
// command matching weftRef (an implementer fork must never touch weft; weft sync is
// webstercli's own in-process job, per the Weft Git Invariant).
func CheckFork(f shuttleengine.ForkReport, outcomePath, summaryPath, workdir string, weftRef *regexp.Regexp) []AuditViolation {
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

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	for _, w := range f.WritePaths {
		cw := resolveWritePath(workdir, w)
		if cw == cleanOutcome || cw == cleanSummary {
			violations = append(violations, AuditViolation{
				Class:          ClassForkContractWrite,
				TranscriptPath: f.TranscriptPath,
				Detail:         fmt.Sprintf("fork wrote %q — outcome.yaml and summary.md are Master's own contract files; a fork writing either forges the run's terminal judgment", w),
			})
		}
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

// resolveWritePath canonicalizes one transcript-recorded write path for
// comparison against the run's absolute contract paths: cleaned, and — when the
// transcript recorded a RELATIVE path — resolved against workdir, the pane's
// working directory every agent's relative tool paths are anchored at. The
// transcript records whatever file_path string the agent passed to its Write
// tool, and agents freely mix absolute and relative spellings for the same file
// (observed live in round fable-r3: one Master wrote "_lyx/webster/outcome.yaml",
// the next wrote the absolute path — the unresolved comparison failed the first
// run's exit audit with a false parent-write violation).
func resolveWritePath(workdir, path string) string {
	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	return filepath.Join(workdir, cleaned)
}

// CheckParent evaluates Master's own parent-session facts against webster's Master
// policy: the mirror image of CheckFork's write posture. A fork MUST write to
// implement its batch; Master must NOT — except for the run's two contract files,
// outcomePath and summaryPath (_lyx/webster/outcome.yaml and _lyx/webster/
// summary.md), since a blanket write ban would break the outcome/summary contract
// itself. workdir is the pane's working directory (layout.Cwd — the same dir the
// fork audit keys transcripts on); each ParentWrites entry is canonicalized via
// resolveWritePath before comparing, so a relative-vs-absolute or "./" spelling
// difference between the transcript's raw entry and the caller-supplied absolute
// contract paths never false-positives. Three hard violations:
// any named spawn (silent context loss — same posture as burlerengine's
// NamedSpawns check), any parent write outside the two contract files (Master
// implementing a batch itself, or hand-writing a batch report — the same
// silent-quality-degradation class as a named spawn), and any parent Bash command
// matching weftRef (Master never drives weft directly; weftengine.Commit/Push run
// in-process inside webstercli's verbs, per the Weft Git Invariant).
func CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath, workdir string, weftRef *regexp.Regexp) []AuditViolation {
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
		cw := resolveWritePath(workdir, w)
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

// ErrNoForkTranscripts is the sentinel ClassifyAttribution's zero-new-transcript
// case wraps. record-batch's caller (batch 5) issues this as its own hard error
// AFTER SettleRetry's settle window is exhausted — never on the first miss, since
// a fork's transcript file may not have flushed to disk yet the instant the Agent
// tool call returns (the discussion's "first miss is inconclusive" flush-timing
// caveat). A report file existing alongside zero new transcripts does NOT save the
// batch from this error: a report with no fork behind it means Master wrote it
// itself, which is exactly the defect this check exists to catch (pinned check
// order: transcript count is decided BEFORE report presence).
var ErrNoForkTranscripts = errors.New("zero new fork transcripts since the previous batch boundary — the batch was never forked")

// DefaultSettleWindow is SettleRetry's recommended total wait budget before its
// caller gives up and treats a zero-transcript result as final: a few seconds is
// enough slack for Claude Code to flush a just-returned fork's
// subagents/<id>.jsonl to disk without meaningfully slowing down a normal
// record-batch call (which almost always finds its transcript on the first scan).
const DefaultSettleWindow = 3 * time.Second

// DefaultSettleTick is SettleRetry's recommended re-scan interval within the
// settle window: frequent enough that a normal (non-degenerate) call resolves
// within one or two ticks of the transcript actually appearing.
const DefaultSettleTick = 250 * time.Millisecond

// Sleeper abstracts time.Sleep so SettleRetry's wait loop never blocks for real
// under test — a recording fake Sleeper lets a test assert exactly how many ticks
// were requested and drive SettleRetry's retry loop to completion instantly,
// mirroring the clock seam builderengine's PollUntilTerminal and shuttleengine's
// wait.go already establish for the same reason.
type Sleeper interface {
	// Sleep blocks (in production) or records a request to block (under test)
	// for d.
	Sleep(d time.Duration)
}

// NewTranscripts returns the ForkReport entries in audit whose TranscriptPath is
// NOT a member of seen. It is a defensive re-filter that runs even when the
// engine's own AuditForksIncremental already excluded seen transcripts (see
// shuttleengine.Engine.AuditForksIncremental) — a caller that assembled audit from
// AuditForks (the full, non-incremental read) instead, or that is re-deriving
// attribution after a settle retry re-fetched everything, still gets the correct
// new-since-seen set either way.
func NewTranscripts(audit shuttleengine.ForkAudit, seen []string) []shuttleengine.ForkReport {
	seenSet := make(map[string]bool, len(seen))
	for _, path := range seen {
		seenSet[path] = true
	}

	var newReports []shuttleengine.ForkReport
	for _, f := range audit.Forks {
		if !seenSet[f.TranscriptPath] {
			newReports = append(newReports, f)
		}
	}
	return newReports
}

// SettleRetry re-invokes fetch on tick's cadence, sleeping via s between attempts,
// until at least one transcript new since seen appears or window elapses —
// implementing the flush-timing de-risk from discussion.md's fork-audit-policy
// decision ("first miss is inconclusive"): the zero-transcript hard error is only
// ever issued by the CALLER, and only after SettleRetry itself has exhausted the
// settle window, never on this function's first fetch. SettleRetry returns as soon
// as fetch reports one or more new transcripts — it never sleeps out the rest of
// window once it has an answer. A fetch error propagates immediately: an audit
// read that itself failed has nothing safe to retry against. When window elapses
// with zero new transcripts, SettleRetry returns the last fetched audit, a nil (or
// empty) newReports slice, and a nil error — it is the caller's job (see
// ClassifyAttribution) to turn that empty result into ErrNoForkTranscripts.
func SettleRetry(
	fetch func() (shuttleengine.ForkAudit, error),
	seen []string,
	window time.Duration,
	tick time.Duration,
	s Sleeper,
) (shuttleengine.ForkAudit, []shuttleengine.ForkReport, error) {
	var elapsed time.Duration

	for {
		audit, err := fetch()
		if err != nil {
			return shuttleengine.ForkAudit{}, nil, err
		}

		newReports := NewTranscripts(audit, seen)
		if len(newReports) > 0 {
			return audit, newReports, nil
		}

		if elapsed >= window {
			return audit, newReports, nil
		}

		s.Sleep(tick)
		elapsed += tick
	}
}

// ClassifyAttribution pins the fork-audit-policy decision's check order over
// newReports, the transcripts SettleRetry (or a direct NewTranscripts call)
// determined are new since the previous batch boundary:
//
//  1. Zero new transcripts: hard error (ErrNoForkTranscripts), REGARDLESS of
//     whether a batch report file exists — a report with no fork behind it means
//     Master wrote it itself. Transcript-count-before-report-presence is what
//     makes that defect unfakeable; the caller must check this BEFORE it even
//     looks for a report file.
//  2. Exactly one new transcript: clean — the normal case. Returns ("", nil).
//  3. More than one new transcript: warning only, never hard — a fork whose
//     Agent call errored mid-flight followed by a direct re-fork, with no
//     record-batch call in between, is legitimate retry behavior, not a defect.
func ClassifyAttribution(newReports []shuttleengine.ForkReport) (warning string, err error) {
	switch len(newReports) {
	case 0:
		return "", ErrNoForkTranscripts
	case 1:
		return "", nil
	default:
		return fmt.Sprintf(
			"%d new fork transcripts since the previous batch boundary — expected exactly one; treating as a fork-error-then-re-fork with no intervening record-batch call",
			len(newReports),
		), nil
	}
}
