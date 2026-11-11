// decision.go — a board entry's decision record: its file name, its store and read, and the discussed payload check.
//
// The record is the file decision-<slug>.md beside board.json, and the entry's Discussed flag says lym stored it.

package boardengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrRecordEntryDone is the error a store of a decision record wraps when the entry is done.
var ErrRecordEntryDone = errors.New("a landed entry keeps the decision record it last stored")

// DecisionRecordError reports a rename that saved the entry under its new slug:
// moving or clearing its decision record file failed.
type DecisionRecordError struct {
	Err error
}

// Error returns the message of the failed record step.
func (e *DecisionRecordError) Error() string { return e.Err.Error() }

// Unwrap returns the failed record step's error.
func (e *DecisionRecordError) Unwrap() error { return e.Err }

// DecisionFileName returns the name of the decision record file of the entry slug, the one place the name is formed.
func DecisionFileName(slug string) string {
	return decisionFilePrefix + slug + decisionFileSuffix
}

// StoreDecisionRecord writes record as the entry slug's decision record and flags the entry discussed, under the write lock.
// The same write stamps the entry's record_body_sha256 with the sha256 of its body.
// The file is written before board.json is saved, so a crash leaves a file with no flag and never a flag with no file.
// It writes nothing when the entry is flagged, its stamp matches its body and the file already holds record.
// It bypasses the run lock for the flag, the stamp and the file only; every other field of a held entry stays locked.
// It refuses an absent entry, and a done entry with an error that matches ErrRecordEntryDone.
func (b *Board) StoreDecisionRecord(slug string, record []byte) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		i, found := store.index(slug)
		if !found {
			return nil, fmt.Errorf("no board entry %q to store the decision record on: restore the entry with `lym board upsert`, then re-run the step that stored", slug)
		}
		if isDone(store.tasks[i]) {
			return nil, fmt.Errorf("entry %q: %w: the run ledger sets aside a board commit whose entries have all ended, so a record stored now would not reach origin", slug, ErrRecordEntryDone)
		}
		name := DecisionFileName(slug)
		stamp := bodySHA256(store.tasks[i].Body)
		if current, err := os.ReadFile(filepath.Join(b.boardPath, name)); err == nil && store.tasks[i].Discussed &&
			store.tasks[i].RecordBodySHA256 == stamp && bytes.Equal(current, record) {
			return noWrite{}, nil
		}
		if err := fsx.AtomicWrite(b.boardPath, name, string(record)); err != nil {
			return nil, fmt.Errorf("write decision record %s: %w", name, err)
		}
		store.tasks[i].Discussed = true
		store.tasks[i].RecordBodySHA256 = stamp
		return nil, nil
	}, nil)
	return err
}

// bodySHA256 returns the hex sha256 of body: the stamp a write of an entry's decision record leaves on the entry.
func bodySHA256(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// recordBodyChanged reports whether task's body differs from the body its decision record was last stored against.
// An entry with no stamp, such as one whose record an older binary stored, reports false.
func recordBodyChanged(task Task) bool {
	return task.RecordBodySHA256 != "" && task.RecordBodySHA256 != bodySHA256(task.Body)
}

// recordStored reports whether the decision record file of the entry slug exists in the board directory.
func (b *Board) recordStored(slug string) bool {
	_, err := os.Stat(filepath.Join(b.boardPath, DecisionFileName(slug)))
	return err == nil
}

// DecisionRecord returns the decision record of the entry selector names, a slug string, a numeric id or a Handle.
// It reads without the lock.
// found is true only when the entry exists, carries the discussed flag and its record file exists;
// an absent entry, an unflagged one and a flagged one with no file each answer false.
// A read error other than a missing file is returned.
func (b *Board) DecisionRecord(selector any) ([]byte, bool, error) {
	task, found, err := b.GetTask(selector)
	if err != nil || !found || !task.Discussed {
		return nil, false, err
	}
	record, err := os.ReadFile(filepath.Join(b.boardPath, DecisionFileName(task.Slug)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read decision record: %w", err)
	}
	return record, true, nil
}

// followDecisionRecords makes the record files follow the entries a saved write changed.
// loaded and after are the entries before and after the write, loaded with a rename already applied;
// renamedFrom is the renamed entry's old slug and empty for any other write.
// A rename moves the old slug's record file to the new slug's name.
// When the old file is absent, it removes any file at the new slug's name, flagged entry or not:
// a file at a fresh slug's name is always an orphan the renamed entry must not adopt.
// Only a rename's failure is returned, as a *DecisionRecordError.
// Each slug of loaded that after lacks loses its record file;
// a failed removal is logged and returns no error, since an orphaned file is unread while its slug carries no flag.
func followDecisionRecords(boardPath string, loaded, after []Task, renamedFrom, renamedTo string) error {
	var renameErr error
	if renamedFrom != "" {
		oldPath := filepath.Join(boardPath, DecisionFileName(renamedFrom))
		newPath := filepath.Join(boardPath, DecisionFileName(renamedTo))
		err := os.Rename(oldPath, newPath)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Remove(newPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				renameErr = fmt.Errorf("rename saved the entry, but the orphaned decision record %s at the new slug's name could not be removed: %w: remove the file by hand, or the entry reads it as its own record", newPath, err)
			}
		case err != nil:
			renameErr = fmt.Errorf("rename moved the entry but not its decision record: %w: the record stays at %s until the entry's next Discussion approval stores it under the new slug", err, oldPath)
		}
	}

	kept := make(map[string]bool, len(after))
	for _, task := range after {
		kept[task.Slug] = true
	}
	for _, task := range loaded {
		if kept[task.Slug] {
			continue
		}
		path := filepath.Join(boardPath, DecisionFileName(task.Slug))
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("boardengine: a removed entry's decision record could not be removed", "slug", task.Slug, "path", path, "error", err)
		}
	}
	if renameErr != nil {
		return &DecisionRecordError{Err: renameErr}
	}
	return nil
}

// checkDiscussedPayload judges the discussed key of an upsert payload against the stored entry fields["slug"] names.
// The entry counts as unflagged when the store holds none.
// A bool equal to the stored value, false for a new entry, is dropped from fields; any other value is refused.
func checkDiscussedPayload(s *Store, fields map[string]any) error {
	value, carried := fields["discussed"]
	if !carried {
		return nil
	}
	slug, _ := fields["slug"].(string)
	stored, _ := s.GetTask(slug)
	if proposed, isBool := value.(bool); !isBool || proposed != stored.Discussed {
		return fmt.Errorf("entry %q: the discussed flag is never set by hand: lym sets it when a Discussion is approved, and `lym loom decision store` re-copies the record of a run past its approval; drop the discussed key from the payload", slug)
	}
	delete(fields, "discussed")
	return nil
}
