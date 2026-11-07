// refusalcheck_helper_test.go holds assertRefusalCheck, the refusal assertion the destructive-gate tests share.
// It serves every tier's tests, since the package's one test binary holds them all.

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
