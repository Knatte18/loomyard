// owners.go — the owner records of a board.json and the effective owner of an entry.
//
// An entry's owner is the machine that may start, resume and land its run; ParseOwners reads the records from board bytes and Effective resolves the owner an entry reads as.

package boardengine

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RootSubject is what `lym adopt` takes to move the ownership of the root project, the one the board's prime owner records.
const RootSubject = "root"

// ProjectRecipe is the recipe of a beam project's run, the only recipe a project carries.
const ProjectRecipe = "pawl"

// OwnerEntry is what the owner rules read of one board entry.
type OwnerEntry struct {
	// Owner is the recorded owner machine of a beam project, empty when none is recorded and for every other entry.
	Owner string
	// IsTask reports whether the entry is a task and not a note or a project.
	IsTask bool
	// IsProject reports whether the entry is a project.
	IsProject bool
	// Beam reports whether the entry is a project that is a beam.
	Beam bool
	// BeamColor is the colour a beam project records, empty when none is recorded and for every other entry.
	BeamColor string
	// Done reports whether the entry is marked done.
	Done bool
	// RunHeld reports whether a run status stands on the entry; it does not change whose owner the entry reads.
	RunHeld bool
}

// isBeamProject reports whether the entry is a project that is a beam.
func (e OwnerEntry) isBeamProject() bool {
	return e.IsProject && e.Beam
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
// Only a beam project's recorded owner is read, so a stray owner recorded on any other entry is never judged.
func ownersOf(primeOwner string, tasks []Task) Owners {
	owners := Owners{Prime: primeOwner, Entries: make(map[string]OwnerEntry, len(tasks))}
	for _, task := range tasks {
		entry := OwnerEntry{
			IsTask:    task.Kind == KindTask,
			IsProject: task.Kind == KindProject,
			Beam:      task.Beam,
			Done:      isDone(task),
			RunHeld:   IsRunStatus(task.Status),
		}
		if entry.isBeamProject() {
			entry.Owner = task.OwnerMachine
			entry.BeamColor = task.BeamColor
		}
		owners.Entries[task.Slug] = entry
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
	subject, err := ownersOf(s.primeOwner, s.tasks).AdoptSubject(slug, parentOfOrPrime(s.parentOf))
	if err != nil {
		return fmt.Errorf("entry %q: resolve what adopt takes: %w", slug, err)
	}
	return fmt.Errorf("entry %q is owned by %q, and a board write never changes an owner: run `lym adopt %s` from this machine's prime to take it over", slug, effective, subject)
}

// Effective returns the owner machine the entry slug reads as, where "" means owned by no other machine.
// A slug the board does not hold reads as "".
// A beam project reads its recorded owner, else its parent's; every other entry reads its parent's owner, and a task or grouping project records none.
// parentOf names the slug of the entry's parent pair, or "" for the root, whose owner is the prime owner; an error from it is returned.
func (o Owners) Effective(slug string, parentOf func(slug string) (string, error)) (string, error) {
	entry, held := o.Entries[slug]
	if !held {
		return "", nil
	}
	if entry.isBeamProject() && entry.Owner != "" {
		return entry.Owner, nil
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

// AdoptSubject returns what `lym adopt` takes to move the ownership of the entry slug: the slug itself for a beam project, else the nearest beam project above it, else RootSubject.
// A slug the board does not hold is its own subject.
// parentOf is read as for Effective, and an error from it is returned.
func (o Owners) AdoptSubject(slug string, parentOf func(slug string) (string, error)) (string, error) {
	entry, held := o.Entries[slug]
	if !held || entry.isBeamProject() {
		return slug, nil
	}
	parent, err := parentOf(slug)
	if err != nil {
		return "", err
	}
	if parent == "" {
		return RootSubject, nil
	}
	return o.AdoptSubject(parent, parentOf)
}

// OwnedElsewhere reports whether owner names a machine other than here; an empty owner is owned by no other machine.
func OwnedElsewhere(owner, here string) bool {
	return owner != "" && owner != here
}
