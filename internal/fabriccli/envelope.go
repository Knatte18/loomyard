// envelope.go declares the helpers every mutating verb handler routes its output through:
// okWithRecord for the success path, errWithRecord for the failure path, and
// errConflictsWithRecord for the dedicated conflict-result failure path a merge verb takes when
// MergeResult.Conflicts is non-empty. All three emit the fixed "mutations"/"partial" key pair the
// fabric envelope carries on every verb outcome, so the fixed key set is declared once here rather
// than open-coded at each of fabriccli's mutating call sites.
//
// "mutations" is always a JSON array, never null — rec.Entries() never returns nil — and "partial" is
// always a bool, never absent. A consumer therefore never has to distinguish absent from false, and
// the key set does not vary by outcome: that is the property that lets a test assert the shape once
// per verb instead of once per path. partial's one derivation rule is "error != nil AND record
// non-empty" — okWithRecord's success path has no error, so it is unconditionally false there.
//
// internal/fabricengine's read-only verbs (list, pairs, status, diff) deliberately do NOT route
// through these helpers: nothing was mutated, so there is no record to report.
package fabriccli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// okWithRecord emits a success envelope carrying rec's accumulated mutations alongside the caller's
// fields. It sets fields["mutations"] and fields["partial"] = false — unconditionally false, since
// partial's one derivation rule requires a non-nil error and there is none on this path — overriding
// any caller-supplied "ok", "mutations" or "partial" key, then delegates to output.Ok.
func okWithRecord(w io.Writer, rec fabricengine.Mutations, fields map[string]any) int {
	fields["mutations"] = rec.Entries()
	fields["partial"] = false
	return output.Ok(w, fields)
}

// errWithRecord emits a failure envelope carrying rec's accumulated mutations and the flattened err
// string, with no fields of the verb's own. It is errWithRecordFields with an empty field set, and
// exists because most failing handlers have nothing left to report beyond the error.
func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int {
	return errWithRecordFields(w, rec, err, nil)
}

// errWithRecordFields emits a failure envelope carrying rec's accumulated mutations, the flattened
// err string, the caller's own fields, and — when err carries a gate refusal — a "refusal" object
// with exactly the four keys check, what, target and reason. It sets fields["mutations"] and
// fields["partial"] (true when rec is non-empty, since err is non-nil by construction here),
// overriding any caller-supplied "ok", "error", "mutations" or "partial" key, then delegates to
// output.ErrFields. The flattened error string is retained unchanged: dropping it would break the
// "every failure carries an error string" contract every other module's envelope holds.
//
// The fields parameter exists for a verb that fails while still having a full per-item report to
// deliver — `reconcile`, whose per-pair outcomes must reach the caller on the failure path exactly
// as they do on the success path, since a caller learning only "one pair failed" cannot tell which.
// A caller passing nil gets the same envelope errWithRecord has always produced.
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

// errConflictsWithRecord emits the dedicated conflict-result failure envelope a merge verb returns
// when the engine reports conflicts without an error (a nil engine error, MergeResult.Conflicts
// non-empty). It sets fields["mutations"] from rec, fields["partial"] to the literal false — never
// computed, since the Mutation Record Invariant's "error != nil AND record non-empty" rule yields
// false when the engine returned a nil error — and fields["conflicts"] to conflicts, never null.
// It never routes through errWithRecordFields, whose partial = rec.Len() > 0 computation would
// wrongly report true here (the "conflict envelope never goes through errWithRecordFields" Shared
// Decision).
// The error text is the fixed, side-free message every conflict envelope carries, and it names the
// FULL resolve → merge-stage → continue sequence rather than only the final step: --continue gates
// on the git index, so an operator (or agent) following an error text that skipped merge-stage
// edited the files, ran --continue, and looped on "unresolved conflicts remain" forever — and for a
// conflict under a wired junction name plain `git add` cannot substitute at all.
func errConflictsWithRecord(w io.Writer, rec fabricengine.Mutations, conflicts []string) int {
	fields := map[string]any{
		"mutations": rec.Entries(),
		"partial":   false,
		"conflicts": conflicts,
	}
	return output.ErrFields(w, `merge produced conflicts; resolve each listed path, mark it resolved with "lyx fabric merge-stage <path>...", then run "lyx fabric merge --continue"`, fields)
}
