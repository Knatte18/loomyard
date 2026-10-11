// restore.go implements RestoreRenumber, the only reader of the renumber journal, which undoes an interrupted InsertCard or RemoveCard.

package planparser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

var (
	// ErrRenumberNoJournal marks a restore with no journal on disk.
	ErrRenumberNoJournal = errors.New("planparser: no renumber journal to restore")
	// ErrRenumberJournalInvalid marks a journal that does not decode, or names a file outside the plan directory's top level.
	ErrRenumberJournalInvalid = errors.New("planparser: renumber journal is invalid")
	// ErrRenumberRestoreFailed marks a restore whose file operation failed; the journal is kept, so a re-run completes it.
	ErrRenumberRestoreFailed = errors.New("planparser: renumber restore failed")
)

// journalCardFileRe is the shape cardFileName renders for any slug the Card Index parser accepts.
var journalCardFileRe = regexp.MustCompile(`^\d+-\S+\.md$`)

// RestoreResult is what RestoreRenumber changed: the names it removed and the names it wrote back.
type RestoreResult struct {
	Removed  []string
	Restored []string
}

// RestoreRenumber undoes an interrupted InsertCard or RemoveCard from the journal it left at journalPath: it removes every file the call created, writes every file the call rewrote or deleted back, and removes the journal last.
// A journal that names anything but the overview or a card file at the plan directory's top level is refused before any file is written.
// A file operation that fails keeps the journal, and a re-run completes the restore.
// The restore discards any edit made to the journaled files since the interrupted call.
func RestoreRenumber(ops RenumberIO, planDir, journalPath string) (*RestoreResult, error) {
	journal, found, err := readRenumberJournal(ops, journalPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrRenumberNoJournal, journalPath)
	}

	restored := make([]string, 0, len(journal.Files))
	for name := range journal.Files {
		restored = append(restored, name)
	}
	sort.Strings(restored)
	for _, name := range append(append([]string(nil), restored...), journal.Created...) {
		if !journalNameOK(name) {
			return nil, fmt.Errorf("%w: %s names %q, which is not the overview or a card file at the top level of the plan directory", ErrRenumberJournalInvalid, journalPath, name)
		}
	}

	if err := reverseRenumber(ops, planDir, journal); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRenumberRestoreFailed, err)
	}
	if err := ops.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: remove journal: %w", ErrRenumberRestoreFailed, err)
	}
	return &RestoreResult{Removed: journal.Created, Restored: restored}, nil
}

// journalNameOK reports whether name is the overview or a card file name, a base name with no path separator.
func journalNameOK(name string) bool {
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return false
	}
	return name == overviewFileName || journalCardFileRe.MatchString(name)
}

// readRenumberJournal reads and decodes the journal at journalPath.
// found is false when no journal is there, a temporary journal beside it being ignored.
func readRenumberJournal(ops RenumberIO, journalPath string) (journal renumberJournal, found bool, err error) {
	data, err := ops.ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return renumberJournal{}, false, nil
	}
	if err != nil {
		return renumberJournal{}, false, fmt.Errorf("%w: read journal %s: %w", ErrRenumberIOFailed, journalPath, err)
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return renumberJournal{}, false, fmt.Errorf("%w: %s does not decode: %w", ErrRenumberJournalInvalid, journalPath, err)
	}
	return journal, true, nil
}
