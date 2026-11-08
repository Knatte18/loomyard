// record.go holds Record, the run's pull-request record, and the locked reads and writes of its file.

package pullrequest

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// Record is the state a run keeps about its pull request, one JSON file in the run's scratch directory.
type Record struct {
	// Number is the pull request's number and the one field always written.
	Number int `json:"number"`
	// URL is the pull request's page on GitHub.
	URL string `json:"url,omitempty"`
	// Base is the branch the pull request merges into.
	Base string `json:"base,omitempty"`
	// Head is the task branch the pull request merges from.
	Head string `json:"head,omitempty"`
	// GatedHead is the head the PR-Review row last gated and pushed.
	GatedHead string `json:"gated_head,omitempty"`
	// MailedHead is the head whose gate failure was mailed, so one failure is mailed once.
	MailedHead string `json:"mailed_head,omitempty"`
	// StrandGUID, StrandName, SessionID and Engine name the interactive PR-Review session, so a later pass resumes it.
	StrandGUID string `json:"strand_guid,omitempty"`
	StrandName string `json:"strand_name,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Engine     string `json:"engine,omitempty"`
	// Continue is the operator's continue flag, set by `lym loom pr-continue`.
	Continue bool `json:"continue,omitempty"`
	// ReviewEnded records that the review ended.
	// The PR-Review row sets it when it closes the pull request on the continue flag.
	// Finalize sets it on the record it opens for a parent that takes its pull request with no review.
	// In that case the pull request is left for Finalize to merge.
	// The JSON key stays `closed_for_continue`, so a record written by an older build reads the same.
	ReviewEnded bool `json:"closed_for_continue,omitempty"`
	// ParentReviewAsked records that the PR-Review row asked the parent orch to review this pull request.
	ParentReviewAsked bool `json:"parent_review_asked,omitempty"`
}

// ErrNoRecord is returned when the record's file does not exist.
var ErrNoRecord = errors.New("pullrequest: no pull request record")

// ReadRecord reads the record at path under the lock at lockPath.
// It reports false when the file does not exist.
func ReadRecord(path, lockPath string) (Record, bool, error) {
	return state.ReadJSON[Record](path, lockPath)
}

// WriteRecord writes rec to path under the lock at lockPath.
// A key of the replaced file that Record does not declare survives the write.
func WriteRecord(path, lockPath string, rec Record) error {
	return state.WriteJSON(path, lockPath, rec)
}

// UpdateRecord applies mutate to the record at path under one exclusive lock and writes the result.
// It returns ErrNoRecord and writes nothing when the file does not exist, and writes nothing when mutate fails.
// A key of the file that Record does not declare survives the rewrite.
func UpdateRecord(path, lockPath string, mutate func(rec *Record) error) error {
	return state.UpdateJSON(path, lockPath, func(cur Record, found bool) (Record, error) {
		if !found {
			return Record{}, ErrNoRecord
		}
		if err := mutate(&cur); err != nil {
			return Record{}, err
		}
		return cur, nil
	})
}

// RemoveRecord deletes the record's file under the lock at lockPath; a file already absent is no error.
func RemoveRecord(path, lockPath string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("pullrequest: remove record: acquire write lock: %w", err)
	}
	defer l.Release()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("pullrequest: remove record: %w", err)
	}
	return nil
}
