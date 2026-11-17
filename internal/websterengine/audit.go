// audit.go implements webster's fail-loud policy over the fork facts in shuttleengine.ForkAudit and ForkReport.
// Forks are implementers, so CheckFork allows Write/Edit and repo-native git.
// It bans nested Agent calls, fabric references, and writes to contract files, the plan, or run state.
// CheckParent guards Master's own parent transcript, which may write only the two contract files.
// Every function here is pure: facts in, verdict out. Transcript reading stays in claudeengine.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RefMatcher is the seam through which the fabric-reference violation class is detected.
// Matches is called without a nil check, so a nil RefMatcher panics.
type RefMatcher interface {
	// Matches reports whether cmd references fabric's two-checkout mechanism.
	Matches(cmd string) bool
}

// NeverMatches is the RefMatcher for a mode with no fabric repo, so no command can reference one.
// Being a named type, every construction site shares this one supplier.
type NeverMatches struct{}

// Matches always reports false, because a mode with no fabric repo has no fabric reference to match.
func (NeverMatches) Matches(string) bool {
	return false
}

// AuditViolationClass discriminates the fail-loud violation classes an audit can report.
// It lets a caller branch on the kind of violation without parsing the free text of Detail.
type AuditViolationClass string

// The hard violation classes an audit can report.
const (
	// ClassNestedAgent means a fork's transcript attempted an Agent tool call:
	// forks cannot nest, so the attempt is a violation even when Claude Code denied it.
	ClassNestedAgent AuditViolationClass = "nested-agent"
	// ClassFabricReference means a Bash command (fork or parent) invoked a lym fabric subcommand.
	// It also means a Bash command referenced the fabric worktree path.
	// Agents never touch the fabric repo directly.
	ClassFabricReference AuditViolationClass = "fabric-reference"
	// ClassNamedSpawn means Master's parent transcript recorded one or more Agent calls carrying a name parameter.
	// Named forks silently lose inherited context.
	ClassNamedSpawn AuditViolationClass = "named-spawn"
	// ClassParentWrite means Master's parent transcript wrote a file besides the two contract files.
	// Master may write only outcome.yaml and summary.md.
	// Any other write means Master is implementing a batch itself or hand-writing a batch report.
	ClassParentWrite AuditViolationClass = "parent-write"
	// ClassForkContractWrite means a fork's transcript wrote one of the run's two contract files.
	// Master alone may write those, so a fork writing one forges the run's terminal judgment.
	// A write to a contract file is reported only as this class, never also as a plan or state write.
	ClassForkContractWrite AuditViolationClass = "fork-contract-write"
	// ClassForkPlanWrite means a fork's transcript wrote under the run's plan directory, which only webster rewrites.
	// A fork writing there can change a later card or its own card's Verify.
	ClassForkPlanWrite AuditViolationClass = "fork-plan-write"
	// ClassForkStateWrite means a fork's transcript wrote under webster's run directory outside its own batch report.
	// Only webster writes the run state, so such a write can forge a batch record.
	ClassForkStateWrite AuditViolationClass = "fork-state-write"
)

// AuditViolation is one hard policy violation found in a fork's transcript or in Master's parent transcript.
// Every hard violation is an error, because the fail-loud posture treats it as build-breaking, not merely logged.
// AuditViolation implements the error interface, so a caller can return one as an ordinary Go error.
// TranscriptPath is the fork's transcript path for a fork violation, and empty for a parent violation.
// A parent violation has no transcript path because webster tracks Master's session ID in State.MasterSessionID.
// Key is the finding's deterministic identity.
// A fork key is `fork:<transcript path>:<class>:<ordinal>`, where the ordinal counts that class within the transcript.
// A parent key is `parent:<class>:<ordinal>`.
// Its ordinal indexes ParentWrites or ParentBashCommands, or runs from 1 to NamedSpawns for named-spawn findings.
// Path is the transcript-recorded write path for parent-write and fork-contract-write, empty for every other class.
// Command is the Bash command a fabric-reference finding matched, empty for every other class.
type AuditViolation struct {
	Class          AuditViolationClass
	TranscriptPath string
	Detail         string
	Key            string
	Path           string
	Command        string
}

// AuditSeverity classifies an audit finding as a correctness or a policy finding.
type AuditSeverity string

const (
	// AuditSeverityCorrectness marks a finding that can change the run's own state or the code under review.
	AuditSeverityCorrectness AuditSeverity = "correctness"
	// AuditSeverityPolicy marks a finding that breaks a process rule without touching the run's state or tracked content.
	AuditSeverityPolicy AuditSeverity = "policy"
)

// forkKey builds a fork finding's Key from its transcript, class and per-class ordinal.
func forkKey(transcript string, class AuditViolationClass, ordinal int) string {
	return fmt.Sprintf("fork:%s:%s:%d", transcript, class, ordinal)
}

// parentKey builds a parent finding's Key from its class and ordinal.
func parentKey(class AuditViolationClass, ordinal int) string {
	return fmt.Sprintf("parent:%s:%d", class, ordinal)
}

// Error implements the error interface, formatting the violation as a single-line, webster-prefixed
// message a caller can wrap or surface verbatim.
func (v AuditViolation) Error() string {
	if v.TranscriptPath == "" {
		return fmt.Sprintf("webster: %s violation: %s", v.Class, v.Detail)
	}
	return fmt.Sprintf("webster: %s violation in %q: %s", v.Class, v.TranscriptPath, v.Detail)
}

// isOwnReportWrite reports whether write, resolved against workdir, is ownReport: equal to its cleaned path, or canonicalizing to the same path.
func isOwnReportWrite(workdir, write, ownReport string) bool {
	resolved := resolveWritePath(workdir, write)
	if resolved == filepath.Clean(ownReport) {
		return true
	}
	canonOwn, err := canonicalPath(ownReport)
	if err != nil {
		return false
	}
	canonWrite, err := canonicalPath(resolved)
	return err == nil && canonWrite == canonOwn
}

// CheckFork evaluates one fork's transcript facts against webster's implementer policy.
// Write/Edit and repo-native git are allowed.
// It bans any attempted Agent call and any Bash command referencing the fabric repo.
// It also bans writes to a contract file (outcomePath or summaryPath);
// writes under the plan directory;
// and writes under webster's run directory other than the fork's own report.
// planDirs and websterDirs hold the plan and webster directories' spellings: the told path and its link-resolved form.
// ownReport is the report this fork may write, compared in its told and link-resolved spellings.
// A write that is also a contract write yields only its fork-contract-write finding.
// fabricRef reports whether a Bash command references the fabric repo.
// It is called without a nil check, so a nil fabricRef panics.
func CheckFork(f shuttleengine.ForkReport, outcomePath, summaryPath, workdir string, planDirs, websterDirs []string, ownReport string, fabricRef RefMatcher) []AuditViolation {
	var violations []AuditViolation

	if f.AgentCalls > 0 {
		violations = append(violations, AuditViolation{
			Class:          ClassNestedAgent,
			TranscriptPath: f.TranscriptPath,
			Key:            forkKey(f.TranscriptPath, ClassNestedAgent, 1),
			Detail: fmt.Sprintf(
				"attempted %d Agent tool call(s) — forks cannot nest and must never call the Agent tool, even when the attempt was denied",
				f.AgentCalls,
			),
		})
	}

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	contractWrites := 0
	planWrites := 0
	stateWrites := 0
	for _, w := range f.WritePaths {
		cw := resolveWritePath(workdir, w)
		if cw == cleanOutcome || cw == cleanSummary {
			contractWrites++
			violations = append(violations, AuditViolation{
				Class:          ClassForkContractWrite,
				TranscriptPath: f.TranscriptPath,
				Key:            forkKey(f.TranscriptPath, ClassForkContractWrite, contractWrites),
				Path:           w,
				Detail:         fmt.Sprintf("fork wrote %q — outcome.yaml and summary.md are Master's own contract files; a fork writing either forges the run's terminal judgment", w),
			})
			continue
		}
		for _, dir := range planDirs {
			if pathWithin(filepath.Clean(dir), cw) {
				planWrites++
				violations = append(violations, AuditViolation{
					Class:          ClassForkPlanWrite,
					TranscriptPath: f.TranscriptPath,
					Key:            forkKey(f.TranscriptPath, ClassForkPlanWrite, planWrites),
					Path:           w,
					Detail:         fmt.Sprintf("fork wrote %q — the plan directory is rewritten only by webster itself; a fork writing there can change a later card or its own card's Verify", w),
				})
				break
			}
		}
		if isOwnReportWrite(workdir, w, ownReport) {
			continue
		}
		for _, dir := range websterDirs {
			if pathWithin(filepath.Clean(dir), cw) {
				stateWrites++
				violations = append(violations, AuditViolation{
					Class:          ClassForkStateWrite,
					TranscriptPath: f.TranscriptPath,
					Key:            forkKey(f.TranscriptPath, ClassForkStateWrite, stateWrites),
					Path:           w,
					Detail:         fmt.Sprintf("fork wrote %q — webster's run state is written only by webster and each fork's own report; a fork writing there can forge a batch record", w),
				})
				break
			}
		}
	}

	fabricRefs := 0
	for _, cmd := range f.BashCommands {
		if fabricRef.Matches(cmd) {
			fabricRefs++
			violations = append(violations, AuditViolation{
				Class:          ClassFabricReference,
				TranscriptPath: f.TranscriptPath,
				Key:            forkKey(f.TranscriptPath, ClassFabricReference, fabricRefs),
				Command:        cmd,
				Detail:         fabricReferenceDetail(cmd, "an implementer fork must never touch the fabric repo directly"),
			})
		}
	}

	return violations
}

// planDirSpellings returns geom.PlanDir and its link-resolved form, deduplicated.
// A write spelled through the `_lym` link, or through its target, matches.
// It returns an error when the directory cannot be canonicalized.
func planDirSpellings(geom Geometry) ([]string, error) {
	return dirSpellings(geom.PlanDir)
}

// websterDirSpellings returns geom.WebsterDir and its link-resolved form, deduplicated.
// It returns an error when the directory cannot be canonicalized.
func websterDirSpellings(geom Geometry) ([]string, error) {
	return dirSpellings(geom.WebsterDir)
}

// dirSpellings returns dir and its link-resolved form, deduplicated.
// It returns an error when dir cannot be canonicalized.
func dirSpellings(dir string) ([]string, error) {
	canon, err := canonicalPath(dir)
	if err != nil {
		return nil, err
	}
	dirs := []string{dir}
	if canon != dir {
		dirs = append(dirs, canon)
	}
	return dirs, nil
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

// isTranscriptPathAbsolute reports whether a transcript-recorded write path is absolute:
// either in the running OS's sense or as a POSIX path with a leading "/".
// Transcript-recorded write paths and the workdir and contract paths are always POSIX-style, whatever the platform.
// On Windows, filepath.IsAbs alone misses a leading "/".
// Joining such a path onto workdir again yields a path that never matches the contract paths.
func isTranscriptPathAbsolute(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/")
}

// CheckParent reports the violations in Master's own parent session.
// Master may write only to the two contract files (outcomePath and summaryPath).
// It bans three hard violations:
// any named spawn, any parent write outside the contract files, and any Bash command referencing the fabric repo.
// fabricRef classifies fabric-reference commands.
// It must not be nil: Matches is called unguarded, and a nil interface panics.
// A hub run passes the fabric reference scanner; a standalone run passes a matcher that never matches.
func CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath, workdir string, fabricRef RefMatcher) []AuditViolation {
	var violations []AuditViolation

	for i := 1; i <= a.NamedSpawns; i++ {
		violations = append(violations, AuditViolation{
			Class: ClassNamedSpawn,
			Key:   parentKey(ClassNamedSpawn, i),
			Detail: fmt.Sprintf(
				"named spawn %d of %d: a fork was spawned with a name — named forks silently lose inherited context, which is a silent quality-degradation defect, not an advisory",
				i, a.NamedSpawns,
			),
		})
	}

	cleanOutcome := filepath.Clean(outcomePath)
	cleanSummary := filepath.Clean(summaryPath)
	for i, w := range a.ParentWrites {
		cw := resolveWritePath(workdir, w)
		if cw != cleanOutcome && cw != cleanSummary {
			violations = append(violations, AuditViolation{
				Class:  ClassParentWrite,
				Key:    parentKey(ClassParentWrite, i),
				Path:   w,
				Detail: fmt.Sprintf("Master wrote %q — Master may write only its two contract files (outcome.yaml and summary.md); any other write means Master implemented a batch itself or hand-wrote a batch report", w),
			})
		}
	}

	for i, cmd := range a.ParentBashCommands {
		if fabricRef.Matches(cmd) {
			violations = append(violations, AuditViolation{
				Class:   ClassFabricReference,
				Key:     parentKey(ClassFabricReference, i),
				Command: cmd,
				Detail:  fabricReferenceDetail(cmd, "Master must never touch the fabric repo directly; the fabric sync is webstercli's own in-process job"),
			})
		}
	}

	return violations
}

// fabricReferenceCommandOpen opens the quoted command in a fabric-reference finding's detail.
const fabricReferenceCommandOpen = "ran a fabric-referencing command ("

// fabricReferenceDetail words a fabric-reference finding;
// every fabric-referencing command can rewrite run state, so the wording never splits by command.
func fabricReferenceDetail(cmd, rule string) string {
	return fmt.Sprintf(fabricReferenceCommandOpen+"%q) that can rewrite run state — %s", cmd, rule)
}

// fabricReferenceCommand returns the unquoted command a fabric-reference entry records.
// It reports false for an entry without the quoted command text, such as one an older record wrote.
func fabricReferenceCommand(entry string) (string, bool) {
	_, quoted, found := strings.Cut(entry, fabricReferenceCommandOpen)
	if !found {
		return "", false
	}
	prefix, err := strconv.QuotedPrefix(quoted)
	if err != nil {
		return "", false
	}
	cmd, err := strconv.Unquote(prefix)
	if err != nil {
		return "", false
	}
	return cmd, true
}

// ClassifyViolation assigns v its severity, checking the correctness rule first.
// Fork contract, plan and state writes are always correctness.
// A fabric-reference is policy when readOnly accepts its command, and correctness otherwise:
// a command reaching the fabric repo can rewrite run state,
// and a re-run of the verify commands cannot detect the rewrite.
// A nil readOnly accepts nothing; the classifier sees only the command's static shape:
// a command behind `bash -c`, a variable or a substitution stays correctness.
// A parent write is correctness when its path lies under the run's state, reports or plan directory.
// The run's `_lym` directory (the parent of geom.WebsterDir) counts too.
// So does the `.lym` state directory (the parent of geom.ScratchDir), which git ignores:
// it holds webster's locks, another module's lock or pause flag, and a reed launch script, which decide what a run does.
// A write within another worktree of the task repository, or within that worktree's `_lym`, is correctness;
// the innermost worktree holding the write decides, so an outer worktree never claims a write inside a nested one.
// A write inside this worktree is correctness when git does not ignore it; every other parent write is policy.
// Prefix tests compare link-resolved paths.
// A write spelled through a link to the run's `_lym` is therefore still correctness.
// The error is only the git probe's or the link resolution's failure.
// Every other class is policy.
func ClassifyViolation(v AuditViolation, geom Geometry, readOnly func(cmd string) bool) (AuditSeverity, error) {
	switch v.Class {
	case ClassForkContractWrite, ClassForkPlanWrite, ClassForkStateWrite:
		return AuditSeverityCorrectness, nil
	case ClassFabricReference:
		if readOnly != nil && readOnly(v.Command) {
			return AuditSeverityPolicy, nil
		}
		return AuditSeverityCorrectness, nil
	case ClassParentWrite:
	default:
		return AuditSeverityPolicy, nil
	}

	written, err := canonicalPath(resolveWritePath(geom.PaneCwd, v.Path))
	if err != nil {
		return "", err
	}
	runDirs := []string{geom.WebsterDir, geom.ReportsDir, geom.PlanDir, filepath.Dir(geom.WebsterDir), filepath.Dir(geom.ScratchDir)}
	for _, dir := range runDirs {
		canon, err := canonicalPath(dir)
		if err != nil {
			return "", err
		}
		if pathWithin(canon, written) {
			return AuditSeverityCorrectness, nil
		}
	}

	worktree, err := canonicalPath(geom.WorktreeRoot)
	if err != nil {
		return "", err
	}
	others, err := geom.git().OtherWorktrees(worktree)
	if err != nil {
		return "", err
	}
	for _, other := range others {
		// The innermost worktree holding a write decides its class,
		// so an enclosing worktree never claims a write inside this one.
		if pathWithin(other, worktree) && pathWithin(worktree, written) {
			continue
		}
		lym, err := canonicalPath(filepath.Join(other, lymdirs.LymDirName))
		if err != nil {
			return "", err
		}
		if pathWithin(other, written) || pathWithin(lym, written) {
			return AuditSeverityCorrectness, nil
		}
	}
	if !pathWithin(worktree, written) {
		return AuditSeverityPolicy, nil
	}
	ignored, err := geom.git().IgnoredPath(geom.WorktreeRoot, written)
	if err != nil {
		return "", err
	}
	if ignored {
		return AuditSeverityPolicy, nil
	}
	return AuditSeverityCorrectness, nil
}

// pathWithin reports whether path is dir itself or lies beneath it;
// both must already be canonical.
func pathWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// canonicalPath resolves links in path through its nearest existing ancestor.
// It re-joins the missing tail, so a path that does not exist yet still compares against a link-resolved directory.
func canonicalPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	var tail []string
	cur := cleaned
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("websterengine: resolve links in %s: %w", cur, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cleaned, nil
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

// ForkWarnings returns a warning for a fork that never returned a final report, and nil otherwise.
// Such a fork ran clean but delivered no findings, which no mechanism can prevent in advance,
// so it is a warning rather than a hard violation.
func ForkWarnings(f shuttleengine.ForkReport) []string {
	if !f.ReportReturned {
		return []string{fmt.Sprintf("fork %q never returned a final report", f.TranscriptPath)}
	}
	return nil
}

// ErrNoForkTranscripts reports that a batch has no counted fork transcript in its current bracket:
// none is new since the previous batch boundary, and none of the bracket's own wrote the batch's report.
// Issue it only after the settle window has run out, never on the first miss:
// a fork's transcript file may not be flushed to disk when the fork call returns.
// The transcript count is checked before report presence.
// A report with no fork behind it, which means Master wrote it itself, fails too.
// The same error fires for a crash window resumed on another machine:
// fork transcripts are machine-local, so the report cannot be re-attributed there.
var ErrNoForkTranscripts = errors.New("no counted fork transcript in the batch's current bracket (none new since the previous batch boundary, and none of the bracket's own that wrote the batch's report) — the batch was never forked (or its transcript is not on this machine: fork transcripts are machine-local, so a crash window resumed on a different machine cannot re-attribute its report)")

// DefaultSettleWindow is the default total wait budget before a zero-transcript result is treated as final.
// The window leaves slack for Claude Code to flush a just-returned fork's subagents/<id>.jsonl to disk.
// It does not slow a normal call, which almost always finds its transcript on the first scan.
const DefaultSettleWindow = 3 * time.Second

// DefaultSettleTick is the default re-scan interval within the settle window.
// It is frequent enough that a normal call sees its transcript within one or two ticks of it appearing.
const DefaultSettleTick = 250 * time.Millisecond

// Sleeper abstracts time.Sleep so a wait loop never blocks for real under test.
// A recording fake lets a test assert how many ticks were requested.
// It also drives the retry loop to completion instantly.
type Sleeper interface {
	// Sleep blocks for d in production, and records the request under test.
	Sleep(d time.Duration)
}

// NewTranscripts returns the fork reports in audit whose transcript path is not in seen.
// It is a defensive re-filter: the caller may not have filtered the audit already, or may be re-deriving after a retry.
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

// SettleRetry calls fetch repeatedly, sleeping tick between attempts through s.
// It stops once fetch reports a transcript new since seen, or once window has elapsed.
// A first empty result is inconclusive: a fork's transcript may not be flushed to disk when the fork call returns.
// It returns as soon as fetch reports a new transcript and never sleeps out the rest of window.
// A fetch error returns at once, since an audit read that failed has nothing safe to retry against.
// When window elapses with no new transcript, it returns the last audit, a nil or empty newReports, and a nil error.
// The caller decides whether that empty result is an error.
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

// ClassifyAttribution classifies newReports, the counted fork transcripts:
// those new since the previous batch boundary, plus the bracket's own that wrote the report.
//
//  1. Zero counted transcripts: returns ErrNoForkTranscripts, whether or not a batch report file exists.
//     A report with no fork behind it means Master wrote it itself.
//     The count is checked first, before the report file is looked for.
//  2. Exactly one counted transcript: clean, the normal case. Returns ("", nil).
//  3. More than one counted transcript: a warning, never an error.
//     A fork whose call errored, followed by a direct re-fork, is legitimate retry behavior.
func ClassifyAttribution(newReports []shuttleengine.ForkReport) (warning string, err error) {
	switch len(newReports) {
	case 0:
		return "", ErrNoForkTranscripts
	case 1:
		return "", nil
	default:
		return fmt.Sprintf(
			"%d fork transcripts count toward the batch — expected exactly one; treating as a fork-error-then-re-fork with no intervening record-batch call",
			len(newReports),
		), nil
	}
}
