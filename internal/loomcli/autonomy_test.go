// autonomy_test.go — tier 0 tests for the setting lookup that answers the keys a run's autonomy forces.

package loomcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAutonomySettingLookup pins what the wrapper answers for a forced key, an unforced key and a failed lookup.
// The stub lookup answers raddle.auto_approve false, as a profile's raddle block leaves it.
// It also pins that autonomy is read on each forced key's call.
func TestAutonomySettingLookup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	errStub := errors.New("stub lookup failed")
	stub := func(module, key string) (any, error) {
		switch module + "." + key {
		case "loom.raddle.auto_approve":
			return false, nil
		case "drift.judge_interactive":
			return true, nil
		case "loom.raddle.model":
			return "opus[high]", nil
		}
		return nil, errStub
	}
	tests := []struct {
		name       string
		autonomous bool
		module     string
		key        string
		want       any
		wantErr    error
	}{
		{"AutonomyForcesRaddleAutoApprove", true, "loom", "raddle.auto_approve", true, nil},
		{"AutonomyForcesJudgeInteractiveOff", true, "drift", "judge_interactive", false, nil},
		{"AutonomyLeavesAnUnforcedKeyToTheLookup", true, "loom", "raddle.model", "opus[high]", nil},
		{"AnUnforcedKeyErrorPassesThrough", true, "loom", "code.review_ask_parent", nil, errStub},
		{"NoAutonomyAnswersTheLookupsRaddleAutoApprove", false, "loom", "raddle.auto_approve", false, nil},
		{"NoAutonomyAnswersTheLookupsJudgeInteractive", false, "drift", "judge_interactive", true, nil},
		{"AForcedKeyErrorIsNotReachedUnderAutonomy", true, "loom", "pr.review_ask_parent", true, nil},
		{"NoAutonomyPassesAForcedKeyErrorThrough", false, "loom", "pr.review_ask_parent", nil, errStub},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lookup := autonomySettingLookup(stub, func() bool { return tt.autonomous })

			got, err := lookup(tt.module, tt.key)

			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("lookup(%q, %q) = %v, %v; want %v, %v", tt.module, tt.key, got, err, tt.want, tt.wantErr)
			}
		})
	}

	t.Run("AutonomyIsReadOnEachForcedCall", func(t *testing.T) {
		t.Parallel()
		on := false
		lookup := autonomySettingLookup(stub, func() bool { return on })
		if got, _ := lookup("loom", "raddle.auto_approve"); got != false {
			t.Errorf("lookup with autonomy off = %v; want the lookup's false", got)
		}
		on = true
		if got, _ := lookup("loom", "raddle.auto_approve"); got != true {
			t.Errorf("lookup with autonomy turned on = %v; want the forced true", got)
		}
	})
}
