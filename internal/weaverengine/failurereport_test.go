// failurereport_test.go pins the verify-failure record a failed batch gate run builds from its log.

package weaverengine_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestFailureReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const failedPackage = "FAIL\texample.com/m/p\t0.01s\n"
	triaged := failedPackage + gateslot.FailureReportHeader + "\n" +
		"tier0 (exit 1), log /runs/a.log:\n" +
		"  example.com/m/p TestA:\n" +
		"    " + gateslot.TriageLinePrefix + "not in the coverage maps\n" +
		"    a_test.go:3: boom\n" +
		gateslot.FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	failed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	timedOut := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: "gate tier0 exceeded timeout_min 30m0s and was killed"}

	tests := []struct {
		name   string
		result verifytree.Result
		output string
		want   map[string][]string
	}{
		{
			name: "AnOutputWithTriageLinesFillsTriage", result: failed, output: triaged,
			want: map[string][]string{"example.com/m/p.TestA": {gateslot.TriageLinePrefix + "not in the coverage maps"}},
		},
		{name: "ATimedOutRunLeavesTriageEmpty", result: timedOut, output: triaged},
		{name: "AnOutputWithoutTriageLinesLeavesTriageEmpty", result: failed, output: failedPackage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := weaverengine.FailureReport("lym gate test --wait --gate batch", 1, 2, tt.result, tt.output)
			if diff := cmp.Diff(tt.want, got.Triage); diff != "" {
				t.Errorf("FailureReport().Triage mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
