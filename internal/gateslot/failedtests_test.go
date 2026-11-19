// failedtests_test.go covers the read of a failure report's failed tests and the call that runs them alone.

package gateslot

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPackagesFailedOutsideTests pins that only a report's failed-outside-any-test lines name packages, each once.
func TestPackagesFailedOutsideTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return "FAIL\n" + FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" +
			FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	}
	tests := []struct {
		name string
		log  string
		want []string
	}{
		{
			name: "packages failed outside any test across runs, each once, beside a failed test",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA:",
				"  example.com/m/b, failed outside any test:",
				"    b_test.go:3: undefined: x",
				"tier1 (exit 1), log /runs/b.log:",
				"  example.com/m/b, failed outside any test:",
				"  example.com/m/c, failed outside any test:",
			),
			want: []string{"example.com/m/b", "example.com/m/c"},
		},
		{
			name: "a failure's own output naming the suffix is no package",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "    example.com/m/z, failed outside any test:"),
		},
		{
			name: "a line past the report's cut is not read",
			log:  report("tier0 (exit 1), log /runs/a.log:", ReportCutPrefix+"3 more lines in the log", "  example.com/m/b, failed outside any test:"),
		},
		{
			name: "a log with no report names none",
			log:  "FAIL\texample.com/m/b [build failed]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PackagesFailedOutsideTests(tt.log); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PackagesFailedOutsideTests() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestFailedTests pins which failure reports name their failed tests: only one whose every failure is a named test.
func TestFailedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return "=== RUN   TestA\nFAIL\n" + FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" +
			FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	}
	tests := []struct {
		name   string
		log    string
		want   []FailedTest
		wantOK bool
	}{
		{
			name: "tests of a tier and a guard run each keep their run's tier, a subtest folded into its test once",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA/sub:",
				"    a_test.go:3: got 1; want 2",
				"  example.com/m/a TestA/other:",
				"  example.com/m/b TestB:",
				"guard tier1 (exit 1), log /runs/g.log:",
				"  example.com/m/g TestGuard:",
				"gate test: the change touches a/a.go",
			),
			want:   []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}, {Tier: 0, Package: "example.com/m/b", Test: "TestB"}, {Tier: 1, Package: "example.com/m/g", Test: "TestGuard"}},
			wantOK: true,
		},
		{
			name: "triage lines under a failed test change nothing about the tests the report names",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA/sub:",
				"    "+TriageLinePrefix+"not in the coverage maps",
				"    a_test.go:3: got 1; want 2",
				"  example.com/m/b TestB:",
				"    "+TriageLinePrefix+"reaches none of this run's changes; map commit c1",
				"    "+TriageLinePrefix+"lym gate reach --test b.TestB --since <last green commit>",
			),
			want:   []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}, {Tier: 0, Package: "example.com/m/b", Test: "TestB"}},
			wantOK: true,
		},
		{
			name: "a log with no report names none",
			log:  "--- FAIL: TestA (0.00s)\nFAIL\texample.com/m/a\t0.1s\n",
		},
		{
			name: "a failure outside any test names none",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "  example.com/m/b, failed outside any test:", "    b.go:3: undefined: x"),
		},
		{
			name: "a build step names none",
			log:  report("build (exit 2), log /runs/b.log:", "  its output:", "    main.go:1: syntax error"),
		},
		{
			name: "a vet run names none",
			log:  report("vet tier1 (exit 1), log /runs/v.log:", "  example.com/m/a TestA:"),
		},
		{
			name: "a timed-out run's cause line names none",
			log:  report("tier1 (timed out), log /runs/a.log:", "  lym gate test: tier timeout: tier1 exceeded timeout_min 30m0s; find the hang"),
		},
		{
			name: "a run that names no failed test beside one that does names none",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  no failing test or error line was read from its output; its log holds the cause", "tier1 (exit 1), log /runs/b.log:", "  example.com/m/a TestA:"),
		},
		{
			name: "a cut report names none",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "... 12 more lines cut (tier0: 12); the logs named above hold the cut lines of a run with a log"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := FailedTests(tt.log)
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FailedTests() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestNamedFailedTests pins that every named failed test of a report is read, whatever else the report holds.
// A test below the report's cut line is not read.
func TestNamedFailedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return "=== RUN   TestA\nFAIL\n" + FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" +
			FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	}
	tests := []struct {
		name string
		log  string
		want []FailedTest
	}{
		{
			name: "tests of a tier and a guard run keep their run's tier, a subtest folded into its test once",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA/sub:",
				"    a_test.go:3: got 1; want 2",
				"  example.com/m/a TestA/other:",
				"guard tier1 (exit 1), log /runs/g.log:",
				"  example.com/m/g TestGuard:",
			),
			want: []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}, {Tier: 1, Package: "example.com/m/g", Test: "TestGuard"}},
		},
		{
			name: "a test beside a cut line is read, and a test below the cut line is not",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA:",
				ReportCutPrefix+"12 more lines cut (tier0: 12); the logs named above hold the cut lines of a run with a log",
				"  example.com/m/b TestB:",
			),
			want: []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}},
		},
		{
			name: "a test beside another package's failure outside any test is read",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/b"+ReportOutsideAnyTestSuffix+":",
				"    b.go:3: undefined: x",
				"  example.com/m/a TestA:",
			),
			want: []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}},
		},
		{
			name: "a test beside a cause line and unparsed output is read",
			log: report(
				"tier1 (timed out), log /runs/a.log:",
				"  lym gate test: tier timeout: tier1 exceeded timeout_min 30m0s; find the hang",
				"  "+ReportOutputLine,
				"    stray line",
				"  example.com/m/a TestA:",
			),
			want: []FailedTest{{Tier: 1, Package: "example.com/m/a", Test: "TestA"}},
		},
		{
			name: "a test under a run that is no tier or guard run has tier -1",
			log:  report("vet tier1 (exit 1), log /runs/v.log:", "  example.com/m/a TestA:"),
			want: []FailedTest{{Tier: -1, Package: "example.com/m/a", Test: "TestA"}},
		},
		{name: "a log with no report names none", log: "--- FAIL: TestA (0.00s)\nFAIL\texample.com/m/a\t0.1s\n"},
		{name: "a report that names no test names none", log: report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/b"+ReportOutsideAnyTestSuffix+":")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NamedFailedTests(tt.log); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedFailedTests() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestTriageLines pins which triage lines a failure report yields per failed test, and that a log with none yields none.
func TestTriageLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return "=== RUN   TestA\nFAIL\n" + FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" +
			FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	}
	notMapped := TriageLinePrefix + "not in the coverage maps"
	tests := []struct {
		name string
		log  string
		want map[string][]string
	}{
		{
			name: "triage lines under two tests, a subtest named by its path, keep their prefix and their order",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA/sub:",
				"    "+notMapped,
				"    a_test.go:3: got 1; want 2",
				"  example.com/m/b TestB:",
				"    "+TriageLinePrefix+"reaches none of this run's changes; map commit c1",
				"    "+TriageLinePrefix+"lym gate reach --test b.TestB --since <last green commit>",
				"guard tier1 (exit 1), log /runs/g.log:",
				"  example.com/m/g TestGuard:",
				"    "+notMapped,
			),
			want: map[string][]string{
				"example.com/m/a.TestA/sub": {notMapped},
				"example.com/m/b.TestB": {
					TriageLinePrefix + "reaches none of this run's changes; map commit c1",
					TriageLinePrefix + "lym gate reach --test b.TestB --since <last green commit>",
				},
				"example.com/m/g.TestGuard": {notMapped},
			},
		},
		{
			name: "a report with no triage line yields none",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "    a_test.go:3: boom"),
		},
		{
			name: "a triage-shaped line under a failure outside any test belongs to no test",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/b, failed outside any test:", "    "+notMapped),
		},
		{
			name: "a log with no report yields none",
			log:  "--- FAIL: TestA (0.00s)\n    " + notMapped + "\nFAIL\texample.com/m/a\t0.1s\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TriageLines(tt.log); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TriageLines() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestTierRunFailures pins which failure reports list tier and guard runs alone, and how each failure is named.
func TestTierRunFailures(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return "FAIL\n" + FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" +
			FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\n"
	}
	tests := []struct {
		name   string
		log    string
		want   []string
		wantOK bool
	}{
		{
			name: "every failure of tier and guard runs is named with its run's label, a run naming none by its outcome, up to a cut",
			log: report(
				"tier0 (exit 1), log /runs/a.log:",
				"  example.com/m/a TestA/sub:",
				"    a_test.go:3: got 1; want 2",
				"  Ns.Tests.EnumeratesTree(x: 1):",
				"    System.UnauthorizedAccessException : denied",
				"tier1 (exit 1), log /runs/b.log:",
				"  its output:",
				"    some line",
				"guard tier1 (exit 1), log /runs/g.log:",
				"  example.com/m/g TestGuard:",
				"... 3 more lines cut (guard tier1: 3); the logs named above hold the cut lines of a run with a log",
			),
			want:   []string{"tier0 example.com/m/a TestA/sub", "tier0 Ns.Tests.EnumeratesTree(x: 1)", "tier1 (exit 1)", "guard tier1 example.com/m/g TestGuard"},
			wantOK: true,
		},
		{
			name:   "a run with a cause line and no failure is listed by its outcome",
			log:    report("tier1 (timed out), log /runs/a.log:", "  lym gate test: tier timeout: tier1 exceeded timeout_min 30m0s; find the hang"),
			want:   []string{"tier1 (timed out)"},
			wantOK: true,
		},
		{
			name: "a package that failed outside any test is no test run",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "  example.com/m/b, failed outside any test:", "    b.go:3: undefined: x"),
		},
		{
			name: "a report that lists no run names none",
			log:  report(),
		},
		{
			name: "a build step beside a tier run is no test run",
			log:  report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "build (exit 2), log /runs/b.log:", "  its output:"),
		},
		{
			name: "a vet run is no test run",
			log:  report("vet tier1 (exit 1), log /runs/v.log:", "  its output:", "    p.go:3:2: unreachable code"),
		},
		{
			name: "a lint step is no test run",
			log:  report("lint comments (1 finding); way forward: fix it:", "  a.go:3: // wrapped at the"),
		},
		{
			name: "a log with no report names none",
			log:  "--- FAIL: TestA (0.00s)\nFAIL\texample.com/m/a\t0.1s\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := TierRunFailures(tt.log)
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TierRunFailures() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestFailedTestsCall pins the command that runs failed tests alone: one call per tier in tier order, quoted per shell.
func TestFailedTestsCall(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		failed  []FailedTest
		want    string
		wantErr bool
	}{
		{
			name: "one call per tier in tier order, each package and name once",
			goos: "linux",
			failed: []FailedTest{
				{Tier: 1, Package: "example.com/m/b", Test: "TestB"},
				{Tier: 0, Package: "example.com/m/c", Test: "TestA"},
				{Tier: 0, Package: "example.com/m/a", Test: "TestC"},
				{Tier: 0, Package: "example.com/m/a", Test: "TestA"},
			},
			want: "lym gate test --wait --stop-on-fail --tier 0 'example.com/m/a' 'example.com/m/c' -- -run '^(TestA|TestC)$'" +
				" && lym gate test --wait --stop-on-fail --tier 1 'example.com/m/b' -- -run '^(TestB)$'",
		},
		{
			name:   "cmd quoting on windows",
			goos:   "windows",
			failed: []FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}},
			want:   `lym gate test --wait --stop-on-fail --tier 0 "example.com/m/a" -- -run "^(TestA)$"`,
		},
		{
			name:    "no failed test is refused",
			goos:    "linux",
			wantErr: true,
		},
		{
			name:    "a package cmd cannot quote is refused",
			goos:    "windows",
			failed:  []FailedTest{{Tier: 0, Package: "example.com/m/100%", Test: "TestA"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := FailedTestsCall(tt.goos, tt.failed)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("FailedTestsCall(%q, %+v) = %q, %v; want %q, error %v", tt.goos, tt.failed, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
