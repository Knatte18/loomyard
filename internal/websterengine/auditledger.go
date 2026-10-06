// auditledger.go is the durable ledger webster's audit dispositions record into.
// A finding is dispositioned at most once per run, by identity: the whole-session parent audit repeats every earlier finding on each record-batch,
// so the ledger is what lets a finding warn or refuse once and then stay quiet.
// The ledger lives in state.json (State.AuditDispositions, plus the batch-level and run-level AuditWarnings lists);
// every helper here mutates the in-memory *State and leaves persisting to the caller.

package websterengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
// It takes class and detail as strings so a later-card drift warning records through the same path as an audit finding.
func recordBatchWarning(st *State, bs *BatchState, id, class, detail string) (text string, added bool) {
	if isDispositioned(st, id) {
		return "", false
	}
	w := AuditWarning{Identity: id, Class: class, Detail: detail}
	markDisposition(st, id, dispositionWarned)
	bs.AuditWarnings = append(bs.AuditWarnings, w)
	return auditWarningText(w), true
}

// recordRunWarning is recordBatchWarning for the run-level list.
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

// ErrAuditNotAcceptable is the sentinel AcceptPendingAudit returns while a pending finding's evidence is missing.
var ErrAuditNotAcceptable = errors.New("webster: pending audit findings cannot be accepted")

// acceptAuditHeadRefusal words AcceptPendingAudit's HEAD refusal, which records no batch and is cleared by re-running accept-audit.
var acceptAuditHeadRefusal = headRefusal{head: "the last batch head", rerun: `re-run "lyx webster accept-audit"`, redoMerge: "once accept-audit accepts the findings"}

// AcceptPendingAudit clears st.PendingAuditFindings and returns what it cleared, once every finding's suspect paths are back at the last recorded batch head.
// The evidence rule: checkSuspectPaths runs over every pending path with the last batch head as base, picked by git ancestry (runEvidenceBases),
// and any differing path, any unverifiable path and any finding with no path refuses with ErrAuditNotAcceptable, mutating nothing.
// It refuses first, with the missing-commit way forward, when a commit the run recorded is not in the repository.
// The evidence covers HEAD as well as the worktree:
// when the last batch head is known, reconcileReportHead must accept HEAD (the head itself, or a clean parent merge on top of it), else the verb refuses with ErrAuditNotAcceptable;
// the paths are then checked against that reconciled HEAD, which the differing clause names in its git checkout.
// A differing plan path never gets a git checkout, which cannot restore a plan file:
// its way forward is planPathClause (restore-plan or rebaseline --card).
// It records no disposition, because a later run's audit covers a new Master session whose finding identities never repeat these.
// It never saves;
// the caller holds the state-mutation lease and saves.
//
// A path that is one of the run's two contract files is judged by contractFileStatus before the check, over the write history engine's audit yields (nil engine: none):
// a cleared one is resolved, and an uncleared one refuses naming the path, why, and the delete route.
// The returned bool is true when a path was accepted on an absent contract file, so the caller can tell the operator Master must write the files again.
func AcceptPendingAudit(engine shuttleengine.Engine, st *State, geom Geometry, parentBranch ParentBranchFunc) (accepted []PendingAuditFinding, onAbsentContract bool, err error) {
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
	writes, err := contractWritesFor(engine, st, geom, paths)
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
		parts = append(parts, contractDeleteClause(contracts.Uncleared, "lyx webster accept-audit"))
	}
	planDiffering, gitDiffering, err := splitPlanPaths(geom, differing)
	if err != nil {
		return nil, false, err
	}
	if len(planDiffering) > 0 {
		parts = append(parts, fmt.Sprintf("plan file(s) differ from the plan the run recorded: %s; way forward: %s", strings.Join(planDiffering, ", "), planPathClause("\"lyx webster accept-audit\"")))
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
		parts = append(parts, fmt.Sprintf("%s; %s", strings.Join(what, "; "), resetToStartSteps(stepRunFresh)))
	}
	return nil, false, fmt.Errorf("%w: %s", ErrAuditNotAcceptable, strings.Join(parts, "; "))
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
