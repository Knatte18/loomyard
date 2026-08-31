//go:build integration

// status_mergeinprogress_integration_test.go pins the "merge_in_progress" key on "lyx fabric
// status"'s success envelope across the no-merge and parked-merge cases. The key answers the
// this-pair question only: whether THIS pair has a fabric merge parked awaiting "merge --continue"
// or "merge --abort", never whether some other pair in the hub is mid-merge on this pair's branch.

package fabriccli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestRunCLI_StatusReportsNoMergeInProgressOnACleanPair asserts that "status" on a pair with no
// parked merge reports "merge_in_progress" present and false, alongside the pre-existing "changes"
// key, so a regression that drops or renames "changes" while adding the new key is caught here
// rather than by a distant test.
func TestRunCLI_StatusReportsNoMergeInProgressOnACleanPair(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(status) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	inProgress, present := result["merge_in_progress"]
	if !present {
		t.Fatalf("RunCLI(status) output missing 'merge_in_progress' key; got %v", result)
	}
	if inProgress != false {
		t.Errorf("RunCLI(status) merge_in_progress = %v; want false", inProgress)
	}
	if _, present := result["changes"]; !present {
		t.Errorf("RunCLI(status) output missing 'changes' key; got %v", result)
	}
}

// TestRunCLI_StatusReportsMergeInProgressWhileAMergeIsParked asserts that "status" run from the pair
// holding a parked conflicted merge reports "merge_in_progress" present and true — the assertion
// that proves the field reads the real record rather than a hardcoded constant.
func TestRunCLI_StatusReportsMergeInProgressWhileAMergeIsParked(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var mergeInOut bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &mergeInOut, []string{"merge-in", "feature"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1 (a conflict envelope)\noutput: %s", exitCode, mergeInOut.String())
	}

	var out bytes.Buffer
	exitCode = fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(status) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	inProgress, present := result["merge_in_progress"]
	if !present {
		t.Fatalf("RunCLI(status) output missing 'merge_in_progress' key; got %v", result)
	}
	if inProgress != true {
		t.Errorf("RunCLI(status) merge_in_progress = %v; want true", inProgress)
	}
}
