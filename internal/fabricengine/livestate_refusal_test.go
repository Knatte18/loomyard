// livestate_refusal_test.go gives a cell a way to assert which layer of fabric refused a destructive request.
// The destructive gate (internal/fabricengine/destroy.go) and a verb's pre-flight checks can both refuse a request,
// and a check of err != nil cannot tell them apart.
// RefusedByGate and RefusedBefore pin each refusal to one layer.

package fabricengine_test

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// RefusedByGate reports whether err is a refusal from fabric's destructive gate for the given check.
//
// It compares the check the gate reports against check exactly.
// An error from a pre-flight check does not match.
// RefusedByGate reports false for a nil err.
// A cell that expected a refusal and got success therefore fails on the expectation, not on a nil dereference.
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

// RefusedBefore reports whether err is a refusal from a verb's pre-flight check whose message contains substring.
//
// Pre-flight checks run before a request reaches the destructive gate.
// RefusedBefore reports false for a nil err.
// It also reports false when the message contains "check failed":
// a gate refusal embeds the same reason text, and without this exclusion it would match as a pre-flight refusal.
func RefusedBefore(err error, substring string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, substring) && !strings.Contains(msg, "check failed")
}
