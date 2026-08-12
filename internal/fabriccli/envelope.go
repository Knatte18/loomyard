// envelope.go declares the two helpers every mutating verb handler routes its output through:
// okWithRecord for the success path, errWithRecord for the failure path. Both emit the fixed
// "mutations"/"partial" key pair the fabric envelope carries on every verb outcome, so the fixed key
// set is declared once here rather than open-coded at each of fabriccli's mutating call sites.
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

// errWithRecord emits a failure envelope carrying rec's accumulated mutations, the flattened err
// string, and — when err carries a gate refusal — a "refusal" object with exactly the four keys
// check, what, target and reason. It sets fields["mutations"] and fields["partial"] (true when rec is
// non-empty, since err is non-nil by construction here), overriding any caller-supplied "ok", "error",
// "mutations" or "partial" key, then delegates to output.ErrFields. The flattened error string is
// retained unchanged: dropping it would break the "every failure carries an error string" contract
// every other module's envelope holds.
func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int {
	fields := map[string]any{
		"mutations": rec.Entries(),
		"partial":   rec.Len() > 0,
	}
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
