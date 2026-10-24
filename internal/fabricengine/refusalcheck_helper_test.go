// refusalcheck_helper_test.go holds assertRefusalCheck, the destructive-gate refusal assertion the tier0 and tier1 gate tests share.
// It carries no build tag, so it compiles into every tag set.

package fabricengine

import (
	"errors"
	"testing"
)

// assertRefusalCheck fails the test unless err is a *destructiveRefusal carrying want as its Check.
func assertRefusalCheck(t *testing.T, err error, want Check) {
	t.Helper()
	var refusal *destructiveRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v (%T); want a *destructiveRefusal", err, err)
	}
	if refusal.Check != want {
		t.Errorf("refusal.Check = %s; want %s", refusal.Check, want)
	}
}
