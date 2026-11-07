// entrymerge.go holds the field table and the three-way merge, clash and removal judgments over it.

package entrymerge

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// The reasons a Finding gives.
const (
	reasonChangedToo = "origin changed the field too"
	reasonAddedToo   = "origin added an entry with this slug too"
)

// Field is one comparable field of a record.
// Name is its JSON name.
// Get is a canonical getter that reads nil and empty alike.
// Set copies the field from another record into dst.
type Field[T any] struct {
	Name string
	Get  func(T) any
	Set  func(dst *T, src T)
}

// Table is the key and the merged fields of a record type.
// Fields are in the order a change reports them.
type Table[T any] struct {
	Key    func(T) string
	Fields []Field[T]
}

// Finding is one field of one entry the merge set aside.
// Local is the commit's value of the field, rendered as JSON.
type Finding struct {
	Entry  string
	Field  string
	Reason string
	Local  string
}

// Changed returns the fields whose values differ between before and commit, in table order.
// before is the record at the commit's parent.
func (t Table[T]) Changed(before, commit T) []Field[T] {
	var changed []Field[T]
	for _, field := range t.Fields {
		if !reflect.DeepEqual(field.Get(before), field.Get(commit)) {
			changed = append(changed, field)
		}
	}
	return changed
}

// Change applies the fields the commit changed against before onto origin's entry.
// It returns the merged entry, the names of the fields applied, and the changes set aside.
// merged starts as origin.
// The caller passes a deep copy when the record holds reference fields it must not share with origin's own.
// A changed field that origin already holds at the commit's value is skipped.
// A changed field whose origin value is neither the merge base's nor before's is reported lost.
// It keeps origin's value; the finding carries the commit's value as Local.
// Every other changed field is set from the commit and named in applied.
func (t Table[T]) Change(base, before, origin, commit T) (merged T, applied []string, lost []Finding) {
	merged = origin
	for _, field := range t.Changed(before, commit) {
		current, local := field.Get(origin), field.Get(commit)
		if reflect.DeepEqual(current, local) {
			continue
		}
		if !reflect.DeepEqual(current, field.Get(base)) && !reflect.DeepEqual(current, field.Get(before)) {
			lost = append(lost, Finding{Entry: t.Key(commit), Field: field.Name, Reason: reasonChangedToo, Local: Render(local)})
			continue
		}
		field.Set(&merged, commit)
		applied = append(applied, field.Name)
	}
	return merged, applied, lost
}

// AddClash reports each field the commit's added entry carries differently from an entry origin added under the same key.
func (t Table[T]) AddClash(origin, commit T) []Finding {
	var clashes []Finding
	for _, field := range t.Fields {
		if local := field.Get(commit); !reflect.DeepEqual(field.Get(origin), local) {
			clashes = append(clashes, Finding{Entry: t.Key(commit), Field: field.Name, Reason: reasonAddedToo, Local: Render(local)})
		}
	}
	return clashes
}

// Same reports whether every field of a and b is equal.
func (t Table[T]) Same(a, b T) bool {
	for _, field := range t.Fields {
		if !reflect.DeepEqual(field.Get(a), field.Get(b)) {
			return false
		}
	}
	return true
}

// Render renders a field value as JSON, or as fmt.Sprint renders it when it has no JSON form.
func Render(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}
