// registry.go — the machine registry and the adopt holds.
// They are the two top-level records of a board.json that no entry carries.
//
// The registry maps a machine id to its current name and the names it replaced;
// ReadsAsHere is the one comparison of a recorded machine name with this machine.
// A hold records an adopt in flight for the whole hub; HoldOf answers whether it holds a slug.

package boardengine

import (
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/machinename"
)

const (
	// HoldKindAdopt is the kind of a hold that stands from an adopt's takeover until its check turns green.
	HoldKindAdopt = "adopt"
	// HoldKindAdoptFix is the kind of a hold that stands while an adopt's fix task is open.
	// It holds every slug but the fix task's own.
	HoldKindAdoptFix = "adopt-fix"
)

// Registry maps a machine id to the machine's entry.
// A nil Registry is a board that registered no machine.
type Registry map[string]MachineEntry

// MachineEntry is one machine's record in the registry.
// Unknown holds the keys this binary does not know, written back after the known ones in key order.
type MachineEntry struct {
	// Name is the machine's current name.
	Name string `json:"name"`
	// Former lists the names the machine replaced.
	Former  []string                   `json:"former,omitempty"`
	Unknown map[string]json.RawMessage `json:"-"`
}

// machineEntryKeys is the set of lower-cased JSON keys MachineEntry declares.
var machineEntryKeys = jsonKeySet(reflect.TypeFor[MachineEntry]())

// MarshalJSON encodes the known fields, then the Unknown members after them in key order.
func (e MachineEntry) MarshalJSON() ([]byte, error) {
	type plain MachineEntry
	object, err := json.Marshal(plain(e))
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, e.Unknown)
}

// UnmarshalJSON decodes the known fields and keeps every other key in Unknown.
func (e *MachineEntry) UnmarshalJSON(data []byte) error {
	type plain MachineEntry
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = MachineEntry(decoded)
	e.Unknown = unknownMembers(data, machineEntryKeys)
	return nil
}

// Resolve returns the entry whose current name is name, else the entry whose former names hold it, with its id.
// Found is false when no entry names it and for an empty name.
// Several entries holding the name as a former name resolve to the one with the smallest id.
func (r Registry) Resolve(name string) (id string, entry MachineEntry, found bool) {
	if name == "" {
		return "", MachineEntry{}, false
	}
	ids := make([]string, 0, len(r))
	for candidate := range r {
		ids = append(ids, candidate)
	}
	slices.Sort(ids)
	for _, candidate := range ids {
		if r[candidate].Name == name {
			return candidate, r[candidate], true
		}
	}
	for _, candidate := range ids {
		if slices.Contains(r[candidate].Former, name) {
			return candidate, r[candidate], true
		}
	}
	return "", MachineEntry{}, false
}

// ReadsAsHere reports whether the recorded machine name reads as the machine here.
// A name an entry resolves reads as here only when that entry's id is here's id,
// so a former name of a registered machine reads as that machine.
// A name no entry resolves reads as here when it is here's current or a former name.
// An empty name reads as no machine.
func (r Registry) ReadsAsHere(name string, here machinename.Identity) bool {
	if name == "" {
		return false
	}
	if id, _, found := r.Resolve(name); found {
		return id == here.ID
	}
	return name == here.Name || slices.Contains(here.Former, name)
}

// Hold is one adopt hold in a board.json: an adopt in flight that holds runs hub-wide.
// Unknown holds the keys this binary does not know, written back after the known ones in key order.
type Hold struct {
	// Kind is HoldKindAdopt or HoldKindAdoptFix.
	Kind string `json:"kind"`
	// Machine is the name of the machine that wrote the hold.
	Machine string `json:"machine"`
	// At is when the hold was written.
	At time.Time `json:"at"`
	// Subjects lists the adopted subjects the hold covers.
	Subjects []HoldSubject `json:"subjects"`
	// FixTask is the slug of the fix task an adopt-fix hold stands over; empty for another kind.
	FixTask string `json:"fix_task,omitempty"`
	// FixMachine is the machine whose check filed FixTask; empty on a hold whose Machine filed it.
	FixMachine string                     `json:"fix_machine,omitempty"`
	Unknown    map[string]json.RawMessage `json:"-"`
}

// HoldSubject is one adopted subject of a hold.
type HoldSubject struct {
	// Subject is what `lym adopt` took: a beam project's slug or the root subject.
	Subject string `json:"subject"`
	// PriorOwner is the subject's owner before the adopt, empty when unknown.
	PriorOwner string `json:"prior_owner,omitempty"`
	// Tries is the subject's fix-try count under an adopt-fix hold.
	Tries int `json:"tries,omitempty"`
}

// holdKeys is the set of lower-cased JSON keys Hold declares.
var holdKeys = jsonKeySet(reflect.TypeFor[Hold]())

// MarshalJSON encodes the known fields, then the Unknown members after them in key order.
func (h Hold) MarshalJSON() ([]byte, error) {
	type plain Hold
	object, err := json.Marshal(plain(h))
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, h.Unknown)
}

// UnmarshalJSON decodes the known fields and keeps every other key in Unknown.
func (h *Hold) UnmarshalJSON(data []byte) error {
	type plain Hold
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*h = Hold(decoded)
	h.Unknown = unknownMembers(data, holdKeys)
	return nil
}

// HoldOf returns the first hold that holds slug.
// Every hold holds every slug, except an adopt-fix hold, which does not hold its fix task.
func (o Owners) HoldOf(slug string) (Hold, bool) {
	for _, hold := range o.Holds {
		if hold.Kind == HoldKindAdoptFix && hold.FixTask == slug {
			continue
		}
		return hold, true
	}
	return Hold{}, false
}
