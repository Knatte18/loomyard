// cli_test.go pins the adopt envelope's fields that depend on the outcome of the adopt checks' start;
// it also pins the fields that depend on the reconcile's warnings and on the deploy.

package adoptcli

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAdoptFields(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	started := []suiteStart{{Subject: "root", PID: 42}, {Subject: "alpha", PID: 43}}
	tests := []struct {
		name          string
		result        outcome
		wantMessage   string
		wantWarnings  []string
		wantReconcile []string
		wantChecks    []suiteStart
		wantRuns      []runReport
	}{
		{
			name:        "StartedChecksCarryTheirMessageAndTheChecks",
			result:      outcome{Checks: started, SuiteMessage: "the adopt checks run"},
			wantMessage: "the adopt checks run",
			wantChecks:  started,
		},
		{
			name:         "FailedStartCarriesOnlyTheWarning",
			result:       outcome{CheckWarnings: []string{"the adopt check of alpha was not started"}},
			wantWarnings: []string{"the adopt check of alpha was not started"},
		},
		{
			name:          "ReconcileWarningsAreTheWarningsField",
			result:        outcome{Warnings: []string{"code-alpha is diverged against origin"}},
			wantReconcile: []string{"code-alpha is diverged against origin"},
		},
		{
			name:     "RunsAreAlwaysPresentAndCarryTheReports",
			result:   outcome{Runs: []runReport{{Slug: "t1", State: runStateWaitingForCheck, Line: "t1: waiting for the adopt check"}}},
			wantRuns: []runReport{{Slug: "t1", State: runStateWaitingForCheck, Line: "t1: waiting for the adopt check"}},
		},
		{
			name:   "ARunDeployIsTheDeployField",
			result: outcome{Deploy: &deployOutcome{Installed: "87553450ec14", Head: "2d319fe5", Log: "deploy.log"}},
		},
		{name: "NoCheckCarriesNoneAndAnEmptyRunsArray"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fields := adoptFields(tt.result)

			message, hasMessage := fields["message"]
			if hasMessage != (tt.wantMessage != "") || (hasMessage && message != tt.wantMessage) {
				t.Errorf("adoptFields()[message] = %v, present %v; want %q", message, hasMessage, tt.wantMessage)
			}
			warnings, hasWarnings := fields["check_warnings"]
			if hasWarnings != (tt.wantWarnings != nil) || (hasWarnings && !reflect.DeepEqual(warnings, tt.wantWarnings)) {
				t.Errorf("adoptFields()[check_warnings] = %v, present %v; want %v", warnings, hasWarnings, tt.wantWarnings)
			}
			reconcile, hasReconcile := fields["warnings"]
			if hasReconcile != (tt.wantReconcile != nil) || (hasReconcile && !reflect.DeepEqual(reconcile, tt.wantReconcile)) {
				t.Errorf("adoptFields()[warnings] = %v, present %v; want %v", reconcile, hasReconcile, tt.wantReconcile)
			}
			checks, hasChecks := fields["checks"]
			if hasChecks != (tt.wantChecks != nil) || (hasChecks && !reflect.DeepEqual(checks, tt.wantChecks)) {
				t.Errorf("adoptFields()[checks] = %v, present %v; want %v", checks, hasChecks, tt.wantChecks)
			}
			wantRuns := tt.wantRuns
			if wantRuns == nil {
				wantRuns = []runReport{}
			}
			if runs, hasRuns := fields["runs"]; !hasRuns || !reflect.DeepEqual(runs, wantRuns) {
				t.Errorf("adoptFields()[runs] = %v, present %v; want %v", runs, hasRuns, wantRuns)
			}
			if deploy, hasDeploy := fields["deploy"]; hasDeploy != (tt.result.Deploy != nil) || (hasDeploy && deploy != tt.result.Deploy) {
				t.Errorf("adoptFields()[deploy] = %v, present %v; want %v", deploy, hasDeploy, tt.result.Deploy)
			}
			for _, retired := range []string{"suite", "suite_warning"} {
				if _, present := fields[retired]; present {
					t.Errorf("adoptFields() holds the retired field %q", retired)
				}
			}
		})
	}
}
