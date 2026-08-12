//go:build integration

// refusal.go gives a cell a way to assert WHICH layer of fabric refused a destructive request,
// rather than merely that some error came back.
// fabric's destructive gate (internal/fabricengine/destroy.go) and a verb's own pre-flight checks
// (e.g. Remove's dirty-worktree message before it ever reaches the gate) can both refuse a request,
// and a cell that only checked "err != nil" could not tell one from the other — the two-kind scheme
// here (RefusedByGate, RefusedBefore) is what lets a cell pin the refusal to a specific layer.

package fabrictest

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// RefusedByGate reports whether err is a refusal from fabric's destructive gate for the given check.
//
// It is built on fabricengine.RefusalOf, the exported accessor onto the gate's own *destructiveRefusal:
// RefusalOf unwraps err via errors.As and, when it finds one, returns a Refusal carrying the exact Check
// that refused. RefusedByGate compares that Refusal.Check against the wanted check directly, rather than
// matching a rendered "<check> check failed" substring — strictly more precise than substring matching,
// and what RefusalOf exists for.
// RefusedByGate reports false for a nil err, so a cell that expected a refusal and instead got a
// success fails on the expectation rather than panicking on a nil dereference.
func RefusedByGate(err error, check fabricengine.Check) bool {
	if err == nil {
		return false
	}
	refusal, ok := fabricengine.RefusalOf(err)
	if !ok {
		return false
	}
	return refusal.Check == check
}

// RefusedBefore reports whether err is a refusal from one of fabric's own pre-flight checks — the
// verb-level guards that run before a request ever reaches the destructive gate — carrying substring
// in its message.
//
// The exclusion of the literal "check failed" is mandatory, not defensive. The gate's dirtiness reason
// at destroy.go:564 is byte-identical to Remove's own pre-flight message at remove.go:74 — both are
// exactly "worktree has uncommitted changes; use --force" — so a gate dirtiness refusal renders as
// "refusing to remove worktree: dirtiness check failed for <target>: worktree has uncommitted changes;
// use --force", which CONTAINS the pre-flight string. Without the exclusion, RefusedBefore would
// report a pre-flight refusal when the gate itself refused, and the layer-pinning property the whole
// two-kind scheme rests on would be false in the pre-flight-to-gate direction.
// RefusedBefore reports false for a nil err, matching RefusedByGate's posture.
func RefusedBefore(err error, substring string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, substring) && !strings.Contains(msg, "check failed")
}
