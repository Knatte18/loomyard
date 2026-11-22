// reports_test.go covers the report chain: steps judged in the background write nothing until the call writes them.

package gatecli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReportChain pins that a chain's judged reports reach the gate directory only when the call writes them.
// The written reports carry the gate key, and the steps' lines print in step order.
func TestReportChain(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "work"}
	var stderr bytes.Buffer
	exe := &execution{ctx: context.Background(), location: location, request: testRequest{gate: "reported", stderr: &stderr}}
	chain := exe.startReports(exe.ctx, reportSteps{reach: true, redundancy: true, mutation: true}, false)
	judged := chain.wait()
	paths := []string{reachReportPath(location), redundancyReportPath(location), mutationReportPath(location)}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) error = %v before the write; want not exist", path, err)
		}
	}
	judged.write(&stderr)

	want := "gate test: reach report: " + noteNoHub + "; written to " + paths[0] + "\n" +
		"gate test: redundancy report: " + noteNoHub + "; written to " + paths[1] + "\n" +
		"gate test: mutation report: " + noteNoHub + "; written to " + paths[2] + "\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q; want %q", stderr.String(), want)
	}
	reach, found, err := ReadReachReport(location)
	if err != nil || !found || reach.GateKey != "reported" {
		t.Errorf("ReadReachReport() = gate key %q, found %v, err %v; want gate key reported", reach.GateKey, found, err)
	}
	redundancy, found, err := readRedundancyReport(paths[1])
	if err != nil || !found || redundancy.GateKey != "reported" {
		t.Errorf("readRedundancyReport() = gate key %q, found %v, err %v; want gate key reported", redundancy.GateKey, found, err)
	}
	mutation, found, err := ReadMutationReport(location)
	if err != nil || !found || mutation.GateKey != "reported" {
		t.Errorf("ReadMutationReport() = gate key %q, found %v, err %v; want gate key reported", mutation.GateKey, found, err)
	}
}

// TestJudgedReportsGuard pins that the report steps' memory guard leaves a running tier command to the tier's own guard.
func TestJudgedReportsGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	commands := &runningCommands{}
	commands.add(10)
	guard := (&judgedReports{tierCommands: commands}).guard()

	if guard.foreign == nil || !guard.foreign(10) || guard.foreign(11) {
		t.Errorf("guard.foreign(10), guard.foreign(11) = want true for the running tier command and false for another pid")
	}
}

// TestReportStepsOf pins which steps a gate key's reports list names, and that any is set only when one is named.
func TestReportStepsOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		reports []string
		want    reportSteps
		wantAny bool
	}{
		{"NoReportsNameNoStep", nil, reportSteps{}, false},
		{"EachListedStepIsSet", []string{"reach", "mutation"}, reportSteps{reach: true, mutation: true}, true},
		{"AnUnknownNameIsNoStep", []string{"other"}, reportSteps{}, false},
		{"AllThreeStepsAreSet", []string{"reach", "redundancy", "mutation"}, reportSteps{reach: true, redundancy: true, mutation: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := reportStepsOf(tt.reports)
			if got != tt.want {
				t.Errorf("reportStepsOf(%q) = %+v; want %+v", tt.reports, got, tt.want)
			}
			if got.any() != tt.wantAny {
				t.Errorf("reportStepsOf(%q).any() = %v; want %v", tt.reports, got.any(), tt.wantAny)
			}
		})
	}
}

// TestLastTierStart pins that the report steps start beside the first run of the plan's highest tier.
func TestLastTierStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		tiers []int
		want  int
	}{
		{"AnEmptyPlanHasNoStart", nil, -1},
		{"OneRunStartsAtZero", []int{3}, 0},
		{"TheFirstRunOfTheHighestTierStarts", []int{0, 1, 2, 2}, 2},
		{"TheHighestTierMayLeadThePlan", []int{2, 0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var plan []tierRun
			for _, number := range tt.tiers {
				plan = append(plan, tierRun{number: number})
			}
			if got := lastTierStart(plan); got != tt.want {
				t.Errorf("lastTierStart(tiers %v) = %d; want %d", tt.tiers, got, tt.want)
			}
		})
	}
}
