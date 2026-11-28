// failedtests_test.go covers the read of a failure report's failed tests and the call that runs them alone.

package gateslot

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

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
