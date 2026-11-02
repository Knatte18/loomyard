// fixerattempts.go implements FixerAttempts, the record of the fixer turns one landing row entry spent, kept in the run's scratch directory so a resume never grants a fresh cap.

package landingshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FixerAttempts is the fixer turns one row entry spent.
// An entry has no attempts left when Spent reaches its row's cap, whatever Halt notes.
type FixerAttempts struct {
	// Entry is the row entry the record belongs to.
	Entry int `json:"entry"`
	// Spent is the fixer turns spent.
	Spent int `json:"spent"`
	// Baseline is the HEAD a cap measures a fixer change from.
	Baseline string `json:"baseline"`
	// AlignFrom is the parent tip a Parent-Align entry reads the parent's changes from when the merge record holds no aligned tip.
	AlignFrom string `json:"align_from"`
	// Halt notes the halt that last stopped the entry: "spent", "over-cap", "review-capped" or empty.
	Halt string `json:"halt"`
	// Reviewed is the HEAD a fix review approved the entry's over-cap change at; empty when none did.
	Reviewed string `json:"reviewed,omitempty"`
	// ReviewRounds is the fix reviews the entry ran over its over-cap change.
	ReviewRounds int `json:"review_rounds,omitempty"`
	// FixReview is the report of a fix review whose findings the fixer has yet to fix; empty when none is pending.
	FixReview string `json:"fix_review,omitempty"`
	// Findings are the findings the last fix review left open when the entry halted at its review round cap.
	Findings []string `json:"findings,omitempty"`
	// AcceptedHead is the HEAD the operator accepted the entry's change at after that halt, so the entry lands that change with Findings open; empty when none was accepted.
	// A change at any other HEAD is not covered by the acceptance.
	AcceptedHead string `json:"accepted_head,omitempty"`
	// Judged is the HEAD at which the entry's fixer change was last let through, within the caps, approved or accepted; empty when none was.
	// A later merge of the parent onto it moves Baseline past the parent's commits.
	Judged string `json:"judged,omitempty"`
}

// FixerAttemptsPath is the attempt record's path for row under scratchDir: one file per row.
func FixerAttemptsPath(scratchDir, row string) string {
	return filepath.Join(scratchDir, row+"-fixer-attempts.json")
}

// ReadFixerAttempts reads the record at path and returns it when it names entry.
// A record naming another entry and an absent file both return a zero record for entry, which has spent no attempt.
// An undecodable file is an error naming it, and an unknown key is tolerated.
func ReadFixerAttempts(path string, entry int) (FixerAttempts, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return FixerAttempts{Entry: entry}, nil
	}
	if err != nil {
		return FixerAttempts{}, fmt.Errorf("landingshed: read fixer attempts %s: %w", path, err)
	}
	var rec FixerAttempts
	if err := json.Unmarshal(data, &rec); err != nil {
		return FixerAttempts{}, fmt.Errorf("landingshed: decode fixer attempts %s: %w", path, err)
	}
	if rec.Entry != entry {
		return FixerAttempts{Entry: entry}, nil
	}
	return rec, nil
}

// WriteFixerAttempts writes rec to path atomically, creating the parent directory.
func WriteFixerAttempts(path string, rec FixerAttempts) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("landingshed: encode fixer attempts: %w", err)
	}
	return writeBytesAtomic(path, "fixer-attempts", data)
}

// RemoveFixerAttempts removes the record at path; an absent file is no error.
func RemoveFixerAttempts(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("landingshed: remove fixer attempts %s: %w", path, err)
	}
	return nil
}
