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
	return ownersOf(record.PrimeOwner, tasks), nil
}

// ownersOf builds the Owners of a board holding tasks and recording primeOwner.
func ownersOf(primeOwner string, tasks []Task) Owners {
	owners := Owners{Prime: primeOwner, Entries: make(map[string]OwnerEntry, len(tasks))}
	for _, task := range tasks {
		owners.Entries[task.Slug] = OwnerEntry{
			Owner:   task.OwnerMachine,
			IsTask:  task.Kind == KindTask,
			Done:    isDone(task),
			RunHeld: IsRunStatus(task.Status),
		}
	}
	return owners
}

// parentOfOrPrime returns parentOf, or a resolver that reads every parent as the prime when parentOf is nil.
func parentOfOrPrime(parentOf func(slug string) (string, error)) func(slug string) (string, error) {
	if parentOf != nil {
		return parentOf
	}
	return func(string) (string, error) { return "", nil }
}

// effectiveOwner returns the owner the entry slug reads as over the store's own entries and prime owner.
func (s *Store) effectiveOwner(slug string) (string, error) {
	return ownersOf(s.primeOwner, s.tasks).Effective(slug, parentOfOrPrime(s.parentOf))
}

// checkOwnerPayload judges the owner_machine key of an upsert payload against the entry's effective owner.
// A value equal to it is a no-op and is dropped from fields; any other value is refused, since only the owner setter writes an owner.
// A payload without the key passes untouched.
func checkOwnerPayload(s *Store, fields map[string]any) error {
	value, carried := fields["owner_machine"]
	if !carried {
		return nil
	}
	slug, _ := fields["slug"].(string)
	effective, err := s.effectiveOwner(slug)
	if err != nil {
		return fmt.Errorf("entry %q: resolve its owner: %w", slug, err)
	}
	if proposed, isString := value.(string); isString && proposed == effective {
		delete(fields, "owner_machine")
		return nil
	}
	return fmt.Errorf("entry %q is owned by %q, and a board write never changes an owner: run `lyx adopt <slug>` from this machine's prime to take it over", slug, effective)
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
