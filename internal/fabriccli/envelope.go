// envelope.go declares the helpers every mutating verb handler routes its output through.
// okWithRecord emits the success envelope and errWithRecord the failure envelope.
// errConflictsWithRecord emits the failure envelope a merge verb returns when the engine reports conflicts.
// All three emit the fixed "mutations" and "partial" keys that every verb outcome carries.
// The key set is declared here once, not open-coded at each mutating call site in fabriccli.
//
// "mutations" is always a JSON array, never null, because rec.Entries() never returns nil.
// "partial" is always a bool, never absent.
// A consumer therefore never has to distinguish absent from false, and the key set does not vary by outcome.
// That lets a test assert the shape once per verb instead of once per path.
// partial follows one rule: it is true when err is non-nil and the record is non-empty.
// okWithRecord's success path has no error, so partial is always false there.
//
// The read-only verbs of internal/fabricengine do not route through these helpers:
// nothing was mutated, so there is no record to report.
package fabriccli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// okWithRecord emits a success envelope carrying rec's accumulated mutations alongside the caller's fields.
// It sets fields["mutations"] from rec and fields["partial"] to false.
// partial is always false on this path, because its rule needs a non-nil error and there is none here.
// It overrides any caller-supplied "ok", "mutations" or "partial" key, then delegates to output.Ok.
func okWithRecord(w io.Writer, rec fabricengine.Mutations, fields map[string]any) int {
	fields["mutations"] = rec.Entries()
	fields["partial"] = false
	return output.Ok(w, fields)
}

// errWithRecord emits a failure envelope carrying rec's accumulated mutations and the flattened err string.
// It carries no fields of the verb's own, because most failing handlers have nothing to report beyond the error.
func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int {
	return errWithRecordFields(w, rec, err, nil)
}

// errWithRecordFields emits a failure envelope carrying rec's accumulated mutations and the flattened err string.
// It also carries the caller's own fields.
// When err carries a gate refusal, it adds a "refusal" object with exactly four keys: check, what, target and reason.
// It sets fields["mutations"] and fields["partial"], the latter true when rec is non-empty, since err is non-nil here.
// It overrides any caller-supplied "ok", "error", "mutations" or "partial" key, then delegates to output.ErrFields.
// The flattened error string stays in the envelope, because every failure carries an error string.
//
// The fields parameter serves a verb, such as `reconcile`, that fails while still holding a full per-item report.
// Its per-pair outcomes must reach the caller on the failure path just as on the success path:
// a caller told only that one pair failed cannot tell which.
// A nil fields map yields the envelope with the mutations and partial keys and no field of the verb's own.
func errWithRecordFields(w io.Writer, rec fabricengine.Mutations, err error, fields map[string]any) int {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["mutations"] = rec.Entries()
	fields["partial"] = rec.Len() > 0
	if refusal, ok := fabricengine.RefusalOf(err); ok {
		fields["refusal"] = map[string]any{
			"check":  string(refusal.Check),
			"what":   refusal.What,
			"target": refusal.Target,
			"reason": refusal.Reason,
		}
	}
	return output.ErrFields(w, err.Error(), fields)
}

// errConflictsWithRecord emits the failure envelope a merge verb returns when the engine reports conflicts.
// That is a nil engine error with MergeResult.Conflicts non-empty.
// It sets fields["mutations"] from rec and fields["conflicts"] to conflicts, which is never null.
// It sets fields["partial"] to the literal false, because partial needs an error and the engine returned none.
// A helper that computes partial from the record would report true here.
//
// The error text is fixed and names the full sequence: resolve, merge-stage, then continue.
// --continue checks the git index, so an error text that skips merge-stage leaves conflicts unresolved.
// For a conflict under a wired junction name, plain git add cannot substitute for merge-stage.
//
// warnings adds a "warnings" string array only when non-empty,
// so a nil or empty slice leaves the envelope unchanged.
func errConflictsWithRecord(w io.Writer, rec fabricengine.Mutations, conflicts []string, warnings []string) int {
	fields := map[string]any{
		"mutations": rec.Entries(),
		"partial":   false,
		"conflicts": conflicts,
	}
	if len(warnings) > 0 {
		fields["warnings"] = warnings
	}
	return output.ErrFields(w, `merge produced conflicts; resolve each listed path, mark it resolved with "lym fabric merge-stage <path>...", then run "lym fabric merge --continue"`, fields)
}
