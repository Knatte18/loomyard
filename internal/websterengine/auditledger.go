// auditledger.go is the durable ledger webster's audit dispositions record into.
// A finding is dispositioned at most once per run, by identity.
// The whole-session parent audit repeats every earlier finding on each record-batch,
// so the ledger is what lets a finding warn or refuse once and then stay quiet.
// The ledger lives in state.json (State.AuditDispositions, plus the batch-level and run-level AuditWarnings lists);
// every helper here mutates the in-memory *State and leaves persisting to the caller.

package websterengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
)

// AuditWarning is one recorded warning: the finding's identity, its class, and the detail line.
type AuditWarning struct {
	// Identity is the finding's ledger identity (findingIdentity).
	Identity string `json:"identity"`
	// Class is the finding's class name.
	Class string `json:"class"`
	// Detail is the human-readable detail.
	Detail string `json:"detail"`
}

// The disposition values State.AuditDispositions records.
const (
	dispositionWarned = "warned"
	dispositionFailed = "failed"
)

// findingIdentity returns v's ledger identity.
// A parent finding's identity is `<sessionID>/<v.Key>`, because parent ordinals restart in every Master session;
// a fork finding's identity is v.Key, since the transcript path is already unique.
func findingIdentity(sessionID string, v AuditViolation) string {
	if v.TranscriptPath != "" {
		return v.Key
	}
	return sessionID + "/" + v.Key
}

// isDispositioned reports whether id already has a disposition in st.
func isDispositioned(st *State, id string) bool {
	_, ok := st.AuditDispositions[id]
	return ok
}

// markDisposition records disposition for id.
func markDisposition(st *State, id, disposition string) {
	if st.AuditDispositions == nil {
		st.AuditDispositions = map[string]string{}
	}
	st.AuditDispositions[id] = disposition
}

// recordBatchWarning records id as warned and appends one AuditWarning to bs.AuditWarnings.
// It returns added == false, and appends nothing, when id is already dispositioned.
// It takes class and detail as strings, so any warning, not only an audit finding, records through the same path.
func recordBatchWarning(st *State, bs *BatchState, id, class, detail string) (text string, added bool) {
	if isDispositioned(st, id) {
		return "", false
	}
	w := AuditWarning{Identity: id, Class: class, Detail: detail}
	markDisposition(st, id, dispositionWarned)
	bs.AuditWarnings = append(bs.AuditWarnings, w)
	return auditWarningText(w), true
}

// recordRunWarning records id as warned and appends one AuditWarning to st.AuditWarnings, the run-level list.
// It returns added == false, and appends nothing, when id is already dispositioned.
func recordRunWarning(st *State, id, class, detail string) (text string, added bool) {
	if isDispositioned(st, id) {
		return "", false
	}
	w := AuditWarning{Identity: id, Class: class, Detail: detail}
	markDisposition(st, id, dispositionWarned)
	st.AuditWarnings = append(st.AuditWarnings, w)
	return auditWarningText(w), true
}

// recordFailedFinding records id as failed, so the finding that failed a batch never refuses a second time.
func recordFailedFinding(st *State, id string) {
	markDisposition(st, id, dispositionFailed)
}

// ErrAuditNotAcceptable is the sentinel that a refused audit acceptance wraps, so callers can match it with errors.Is.
var ErrAuditNotAcceptable = errors.New("webster: pending audit findings cannot be accepted")

// acceptAuditHeadRefusal words the HEAD refusal of accept-audit, which records no batch and is cleared by re-running it.
var acceptAuditHeadRefusal = headRefusal{resetStep: "lym webster reset --to last-batch-head", rerun: `re-run "lym webster accept-audit"`, redoMerge: "once accept-audit accepts the findings"}

// AcceptPendingAudit clears the pending audit findings and returns what it cleared, once their evidence holds.
// The evidence first needs HEAD to be the last recorded batch head, or a clean parent merge on top of it.
// Every pending path must then match that head.
// A differing or unverifiable path, or a finding with no path, refuses with ErrAuditNotAcceptable.
// A refusal mutates nothing.
// A missing recorded commit refuses first, with the way forward for it.
// A contract file is judged by its write history.
// An uncleared one refuses, naming the path, why, and the delete route.
// A differing plan path cannot be restored by a git checkout.
// Its refusal names restore-plan or rebaseline --card as the way forward.
// Accepting records no disposition.
// A later run's audit covers a new Master session, whose finding identities never repeat these.
// The returned bool reports that a path was accepted on an absent contract file,
// so the caller can tell the operator that Master must write the files again.
// It never saves; the caller holds the state-mutation lease and saves.
func AcceptPendingAudit(engines AuditEngines, st *State, geom Geometry, parentBranch ParentBranchFunc) (accepted []PendingAuditFinding, onAbsentContract bool, err error) {
	pending := st.PendingAuditFindings
	var paths []string
	pathless := false
	for _, f := range pending {
		if len(f.Paths) == 0 {
			pathless = true
		}
		for _, p := range f.Paths {
			if !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		}
	}
	bases, err := runEvidenceBases(geom, st)
	if err != nil {
		return nil, false, err
	}
	if len(bases.Missing) > 0 {
		return nil, false, fmt.Errorf("%w: %s", ErrAuditNotAcceptable, missingCommitsClause(bases.Missing))
	}
	head := bases.Last
	if head != "" {
		if _, err := reconcileHead(geom.git(), geom.WorktreeRoot, head, "accept-audit: last batch head", parentBranch, acceptAuditHeadRefusal); err != nil {
			return nil, false, fmt.Errorf("%w: %v", ErrAuditNotAcceptable, err)
		}
		if head, err = geom.git().HeadSHA(geom.WorktreeRoot); err != nil {
			return nil, false, err
		}
	}
	writes, err := contractWritesFor(engines, st, geom, paths)
	if err != nil {
		return nil, false, err
	}
	contracts, err := splitContractPaths(geom, writes, paths)
	if err != nil {
		return nil, false, err
	}
	differing, unverifiable, err := checkSuspectPaths(geom, st, head, contracts.Rest)
	if err != nil {
		return nil, false, err
	}
	if len(differing) == 0 && len(unverifiable) == 0 && len(contracts.Uncleared) == 0 && !pathless {
		st.PendingAuditFindings = nil
		return pending, len(contracts.Absent) > 0, nil
	}
	var parts []string
	if len(contracts.Uncleared) > 0 {
		parts = append(parts, contractDeleteClause(contracts.Uncleared, "lym webster accept-audit"))
	}
	planDiffering, gitDiffering, err := splitPlanPaths(geom, differing)
	if err != nil {
		return nil, false, err
	}
	if len(planDiffering) > 0 {
		parts = append(parts, fmt.Sprintf("plan file(s) differ from the plan the run recorded: %s; way forward: %s", strings.Join(planDiffering, ", "), planPathClause("\"lym webster accept-audit\"")))
	}
	if len(gitDiffering) > 0 {
		parts = append(parts, fmt.Sprintf("differs from the last batch head; %s", wayForwardSteps(restoreStep(head, gitDiffering), stepAcceptAudit)))
	}
	if len(unverifiable) > 0 || pathless {
		var what []string
		for _, p := range unverifiable {
			reason, _, err := uncheckableReason(geom, st, p)
			if err != nil {
				return nil, false, err
			}
			if reason == "" {
				reason = reasonNoBatchHead
			}
			what = append(what, fmt.Sprintf("%s cannot be checked: %s", p, reason))
		}
		if pathless {
			what = append(what, reasonNoPath)
		}
		parts = append(parts, fmt.Sprintf("%s; %s", strings.Join(what, "; "), resetToStartSteps(stepRun)))
	}
	return nil, false, fmt.Errorf("%w: %s", ErrAuditNotAcceptable, strings.Join(parts, "; "))
}

// AcceptBatchFabricReference clears the Uncheckable entries of failed batch n.
// It clears them only when every entry is a pathless fabric-reference finding and the batch's evidence holds,
// so that `lym webster recover-batch n` can proceed instead of refusing toward the reset-to-start route.
// The verb argument names the caller that accepts, `accept-audit --batch` or `recover-batch`.
// It appears in the re-run that the evidence refusals name, and in each recorded warning's detail.
// The evidence is that the batch's recorded start commit is set,
// and the worktree is clean apart from the run's own state.
// One of two routes must hold:
// HEAD is that start, or the start is an ancestor of HEAD and every entry records a command that readOnly accepts.
// The committed route shows only that the start lies in HEAD's history and that nothing is uncommitted.
// No command readOnly accepts writes tracked content, a ref, config or a remote,
// and `git status` may refresh the index's stat cache, which changes no content.
// It does not inspect the batch's commits.
// The fabric repo's own state is outside the evidence, so the caller vouches for it by running the verb.
// Everywhere else a fabric reference stays a correctness failure.
// This clears it only on that evidence, and only through a verb that vouches for it.
// Each cleared entry is recorded on the batch as an AuditWarning,
// so a recovery carries it and summary.md names it.
// Any other entry, a batch that is not terminal failed, or missing evidence refuses with ErrAuditNotAcceptable.
// The refusal mutates nothing.
// It never saves; the caller holds the state-mutation lease and saves.
func AcceptBatchFabricReference(st *State, geom Geometry, n int, readOnly func(cmd string) bool, verb string) (accepted []string, err error) {
	basis, err := batchFabricReferenceEvidence(st, geom, n, readOnly, fmt.Sprintf("re-run \"lym webster %s %d\"", verb, n))
	if err != nil {
		return nil, err
	}
	bs := st.Batches[n]
	for i, entry := range bs.Uncheckable {
		id := fmt.Sprintf("accepted:batch-%02d:%d", n, i+1)
		markDisposition(st, id, dispositionWarned)
		bs.AuditWarnings = append(bs.AuditWarnings, AuditWarning{Identity: id, Class: string(ClassFabricReference), Detail: "accepted by " + verb + ", " + basis + ": " + strings.TrimPrefix(entry, fabricReferencePrefix)})
	}
	accepted = bs.Uncheckable
	bs.Uncheckable = nil
	return accepted, nil
}

// batchFabricReferenceEvidence reports whether failed batch n's uncheckable entries may be accepted.
// On success it returns the basis that holds.
// It mutates nothing.
// Every entry must be a pathless fabric reference, the batch must have recorded a start commit,
// and the worktree must be clean apart from the run's own state.
// HEAD must be that start, or descend from it with every entry recording a command that readOnly accepts.
// A nil readOnly accepts none.
// rerun is the step a refusal names for running the accepting verb again once the evidence is restored.
// A refusal wraps ErrAuditNotAcceptable and ends in its way forward;
// any other error is a git read failure.
func batchFabricReferenceEvidence(st *State, geom Geometry, n int, readOnly func(cmd string) bool, rerun string) (basis string, err error) {
	bs := st.Batches[n]
	if bs == nil || !bs.Terminal || bs.Status != DigestStatusFailed || len(bs.Uncheckable) == 0 {
		return "", fmt.Errorf("%w: batch %02d is not a failed batch with uncheckable findings; accept-audit --batch accepts only those", ErrAuditNotAcceptable, n)
	}
	if !allPathlessFabricReference(bs.Uncheckable) {
		return "", fmt.Errorf("%w: batch %02d carries an uncheckable finding that is not a pathless fabric reference: %s; %s", ErrAuditNotAcceptable, n, strings.Join(bs.Uncheckable, ", "), resetToStartSteps(stepRun))
	}
	if bs.StartSHA == "" {
		return "", fmt.Errorf("%w: batch %02d recorded no start commit, so its tree cannot be shown unchanged; %s", ErrAuditNotAcceptable, n, resetToStartSteps(stepRun))
	}
	head, err := geom.git().HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return "", err
	}
	atStart := head == bs.StartSHA
	if !atStart {
		reachable, err := geom.git().IsAncestor(geom.WorktreeRoot, bs.StartSHA, head)
		if err != nil {
			return "", err
		}
		if !reachable {
			return "", fmt.Errorf("%w: HEAD %s is not batch %02d's start commit %s, nor does it descend from it; %s", ErrAuditNotAcceptable, head, n, bs.StartSHA, wayForwardSteps(stepResetToStart, "lym webster run"))
		}
	}
	dirty, err := UncommittedPaths(geom)
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 {
		return "", fmt.Errorf("%w: the worktree has uncommitted or untracked changes: %s; way forward: restore or remove them with git, then %s", ErrAuditNotAcceptable, strings.Join(dirty, ", "), rerun)
	}
	if atStart {
		return "HEAD was the batch's start commit and the tree clean", nil
	}
	for _, entry := range bs.Uncheckable {
		if cmd, ok := fabricReferenceCommand(entry); !ok || readOnly == nil || !readOnly(cmd) {
			return "", fmt.Errorf("%w: batch %02d's commits are kept only when every fabric reference is a read-only command, and this one is not: %s; %s", ErrAuditNotAcceptable, n, entry, wayForwardSteps(resetVerb(ResetToBatchStart, n), rerun))
		}
	}
	return "the batch's commits were kept as the command is read-only and the tree clean", nil
}

// UncommittedPaths returns, sorted, the worktree-relative paths of the task's uncommitted work:
// every tracked change and untracked file that git status names, minus the run's own state.
// A clean tree returns none, and a git read failure is returned.
func UncommittedPaths(geom Geometry) ([]string, error) {
	dirtyPaths, err := geom.git().DirtyPaths(geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	var uncommitted []string
	for _, p := range dirtyPaths {
		if !runOwnPath(geom, p) {
			uncommitted = append(uncommitted, p)
		}
	}
	sort.Strings(uncommitted)
	return uncommitted, nil
}

// runOwnPath reports whether rel, a worktree-relative path that git status names, is the run's own state.
// Such a path is not the task's content.
// The run's own state is webster's run directory, the plan directory or the scratch directory:
// a path under one of them, or a directory that holds one, as git collapses an untracked directory.
// In a standalone run those live inside the worktree, so they show as changes there;
// in a hub they live on the fabric side and never appear.
func runOwnPath(geom Geometry, rel string) bool {
	p := filepath.Join(geom.WorktreeRoot, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	for _, dir := range []string{geom.WebsterDir, geom.PlanDir, geom.ScratchDir} {
		if dir == "" {
			continue
		}
		if pathWithin(dir, p) || pathWithin(p, dir) {
			return true
		}
	}
	return false
}

// fabricReferencePrefix starts every Uncheckable entry recorded for a pathless fabric-reference finding.
// Such an entry has the form "<class>: <detail>".
const fabricReferencePrefix = string(ClassFabricReference) + ": "

// allPathlessFabricReference reports whether entries is non-empty and every entry is a pathless fabric-reference finding.
func allPathlessFabricReference(entries []string) bool {
	if len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if class, ok := pathlessEntryClass(e); !ok || class != ClassFabricReference {
			return false
		}
	}
	return true
}

// allReadOnlyFabricReferences reports whether entries is non-empty and holds only pathless fabric-reference findings:
// each records a command that readOnly accepts.
// A nil readOnly accepts nothing.
func allReadOnlyFabricReferences(entries []string, readOnly func(cmd string) bool) bool {
	if readOnly == nil || !allPathlessFabricReference(entries) {
		return false
	}
	for _, entry := range entries {
		if cmd, ok := fabricReferenceCommand(entry); !ok || !readOnly(cmd) {
			return false
		}
	}
	return true
}

// auditWarningText renders w as its envelope line.
func auditWarningText(w AuditWarning) string {
	return fmt.Sprintf("audit warning (%s): %s", w.Class, w.Detail)
}

// RecordedAuditWarnings returns every recorded warning's text: batch warnings in batches order first, run-level warnings last.
func RecordedAuditWarnings(st *State, batches []batcher.Batch) []string {
	var out []string
	for _, b := range batches {
		number, _ := batchIdentity(b)
		bs := st.Batches[number]
		if bs == nil {
			continue
		}
		for _, w := range bs.AuditWarnings {
			out = append(out, auditWarningText(w))
		}
	}
	for _, w := range st.AuditWarnings {
		out = append(out, auditWarningText(w))
	}
	return out
}
