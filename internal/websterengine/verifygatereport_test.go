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

// TestCardHint proves the hint names, in card order, every card whose commit touched a failing
// package's own directory (a file in a subdirectory or another package is no touch), yields no hint
// and spawns nothing for opaque or foreign-module failures, and propagates a changed-paths error.
//
//testtiming:keep pins which commits count as touching a failing package, the card order of the hint and the no-hint cases; the covering run-level verify-gate test checks one hint
func TestCardHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		failures []VerifyFailure
		shas     []string
		labels   []string
		changed  func(string) ([]string, error)
		want     []string
		wantErr  bool
	}{
		{
			name:     "names the cards that touched a failing package",
			failures: []VerifyFailure{{ID: testModulePath + "/internal/a.TestX", Kind: FailureKindTest, Package: testModulePath + "/internal/a"}},
			shas:     []string{"s1", "s2", "s3"},
			labels:   []string{"01-first", "02-second", "03-third"},
			changed: fakeChangedPaths(map[string][]string{
				"s1": {"internal/a/a.go"},
				"s2": {"internal/b/b.go", "internal/a/sub/deep.go"},
				"s3": {"internal/a/a_test.go"},
			}),
			want: []string{"01-first", "03-third"},
		},
		{
			// A nil changedPaths proves the hint spawns nothing when no directory maps.
			name: "opaque and foreign packages yield no hint",
			failures: []VerifyFailure{
				{ID: opaqueFailureID, Kind: FailureKindOpaque},
				{ID: "other.org/x.TestY", Kind: FailureKindTest, Package: "other.org/x"},
			},
			shas:   []string{"s1"},
			labels: []string{"01-first"},
		},
		{
			name:     "a changed-paths error propagates",
			failures: []VerifyFailure{{ID: testModulePath + "/a", Kind: FailureKindPackage, Package: testModulePath + "/a"}},
			shas:     []string{"s1"},
			labels:   []string{"01-first"},
			changed:  fakeChangedPaths(nil),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cardHint(testModulePath, tt.failures, tt.shas, tt.labels, tt.changed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cardHint() error = %v; want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cardHint() = %v, want %v", got, tt.want)
			}
		})
	}
}

//testtiming:keep pins the fixed verify-gate.yaml name, a second write replacing the first and every report field surviving the YAML round trip; the run-level test reads back only the fields its failure sets
func TestVerifyGateReport_RoundTrip(t *testing.T) {
	reportsDir := t.TempDir()
	path := VerifyGateReportPath(reportsDir)
	if filepath.Base(path) != "verify-gate.yaml" {
		t.Errorf("VerifyGateReportPath() = %q, want file verify-gate.yaml", path)
	}

	want := VerifyGateReport{
		Attempt: 2,
		Cap:     3,
		Failures: []VerifyFailure{
			{ID: "m/a.TestX", Kind: FailureKindTest, Package: "m/a", Tail: "boom"},
			{ID: "m/b", Kind: FailureKindPackage, Package: "m/b", Tail: "build failed"},
		},
		TimedOut:   "1h0m0s",
		LogTail:    "hung in TestSlow",
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
		Failures: []VerifyFailure{
			{ID: "m/a.TestX", Kind: FailureKindTest, Package: "m/a", Tail: "line one\nline two"},
			{ID: "m/b", Kind: FailureKindPackage, Package: "m/b"},
		},
		LogPath: "/scratch/verify.log",
		Hint:    []string{"03-third"},
	}
	const fixPromptPath = "/scratch/prompts/verify-fix.md"
	wayForward := verifyGateWayForward(fixPromptPath)
	if !strings.Contains(wayForward, "fixer fork") || !strings.Contains(wayForward, fixPromptPath) {
		t.Errorf("way forward = %q; want it to name the fixer-fork step and %s", wayForward, fixPromptPath)
	}
	assertOpensWithWayForward := func(findings string) {
		t.Helper()
		if !strings.HasPrefix(findings, wayForward+"\nVerify gate failed (attempt ") {
			t.Errorf("findings should open with the way forward before the failure line:\n%s", findings)
		}
	}

	got := renderVerifyGateFindings(r, fixPromptPath)
	assertOpensWithWayForward(got)
	for _, want := range []string{"attempt 2 of 3", "m/a.TestX", "m/b", "line one", "line two", "/scratch/verify.log", "03-third"} {
		if !strings.Contains(got, want) {
			t.Errorf("findings missing %q:\n%s", want, got)
		}
	}

	r.Hint = nil
	if got := renderVerifyGateFindings(r, fixPromptPath); !strings.Contains(got, "No recorded card touched a failing package") {
		t.Errorf("findings without a hint should say no card touched a failing package:\n%s", got)
	}

	timedOut := renderVerifyGateFindings(VerifyGateReport{Attempt: 1, Cap: 3, TimedOut: "1h0m0s", LogPath: "/scratch/verify.log", LogTail: "hung in TestSlow"}, fixPromptPath)
	assertOpensWithWayForward(timedOut)
	for _, want := range []string{"attempt 1 of 3", "did not finish within 1h0m0s and was killed", "/scratch/verify.log", "hung in TestSlow"} {
		if !strings.Contains(timedOut, want) {
			t.Errorf("timed-out findings missing %q:\n%s", want, timedOut)
		}
	}
	if strings.Contains(timedOut, "Failing identities") {
		t.Errorf("timed-out findings must not list identities:\n%s", timedOut)
	}

	dirty := renderVerifyGateFindings(VerifyGateReport{Attempt: 1, Cap: 3, Dirty: []string{"a.go", "b.txt"}}, fixPromptPath)
	assertOpensWithWayForward(dirty)
	for _, want := range []string{"attempt 1 of 3", "a.go", "b.txt"} {
		if !strings.Contains(dirty, want) {
			t.Errorf("dirty findings missing %q:\n%s", want, dirty)
		}
	}
	if strings.Contains(dirty, "Failing identities") {
		t.Errorf("dirty findings must not list identities:\n%s", dirty)
	}
}
