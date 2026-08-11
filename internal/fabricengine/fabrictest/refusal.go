//go:build integration

// refusal.go gives a cell a way to assert WHICH layer of fabric refused a destructive request,
// rather than merely that some error came back.
// fabric's destructive gate (internal/fabricengine/destroy.go) and a verb's own pre-flight checks
// (e.g. Remove's dirty-worktree message before it ever reaches the gate) can both refuse a request,
// and a cell that only checked "err != nil" could not tell one from the other — the two-kind scheme
// here (RefusedByGate, RefusedBefore) is what lets a cell pin the refusal to a specific layer.

package fabrictest

import "strings"

// Check names one of the three destructive-gate checks a *destructiveRefusal can be attributed to:
// containment, ownership, or dirtiness.
// It is a fabrictest-owned copy of fabricengine's own unexported destructiveCheck enum, string-backed
// so RefusedByGate can match it against an error's rendered message without importing the unexported
// type.
type Check string

const (
	// CheckContainment names the gate's containment check: the target must resolve strictly below
	// its declared container.
	CheckContainment Check = "containment"
	// CheckOwnership names the gate's ownership check: the target must satisfy one of the closed set
	// of ownership predicates declared for the request.
	CheckOwnership Check = "ownership"
	// CheckDirtiness names the gate's dirtiness check: the target must be clean, or the caller must
	// have passed --force.
	CheckDirtiness Check = "dirtiness"

	// There is deliberately no CheckForce constant. checkForce is declared in destroy.go and rendered
	// by destructiveCheck.String(), but it is never constructed into a *destructiveRefusal anywhere in
	// the tree: force is consulted only inside checkPathDirtiness, where it makes the dirtiness check
	// PASS rather than fail, so a refusal is never attributed to it. A CheckForce constant could
	// therefore never match a real refusal — do not add it back believing the gate simply forgot to
	// emit it.
)

// RefusedByGate reports whether err is a refusal from fabric's destructive gate for the given check.
//
// The gate renders every refusal as "refusing to <what>: <check> check failed for <target>: <reason>"
// (destroy.go:70-72), so matching on the substring "<check> check failed" is unambiguous and survives
// call-site wrapping (fmt.Errorf("...: %w", err) and its %v cousin), because RefusedByGate searches the
// full rendered error string rather than unwrapping.
// Matching is done by substring rather than errors.As because the gate's one error type,
// *destructiveRefusal, is unexported and therefore unreachable from this package — the rendered
// message IS the gate's honest-reporting contract, so pinning it here tests something real rather than
// a proxy for it.
// RefusedByGate reports false for a nil err, so a cell that expected a refusal and instead got a
// success fails on the expectation rather than panicking on a nil dereference.
func RefusedByGate(err error, check Check) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), string(check)+" check failed")
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
