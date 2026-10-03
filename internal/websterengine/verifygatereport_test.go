package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const testModulePath = "example.com/mod"

func fakeChangedPaths(byCommit map[string][]string) func(string) ([]string, error) {
	return func(sha string) ([]string, error) {
		paths, ok := byCommit[sha]
		if !ok {
			return nil, errors.New("unknown commit " + sha)
		}
		return paths, nil
	}
}

func TestCardHint_NamesCardsThatTouchedAFailingPackage(t *testing.T) {
	failures := []IntegrationFailure{
		{ID: testModulePath + "/internal/a.TestX", Kind: FailureKindTest, Package: testModulePath + "/internal/a"},
	}
	shas := []string{"s1", "s2", "s3"}
	labels := []string{"01-first", "02-second", "03-third"}
	changed := fakeChangedPaths(map[string][]string{
		"s1": {"internal/a/a.go"},
		"s2": {"internal/b/b.go", "internal/a/sub/deep.go"},
		"s3": {"internal/a/a_test.go"},
	})

	got, err := cardHint(testModulePath, failures, shas, labels, changed)
	if err != nil {
		t.Fatalf("cardHint() error = %v", err)
	}
	want := []string{"01-first", "03-third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cardHint() = %v, want %v (a file only in a subdirectory or another package is not a touch)", got, want)
	}
}

func TestCardHint_OpaqueAndForeignPackagesYieldNoHint(t *testing.T) {
	failures := []IntegrationFailure{
		{ID: opaqueFailureID, Kind: FailureKindOpaque},
		{ID: "other.org/x.TestY", Kind: FailureKindTest, Package: "other.org/x"},
	}
	// A nil changedPaths proves the hint spawns nothing when no directory maps.
	got, err := cardHint(testModulePath, failures, []string{"s1"}, []string{"01-first"}, nil)
	if err != nil {
		t.Fatalf("cardHint() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("cardHint() = %v, want no hint", got)
	}
}

func TestCardHint_ChangedPathsErrorPropagates(t *testing.T) {
	failures := []IntegrationFailure{{ID: testModulePath + "/a", Kind: FailureKindPackage, Package: testModulePath + "/a"}}
	_, err := cardHint(testModulePath, failures, []string{"s1"}, []string{"01-first"}, fakeChangedPaths(nil))
	if err == nil {
		t.Fatal("cardHint() error = nil, want the changedPaths error")
	}
}

func TestVerifyGateReport_RoundTrip(t *testing.T) {
	reportsDir := t.TempDir()
	path := VerifyGateReportPath(reportsDir)
	if filepath.Base(path) != "verify-gate.yaml" {
		t.Errorf("VerifyGateReportPath() = %q, want file verify-gate.yaml", path)
	}

	want := VerifyGateReport{
		Attempt: 2,
		Cap:     3,
		Failures: []IntegrationFailure{
			{ID: "m/a.TestX", Kind: FailureKindTest, Package: "m/a", Tail: "boom"},
			{ID: "m/b", Kind: FailureKindPackage, Package: "m/b", Tail: "build failed"},
		},
		LogPath:    "/scratch/verify.log",
		Hint:       []string{"03-third"},
		FixCommits: []string{"abc123", "def456"},
	}
	if err := WriteVerifyGateReport(path, want); err != nil {
		t.Fatalf("WriteVerifyGateReport() error = %v", err)
	}
	// A second write replaces the first.
	if err := WriteVerifyGateReport(path, want); err != nil {
		t.Fatalf("WriteVerifyGateReport() second write error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var got VerifyGateReport
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("report read back = %#v, want %#v", got, want)
	}
}

func TestRenderVerifyGateFindings(t *testing.T) {
	r := VerifyGateReport{
		Attempt: 2,
		Cap:     3,
		Failures: []IntegrationFailure{
			{ID: "m/a.TestX", Kind: FailureKindTest, Package: "m/a", Tail: "line one\nline two"},
			{ID: "m/b", Kind: FailureKindPackage, Package: "m/b"},
		},
		LogPath: "/scratch/verify.log",
		Hint:    []string{"03-third"},
	}
	got := renderVerifyGateFindings(r)
	for _, want := range []string{"attempt 2 of 3", "m/a.TestX", "m/b", "line one", "line two", "/scratch/verify.log", "03-third"} {
		if !strings.Contains(got, want) {
			t.Errorf("findings missing %q:\n%s", want, got)
		}
	}

	r.Hint = nil
	if got := renderVerifyGateFindings(r); !strings.Contains(got, "No recorded card touched a failing package") {
		t.Errorf("findings without a hint should say no card touched a failing package:\n%s", got)
	}

	dirty := renderVerifyGateFindings(VerifyGateReport{Attempt: 1, Cap: 3, Dirty: []string{"a.go", "b.txt"}})
	for _, want := range []string{"attempt 1 of 3", "a.go", "b.txt"} {
		if !strings.Contains(dirty, want) {
			t.Errorf("dirty findings missing %q:\n%s", want, dirty)
		}
	}
	if strings.Contains(dirty, "Failing identities") {
		t.Errorf("dirty findings must not list identities:\n%s", dirty)
	}
}
