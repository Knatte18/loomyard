// roundsnapshot.go implements the per-round snapshot record, round-<n>-snapshot.json in the run directory.
// The review producer writes the round's start into it, and the fix producer adds whether decisions were routed.
// The record is tracked with the run, and every write goes through internal/state so unknown keys survive.

package shedadapters

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// roundSnapshotLockSuffix is appended to the record's name to form its tracked-file lock.
const roundSnapshotLockSuffix = ".lock"

// roundSnapshot is the record of one round's start.
// TargetID is empty when the round had no target snapshot or its id could not be read.
// DecisionsRouted is a pointer so an absent flag is told apart from false.
type roundSnapshot struct {
	FixScope        string                `json:"fix_scope"`
	TargetPaths     []string              `json:"target_paths"`
	TargetID        string                `json:"target_id,omitempty"`
	PostFixTargetID string                `json:"post_fix_target_id,omitempty"`
	AddedDecisions  []addedDecisionDigest `json:"added_decisions,omitempty"`
	DecisionsRouted *bool                 `json:"decisions_routed,omitempty"`
}

// addedDecisionDigest names one `Added after Discussion` entry of the decision record by title and content hash.
type addedDecisionDigest struct {
	Title  string `json:"title"`
	Digest string `json:"digest"`
}

// readRoundSnapshot reads round n's record under runDir.
// It reports false for an absent record.
func readRoundSnapshot(runDir string, n int) (roundSnapshot, bool, error) {
	path := roundSnapshotPath(runDir, n)
	return state.ReadJSON[roundSnapshot](path, lock.TrackedFileLockPath(path, roundSnapshotLockSuffix))
}

// updateRoundSnapshot is a locked read-modify-write over round n's record under runDir.
// mutate receives the record as it stands and whether it existed; an absent record starts as the zero value.
// The write keeps every key of the file the record's type does not know.
func updateRoundSnapshot(runDir string, n int, mutate func(rec *roundSnapshot, found bool)) error {
	path := roundSnapshotPath(runDir, n)
	return state.UpdateJSON(path, lock.TrackedFileLockPath(path, roundSnapshotLockSuffix), func(cur roundSnapshot, found bool) (roundSnapshot, error) {
		mutate(&cur, found)
		return cur, nil
	})
}

// addedDecisionDigests returns one digest per `Added after Discussion` entry of the decision record at recordPath.
// The digests come in record order.
// The digest is the hex SHA-256 of the entry's text, heading and body.
// An empty or absent record path gives an empty list.
func addedDecisionDigests(recordPath string) ([]addedDecisionDigest, error) {
	texts, err := discussionparser.AddedDecisionTexts(recordPath)
	if err != nil || len(texts) == 0 {
		return nil, err
	}
	record, err := os.ReadFile(recordPath)
	if err != nil {
		return nil, fmt.Errorf("shedadapters: read decision record %q: %w", recordPath, err)
	}
	decisions, err := discussionparser.ParseAddedDecisions(record)
	if err != nil {
		return nil, fmt.Errorf("shedadapters: parse the added decisions of %q: %w", recordPath, err)
	}
	if len(decisions) != len(texts) {
		return nil, fmt.Errorf("shedadapters: decision record %q holds %d added entries but %d headings", recordPath, len(texts), len(decisions))
	}
	digests := make([]addedDecisionDigest, len(texts))
	for i, text := range texts {
		sum := sha256.Sum256([]byte(text))
		digests[i] = addedDecisionDigest{Title: decisions[i].Title, Digest: hex.EncodeToString(sum[:])}
	}
	return digests, nil
}
