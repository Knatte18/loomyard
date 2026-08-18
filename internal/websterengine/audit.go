// audit.go implements webster's own fail-loud policy over the provider-invariant
// shuttleengine.ForkAudit/ForkReport fact shapes: the violation classes a forked implementer or
// Master's own parent session can trigger,
// and the fabric-reference matcher both checks share.
// Unlike burlerengine's read-only cluster-round policy (a fork reviewer must never mutate
// anything), webster's forks are implementers — Write/Edit and repo-native git are the whole point of
// a batch, so CheckFork bans only nesting (Agent calls), fabric references, and writes to the run's
// two contract files (outcome.yaml/summary.md — Master's alone), never batch writes.
// Master's own parent transcript is the mirror image: writes are banned everywhere EXCEPT the run's
// two contract files, since a Master that "helpfully" implements a batch itself or hand-writes a
// batch report defeats the fork-audit design as silently as a named spawn does.
// Every function here is pure — facts in, verdict out — per the discussion's TDD-centre framing;
// all transcript reading stays in claudeengine, per the Shuttle Provider-Seam Invariant.

package websterengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RefMatcher is the narrow seam CheckFork and CheckParent consult for the fabric-reference violation
// class. *fabricengine.RefScanner satisfies it without any adapter, since that type already has the
// identical method.
type RefMatcher interface {
	// Matches reports whether cmd references fabric's two-checkout mechanism.
	Matches(cmd string) bool
}

// NeverMatches is the pinned RefMatcher supplier for a mode with no fabric repo at all — standalone
// mode's answer where hub mode supplies a real *fabricengine.RefScanner.
// It lives here, beside the interface it implements, rather than in a geometry package that has no
// business knowing webster's audit vocabulary, and it is a named exported type rather than an inline
// literal so every Deps-construction site shares one supplier instead of re-inventing it.
// CheckFork and CheckParent call Matches unguarded, so a nil RefMatcher is a panic on the first
// standalone record-batch — the field must therefore never be nil in either mode.
type NeverMatches struct{}

// Matches always returns false: NeverMatches never sees a fabric reference, because a mode that
// supplies it has no fabric repo to reference.
func (NeverMatches) Matches(string) bool {
	return false
}

// AuditViolationClass discriminates the fail-loud violation classes CheckFork and CheckParent can
// report.
// It exists so a caller (e.g.
// record-batch's error message,
// or a future metrics hook) can branch on WHAT kind of violation occurred without parsing Detail's
// free-text prose.
type AuditViolationClass string

// The set of hard violation classes CheckFork and CheckParent can report.
const (
	// ClassNestedAgent means a fork's own transcript attempted an Agent tool call — forks cannot nest,
	// even when Claude Code denied the attempt.
	ClassNestedAgent AuditViolationClass = "nested-agent"
	// ClassFabricReference means a Bash command (fork or parent) invoked lyx fabric (or an older,
	// pre-cutover single-word spelling of the same subcommand),
	// or referenced the fabric worktree path — agents never touch the fabric repo directly.
	ClassFabricReference AuditViolationClass = "fabric-reference"
	// ClassNamedSpawn means Master's parent transcript recorded one or more Agent calls carrying a
	// name parameter — named forks silently lose inherited context.
	ClassNamedSpawn AuditViolationClass = "named-spawn"
	// ClassParentWrite means Master's parent transcript wrote a file other than the run's two contract
	// files (outcome.yaml, summary.md) — a Master implementing batches itself or hand-writing a batch
	// report.
	ClassParentWrite AuditViolationClass = "parent-write"
	// ClassForkContractWrite means a fork's own transcript wrote one of the run's two contract files
	// (outcome.yaml, summary.md) — those are Master's only permitted writes, and a fork writing them
	// forges the run's terminal judgment.
	ClassForkContractWrite AuditViolationClass = "fork-contract-write"
)

// AuditViolation is one hard fork-audit policy violation observed in either a fork's own transcript
// or Master's parent transcript.
// Every hard violation is an error — the fail-loud posture treats a detected violation as
// build-breaking, not merely logged — so AuditViolation implements the error interface directly and
// a caller can return a []AuditViolation entry (or an aggregate wrapping them) as an ordinary Go
// error.
// TranscriptPath is the fork's TranscriptPath for a CheckFork violation,
// or "" for a CheckParent violation (ForkAudit carries no path for Master's own parent transcript —
// webster tracks Master's session ID separately, in State.MasterSessionID).
type AuditViolation struct {
	Class          AuditViolationClass
	TranscriptPath string
	Detail         string
}

// Error implements the error interface, formatting the violation as a single-line, webster-prefixed
// message a caller can wrap or surface verbatim.
func (v AuditViolation) Error() string {
	if v.TranscriptPath == "" {
		return fmt.Sprintf("webster: %s violation: %s", v.Class, v.Detail)
	}
	return fmt.Sprintf("webster: %s violation in %q: %s", v.Class, v.TranscriptPath, v.Detail)
}

// CheckFork evaluates one fork's transcript facts against webster's implementer policy: Write/Edit
// and repo-native git are explicitly allowed.
// It bans three hard violations: any attempted Agent call, any write to the two contract files
// (outcomePath or summaryPath), and any Bash command referencing the fabric repo.
func CheckFork(f shuttleengine.ForkReport, outcomePath, summaryPath, workdir string, fabricRef *fabricengine.RefScanner) []AuditViolation {
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
		if fabricRef.Matches(cmd) {
			violations = append(violations, AuditViolation{
				Class:          ClassFabricReference,
				TranscriptPath: f.TranscriptPath,
				Detail:         fmt.Sprintf("ran a fabric-referencing command (%q) — an implementer fork must never touch the fabric repo directly", cmd),
			})
		}
	}

	return violations
}

// resolveWritePath canonicalizes a transcript-recorded write path: cleaned,
// and — when relative — resolved against workdir. This prevents false matches
// when agents mix absolute and relative spellings for the same file.
func resolveWritePath(workdir, path string) string {
	cleaned := filepath.Clean(path)
	if isTranscriptPathAbsolute(path) {
		return cleaned
	}
	return filepath.Join(workdir, cleaned)
}

// isTranscriptPathAbsolute reports whether a transcript-recorded write path is
// already absolute — in either the running OS's native sense (stdlib
// filepath.IsAbs: a drive letter or UNC prefix on Windows) or POSIX-style (a
// leading "/"). Transcript-recorded write paths and this package's own
// workdir/contract-path arguments are always POSIX-style, regardless of the workdir's platform
// (they come from the pane's own working-directory convention, not a raw
// OS path) — so on Windows, stdlib filepath.IsAbs alone reports false for a
// path like "/hub/master-builder/_lyx/webster/outcome.yaml" (Windows requires
// a drive letter or UNC prefix to consider a path absolute), and
// resolveWritePath would incorrectly join an already-absolute path against
// workdir a second time, producing a path that can never match the caller's
// absolute contract paths.
func isTranscriptPathAbsolute(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/")
}

// CheckParent evaluates Master's own parent-session facts: the mirror image of CheckFork's policy.
// Master must NOT write except to the two contract files (outcomePath and summaryPath).
// It bans three hard violations: any named spawn, any parent write outside contract files, and any
// Bash command referencing the fabric repo.
func CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath, workdir string, fabricRef *fabricengine.RefScanner) []AuditViolation {
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
		if fabricRef.Matches(cmd) {
			violations = append(violations, AuditViolation{
				Class:  ClassFabricReference,
				Detail: fmt.Sprintf("ran a fabric-referencing command (%q) — Master must never touch the fabric repo directly; the fabric sync is webstercli's own in-process job", cmd),
			})
		}
	}

	return violations
}

// ForkWarnings evaluates f for webster's warning-only (never round-failing) classes: a fork that
// never returned a final report is sloppiness no mechanism can prevent in advance (the fork ran
// clean but never delivered its findings), so it is collected as a warning rather than a hard
// violation — the same posture burlerengine's auditClusterRound takes for its own ReportReturned ==
// false case.
func ForkWarnings(f shuttleengine.ForkReport) []string {
	if !f.ReportReturned {
		return []string{fmt.Sprintf("fork %q never returned a final report", f.TranscriptPath)}
	}
	return nil
}

// ErrNoForkTranscripts is the sentinel ClassifyAttribution's zero-new-transcript case wraps.
// record-batch's caller (batch 5) issues this as its own hard error AFTER SettleRetry's settle
// window is exhausted — never on the first miss, since a fork's transcript file may not have
// flushed to disk yet the instant the Agent tool call returns (the discussion's "first miss is
// inconclusive" flush-timing caveat).
// A report file existing alongside zero new transcripts does NOT save the batch from this error: a
// report with no fork behind it means Master wrote it itself, which is exactly the defect this
// check exists to catch (pinned check order: transcript count is decided BEFORE report presence).
// The message names the operator recourse because this error is one leg of a three-verb refusal
// circle with no in-band exit: with a report on disk but no fork transcript, begin-batch refuses
// (report exists), record-batch errors here, and recover-batch refuses an OK report — a state a
// forged report produces, but ALSO a legitimate cross-machine resume of the
// report-landed-before-record-batch crash window, since fork transcripts live under the
// machine-local ~/.claude projects dir while state.json and reports are fabric-synced (found live in
// crucible round fable-r1).
var ErrNoForkTranscripts = errors.New("zero new fork transcripts since the previous batch boundary — the batch was never forked (or its transcript is not on this machine: fork transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report; an operator resolves that by moving the batch's report file out of the reports dir and re-driving the batch)")

// DefaultSettleWindow is SettleRetry's recommended total wait budget before its caller gives up and
// treats a zero-transcript result as final: a few seconds is enough slack for Claude Code to flush
// a just-returned fork's subagents/<id>.jsonl to disk without meaningfully slowing down a normal
// record-batch call (which almost always finds its transcript on the first scan).
const DefaultSettleWindow = 3 * time.Second

// DefaultSettleTick is SettleRetry's recommended re-scan interval within the settle window:
// frequent enough that a normal (non-degenerate) call resolves within one or two ticks of the
// transcript actually appearing.
const DefaultSettleTick = 250 * time.Millisecond

// Sleeper abstracts time.Sleep so SettleRetry's wait loop never blocks for real under test — a
// recording fake Sleeper lets a test assert exactly how many ticks were requested and drive
// SettleRetry's retry loop to completion instantly, the same clock seam pattern
// shuttleengine's wait.go already establishes for the same reason.
type Sleeper interface {
	// Sleep blocks (in production) or records a request to block (under test)
	// for d.
	Sleep(d time.Duration)
}

// NewTranscripts returns the ForkReport entries in audit whose TranscriptPath is NOT in seen.
// It is a defensive re-filter for callers that may not have used AuditForksIncremental or that are
// re-deriving after a settle retry.
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

// SettleRetry re-invokes fetch on tick's cadence, sleeping via s between attempts, until at least
// one transcript new since seen appears or window elapses — implementing the flush-timing de-risk
// from discussion.md's fork-audit-policy decision ("first miss is inconclusive"): the
// zero-transcript hard error is only ever issued by the CALLER, and only after SettleRetry itself
// has exhausted the settle window, never on this function's first fetch.
// SettleRetry returns as soon as fetch reports one or more new transcripts — it never sleeps out
// the rest of window once it has an answer.
// A fetch error propagates immediately: an audit read that itself failed has nothing safe to retry
// against.
// When window elapses with zero new transcripts, SettleRetry returns the last fetched audit, a nil
// (or empty) newReports slice, and a nil error — it is the caller's job (see ClassifyAttribution)
// to turn that empty result into ErrNoForkTranscripts.
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
