// bisect_test.go drives bisect's identity-based pass predicate through a fake FabricBisector and a fake verifyRunner keyed by the checked-out SHA,
// so localization is tested without spawning a process.

package websterengine

import (
	"reflect"
	"testing"
)

const bisectPkg = "example.com/p"

// bisectRunner returns a verifyRunner answering from runs, keyed by repo's current checkout.
func bisectRunner(repo *triageFakeRepo, runs map[string]verifyRun) verifyRunner {
	return func(cmd, worktree, logPath string) (verifyRun, error) {
		return runs[repo.current], nil
	}
}

func failing(tests ...string) verifyRun {
	return verifyRun{Passed: false, Output: failOutput(bisectPkg, tests...)}
}

func TestBisect_PreExistingFailureDoesNotBlameFirstCard(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": failing("TestOld"),
		"s2": failing("TestOld", "TestNew"),
		"s3": failing("TestOld", "TestNew"),
	}
	idx, err := bisect(repo, []string{"s1", "s2", "s3"}, "v", "/w", []string{bisectPkg + ".TestNew"}, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("idx = %d; want 1 (the card introducing the regression)", idx)
	}
}

func TestBisect_AbsentIdentityCountsAsPassing(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": {Passed: true},
		"s2": failing("TestOther"),
		"s3": failing("TestNew"),
	}
	idx, err := bisect(repo, []string{"s1", "s2", "s3"}, "v", "/w", []string{bisectPkg + ".TestNew"}, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 2 {
		t.Errorf("idx = %d; want 2", idx)
	}
}

func TestBisect_OpaqueRegressionsUseExitCode(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": {Passed: true},
		"s2": {Passed: false, Output: "no parsable failures"},
		"s3": {Passed: false, Output: "no parsable failures"},
	}
	idx, err := bisect(repo, []string{"s1", "s2", "s3"}, "v", "/w", []string{opaqueFailureID}, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("idx = %d; want 1", idx)
	}
}

func TestBisect_NilRegressionsUseExitCode(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": {Passed: true},
		"s2": failing("TestX"),
	}
	idx, err := bisect(repo, []string{"s1", "s2"}, "v", "/w", nil, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("idx = %d; want 1", idx)
	}
}

func TestBisect_LastSHAWithoutRegressionIsUnknown(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": failing("TestOld"),
		"s2": failing("TestOld"),
	}
	idx, err := bisect(repo, []string{"s1", "s2"}, "v", "/w", []string{bisectPkg + ".TestNew"}, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if idx != -1 {
		t.Errorf("idx = %d; want -1 (unknown)", idx)
	}
}

func TestBisect_RestoresBranch(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": {Passed: true},
		"s2": failing("TestNew"),
	}
	if _, err := bisect(repo, []string{"s1", "s2"}, "v", "/w", []string{bisectPkg + ".TestNew"}, bisectRunner(repo, runs)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repo.restores, []string{"main"}) {
		t.Errorf("restores = %v; want [main]", repo.restores)
	}
}

func TestLocalizeIntegrationFailure_NamesLabel(t *testing.T) {
	repo := &triageFakeRepo{branch: "main"}
	runs := map[string]verifyRun{
		"s1": failing("TestOld"),
		"s2": failing("TestOld", "TestNew"),
	}
	card, sha, err := localizeIntegrationFailure(repo, []string{"s1", "s2"}, []string{"01-a", "02-b"}, "v", "/w", []string{bisectPkg + ".TestNew"}, bisectRunner(repo, runs))
	if err != nil {
		t.Fatal(err)
	}
	if card != "02-b" || sha != "s2" {
		t.Errorf("got (%q, %q); want (02-b, s2)", card, sha)
	}
}
