// auditledger.go is the durable ledger webster's audit dispositions record into.
// A finding is dispositioned at most once per run, by identity: the whole-session parent audit repeats
// every earlier finding on each record-batch, so the ledger is what lets a finding warn or refuse once
// and then stay quiet.
// The ledger lives in state.json (State.AuditDispositions, plus the batch-level and run-level
// AuditWarnings lists); every helper here mutates the in-memory *State and leaves persisting to the caller.

package websterengine

import (
	"fmt"

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
