// auditledger.go is the durable ledger webster's audit dispositions record into.
// A finding is dispositioned at most once per run, by identity: the whole-session parent audit repeats every earlier finding on each record-batch,
// so the ledger is what lets a finding warn or refuse once and then stay quiet.
// The ledger lives in state.json (State.AuditDispositions, plus the batch-level and run-level AuditWarnings lists);
// every helper here mutates the in-memory *State and leaves persisting to the caller.

package websterengine

import (
	"errors"
	"fmt"
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

// AcceptPendingAudit clears st.PendingAuditFindings and returns what it cleared, once every finding's suspect paths are back at the last recorded batch head.
// The evidence rule: checkSuspectPaths runs over every pending path with lastBatchHead(st) as base,
// and any differing path, any unverifiable path and any finding with no path refuses with ErrAuditNotAcceptable, mutating nothing.
// It records no disposition, because a later run's audit covers a new Master session whose finding identities never repeat these.
// It never saves;
// the caller holds the state-mutation lease and saves.
func AcceptPendingAudit(st *State, geom Geometry) ([]PendingAuditFinding, error) {
	pending := st.PendingAuditFindings
	var paths []string
	pathless := false
	for _, f := range pending {
		if len(f.Paths) == 0 {
			pathless = true
		}
		paths = append(paths, f.Paths...)
	}
	head := lastBatchHead(st)
	differing, unverifiable, err := checkSuspectPaths(geom, st, head, paths)
	if err != nil {
		return nil, err
	}
	if len(differing) == 0 && len(unverifiable) == 0 && !pathless {
		st.PendingAuditFindings = nil
		return pending, nil
	}
	var parts []string
	if len(differing) > 0 {
		parts = append(parts, fmt.Sprintf("differs from the last batch head: %s; way forward: restore each with \"git checkout %s -- <path>\" (delete a path the head does not hold), then re-run \"lyx webster accept-audit\"", strings.Join(differing, ", "), head))
	}
	if len(unverifiable) > 0 || pathless {
		var what []string
		if len(unverifiable) > 0 {
			what = append(what, "cannot be checked: "+strings.Join(unverifiable, ", "))
		}
		if pathless {
			what = append(what, "a finding names no path")
		}
		start := runStartCommit(st)
		if start != "" {
			start = " " + start
		}
		parts = append(parts, fmt.Sprintf("%s; way forward: reset the branch to the run's start commit%s with git and run \"lyx webster run --fresh\"", strings.Join(what, "; "), start))
	}
	return nil, fmt.Errorf("%w: %s", ErrAuditNotAcceptable, strings.Join(parts, "; "))
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
