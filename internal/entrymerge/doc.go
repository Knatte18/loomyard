// Package entrymerge is the three-way by-entry merge of one record's fields, parameterised by the record's key and field table.
// A Table names the comparable fields of a record type.
// Change applies the fields a commit changed onto origin's entry.
// AddClash judges an entry both sides added.
// Same judges an entry for removal.
// A field only the commit changed is applied, and a field origin already holds is skipped.
// A field origin moved to a third value since the merge base keeps origin's value and is reported lost.
// An origin value equal to the commit's parent counts as unchanged by origin, since it is the state the commit was made on.
// The package holds no record's own checks.
// Ids, handles, dates, unknown keys, write validation and locks stay with the caller.
// The caller merges the table's fields and keeps everything else as origin holds it.
// It imports the standard library only.
package entrymerge
