// rowstore.go implements the record store of one row gate entry's fixers.
// The store is a JSON list of records in the row visit's directory, with the report and reply paths of each fixer.

package gatefixer

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// rowRecordsFile is the file in a row entry's directory that holds its fixer records.
	rowRecordsFile = "records.json"
	// rowLockSuffix is appended to the records path to name the lock that guards it.
	rowLockSuffix = ".lock"
)

// rowStore is a Store over `<visit dir>/<entry name>/records.json`.
// The file holds a list of Record in the order the fixers were spawned.
// Reads and rewrites go through the state package, which keeps the keys a newer binary wrote.
type rowStore struct {
	dir string
}

// newRowStore returns the store of the entry named entry under the row visit's directory visitDir.
func newRowStore(visitDir, entry string) rowStore {
	return rowStore{dir: filepath.Join(visitDir, entry)}
}

func (s rowStore) recordsPath() string {
	return filepath.Join(s.dir, rowRecordsFile)
}

func (s rowStore) load() ([]Record, error) {
	records, _, err := state.ReadJSON[[]Record](s.recordsPath(), s.recordsPath()+rowLockSuffix)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: read the fixer records %s: %w", s.recordsPath(), err)
	}
	return records, nil
}

// Live returns the newest record with no terminal status, and nil when there is none.
func (s rowStore) Live() (*Record, error) {
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Status == "" {
			live := records[i]
			return &live, nil
		}
	}
	return nil, nil
}

// Save appends rec, or rewrites the record of rec.StrandGUID.
func (s rowStore) Save(rec *Record) error {
	return s.update(func(records []Record) ([]Record, error) {
		for i := range records {
			if records[i].StrandGUID == rec.StrandGUID {
				records[i] = *rec
				return records, nil
			}
		}
		return append(records, *rec), nil
	})
}

// count returns how many records the set holds, terminal ones included.
func (s rowStore) count() (int, error) {
	records, err := s.load()
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

// markTerminal gives the record of guid the status and dead reason, leaving every other record unchanged.
func (s rowStore) markTerminal(guid, status, deadReason string) error {
	return s.update(func(records []Record) ([]Record, error) {
		for i := range records {
			if records[i].StrandGUID == guid {
				records[i].Status = status
				records[i].DeadReason = deadReason
				return records, nil
			}
		}
		return nil, fmt.Errorf("gatefixer: no fixer record of strand %s in %s", guid, s.recordsPath())
	})
}

func (s rowStore) update(mutate func([]Record) ([]Record, error)) error {
	err := state.UpdateJSON(s.recordsPath(), s.recordsPath()+rowLockSuffix, func(cur []Record, _ bool) ([]Record, error) {
		return mutate(cur)
	})
	if err != nil {
		return fmt.Errorf("gatefixer: write the fixer records %s: %w", s.recordsPath(), err)
	}
	return nil
}

// reportPath is the report file of the fixer at position n in the set, counted from 1.
func (s rowStore) reportPath(n int) string {
	return filepath.Join(s.dir, fmt.Sprintf("report-%d.md", n))
}

// replyPath is the reply file of the fixer at position n in the set, counted from 1.
func (s rowStore) replyPath(n int) string {
	return filepath.Join(s.dir, fmt.Sprintf("reply-%d.md", n))
}
