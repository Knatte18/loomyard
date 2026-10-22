// owners.go — the owner records of a board.json and the effective owner of an entry.
//
// An entry's owner is the machine that may start, resume and land its run; ParseOwners reads the records from board bytes and Effective resolves the owner an entry reads as.

package boardengine

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OwnerEntry is what the owner rules read of one board entry.
type OwnerEntry struct {
	// Owner is the entry's recorded owner machine, empty when none is recorded.
	Owner string
	// IsTask reports whether the entry is a task and not a note.
	IsTask bool
	// Done reports whether the entry is marked done.
	Done bool
	// RunHeld reports whether a run holds the entry.
	RunHeld bool
}

// Owners is the owner records of one board.json: the hub's prime owner and each entry's owner facts by slug.
// The zero value is a board with no prime owner and no entry.
type Owners struct {
	// Prime is the machine that owns the hub's prime, empty when none is recorded.
	Prime string
	// Entries maps a slug to its owner facts.
	Entries map[string]OwnerEntry
}

// ParseOwners decodes board.json bytes into Owners.
// Empty input is a board with no entry, and undecodable input is an error.
func ParseOwners(boardJSON []byte) (Owners, error) {
	if len(bytes.TrimSpace(boardJSON)) == 0 {
		return Owners{}, nil
	}
	tasks, err := ParseBoardTasks(boardJSON)
	if err != nil {
		return Owners{}, err
	}
	var record struct {
		PrimeOwner string `json:"prime_owner"`
	}
	if err := json.Unmarshal(boardJSON, &record); err != nil {
		return Owners{}, fmt.Errorf("read prime_owner: %w", err)
	}
	owners := Owners{Prime: record.PrimeOwner, Entries: make(map[string]OwnerEntry, len(tasks))}
	for _, task := range tasks {
		owners.Entries[task.Slug] = OwnerEntry{
			Owner:   task.OwnerMachine,
			IsTask:  task.Kind == KindTask,
			Done:    isDone(task),
			RunHeld: IsRunStatus(task.Status),
		}
	}
	return owners, nil
}

// Effective returns the owner machine the entry slug reads as, where "" means owned by no other machine.
// A board with no prime owner, a slug the board does not hold and a run-held entry with no recorded owner read as "".
// Otherwise the entry's recorded owner stands, and an entry with none reads as its parent's owner.
// parentOf names the slug of the entry's parent pair, or "" for the prime, whose owner is the prime owner; an error from it is returned.
func (o Owners) Effective(slug string, parentOf func(slug string) (string, error)) (string, error) {
	if o.Prime == "" {
		return "", nil
	}
	entry, held := o.Entries[slug]
	if !held {
		return "", nil
	}
	if entry.Owner != "" {
		return entry.Owner, nil
	}
	if entry.RunHeld {
		return "", nil
	}
	parent, err := parentOf(slug)
	if err != nil {
		return "", err
	}
	if parent == "" {
		return o.Prime, nil
	}
	return o.Effective(parent, parentOf)
}

// OwnedElsewhere reports whether owner names a machine other than here; an empty owner is owned by no other machine.
func OwnedElsewhere(owner, here string) bool {
	return owner != "" && owner != here
}
