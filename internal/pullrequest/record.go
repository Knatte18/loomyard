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
// Number is the pull request's number and the one field always written.
type Record struct {
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
	// ClosedForContinue records that the row closed the pull request on the continue flag.
	ClosedForContinue bool `json:"closed_for_continue,omitempty"`
}

// ErrNoRecord is returned by UpdateRecord when the record's file does not exist.
var ErrNoRecord = errors.New("pullrequest: no pull request record")

// ReadRecord reads the record at path under the lock at lockPath.
// It reports false when the file does not exist.
func ReadRecord(path, lockPath string) (Record, bool, error) {
	return state.ReadJSON[Record](path, lockPath)
}

// WriteRecord writes rec to path under the lock at lockPath, keeping any key of the replaced file that Record does not declare.
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
