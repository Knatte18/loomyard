//go:build tier0

// counters_test.go covers the counters a tier names: the text they pass through and the test counts they report.

package gatecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// buildEvent renders one `go test -json` event line of a package's build, which names its package by ImportPath, without its newline.
func buildEvent(action, output string) string {
	line := fmt.Sprintf(`{"Action":%q,"ImportPath":"example.com/p [example.com/p.test]"`, action)
	if output != "" {
		line += fmt.Sprintf(`,"Output":%q`, output)
	}
	return line + "}"
}

// goEvent renders one `go test -json` event line, without its newline.
func goEvent(action, test, output string) string {
	line := fmt.Sprintf(`{"Action":%q,"Package":"example.com/p"`, action)
	if test != "" {
		line += fmt.Sprintf(`,"Test":%q`, test)
	}
	if output != "" {
		line += fmt.Sprintf(`,"Output":%q`, output)
	}
	return line + "}"
}

// goPackageEvent renders one package-level `go test -json` event line for pkg, stamped at the given offset in seconds past a fixed start, without its newline.
func goPackageEvent(pkg, action, output string, offset, elapsed float64) string {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	event := map[string]any{"Action": action, "Package": pkg, "Time": base.Add(time.Duration(offset * float64(time.Second))).Format(time.RFC3339Nano)}
	if output != "" {
		event["Output"] = output
	}
	if elapsed != 0 {
		event["Elapsed"] = elapsed
	}
	line, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return string(line)
}

func TestCounters(t *testing.T) {
	t.Parallel()

	splitEvent := goEvent("output", "TestA", "=== RUN   TestA\n") + "\n"
	vstestPassed := "Passed!  - Failed:     0, Passed:    12, Skipped:     0, Total:    12, Duration: 1 s - x.dll (net8.0)\n"
	vstestFailed := "Failed!  - Failed:     2, Passed:     3, Skipped:     1, Total:     6, Duration: 2 s - y.dll (net8.0)\n"

	tests := []struct {
		name string
		kind string
		// writes are the chunks written in order.
		writes []string
		// held is the suffix of wantOut that only Flush emits.
		held       string
		wantOut    string
		wantCounts tierCounts
		// wantLaunches is the launch list the counter reports.
		wantLaunches []gateslot.PackageLaunch
		// wantTestsRun is the per-package count of tests that ran.
		wantTestsRun map[string]int
		// wantFailures is the failed leaf tests and unexplained failed packages the counter reports.
		wantFailures []runFailure
		// wantUnparsed is the first lines that were no JSON event.
		wantUnparsed []string
	}{
		{
			name: "a package with start, a later event and a closing pass has one launch entry",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goPackageEvent("example.com/p", "start", "", 0, 0), goPackageEvent("example.com/p", "output", "ok  \texample.com/p\t1.5s\n", 0.25, 0), goPackageEvent("example.com/p", "pass", "", 1.5, 1.5),
				goPackageEvent("example.com/q", "start", "", 1, 0), goPackageEvent("example.com/q", "output", "FAIL\texample.com/q\t0.5s\n", 1.75, 0), goPackageEvent("example.com/q", "fail", "", 2, 0.5),
			}, "\n") + "\n"},
			wantOut:      "ok  \texample.com/p\t1.5s\nFAIL\texample.com/q\t0.5s\n",
			wantLaunches: []gateslot.PackageLaunch{{Package: "example.com/p", LaunchSeconds: 0.25, ElapsedSeconds: 1.5}, {Package: "example.com/q", LaunchSeconds: 0.75, ElapsedSeconds: 0.5}},
			wantTestsRun: map[string]int{"example.com/p": 0, "example.com/q": 0},
			wantFailures: []runFailure{{pkg: "example.com/q"}},
		},
		{
			name: "a cached package and a package with no test files have no launch entry",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goPackageEvent("example.com/c", "start", "", 0, 0), goPackageEvent("example.com/c", "output", "ok  \texample.com/c\t(cached)\n", 0.01, 0), goPackageEvent("example.com/c", "pass", "", 0.02, 0),
				goPackageEvent("example.com/n", "start", "", 0, 0), goPackageEvent("example.com/n", "output", "?   \texample.com/n\t[no test files]\n", 0.01, 0), goPackageEvent("example.com/n", "skip", "", 0.02, 0),
			}, "\n") + "\n"},
			wantOut:      "ok  \texample.com/c\t(cached)\n?   \texample.com/n\t[no test files]\n",
			wantTestsRun: map[string]int{"example.com/c": 0, "example.com/n": 0},
		},
		{
			name: "a passing and a failing test",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("output", "TestA", "=== RUN   TestA\n"), goEvent("pass", "TestA", ""),
				goEvent("run", "TestB", ""), goEvent("output", "TestB", "--- FAIL: TestB (0.00s)\n"), goEvent("fail", "TestB", ""),
				goEvent("output", "", "FAIL\n"), goEvent("fail", "", ""),
			}, "\n") + "\n"},
			wantOut:      "=== RUN   TestA\n--- FAIL: TestB (0.00s)\nFAIL\n",
			wantCounts:   tierCounts{discovered: 2, passed: 1, failed: 1},
			wantTestsRun: map[string]int{"example.com/p": 2},
			wantFailures: []runFailure{{pkg: "example.com/p", test: "TestB"}},
		},
		{
			name:         "a skipped test",
			kind:         "go-json",
			writes:       []string{goEvent("run", "TestA", "") + "\n" + goEvent("skip", "TestA", "") + "\n"},
			wantCounts:   tierCounts{discovered: 1, skipped: 1},
			wantTestsRun: map[string]int{"example.com/p": 1},
		},
		{
			name: "subtests count each",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("run", "TestA/sub", ""), goEvent("pass", "TestA/sub", ""), goEvent("pass", "TestA", ""),
			}, "\n") + "\n"},
			wantCounts:   tierCounts{discovered: 2, passed: 2},
			wantTestsRun: map[string]int{"example.com/p": 2},
		},
		{
			name: "a failed subtest is named with its error lines and its failed parent is left out",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("output", "TestA", "=== RUN   TestA\n"),
				goEvent("run", "TestA/sub", ""), goEvent("output", "TestA/sub", "=== RUN   TestA/sub\n"),
				goEvent("output", "TestA/sub", "    a_test.go:12: got 1; want 2\n"), goEvent("output", "TestA/sub", "    a_test.go:13: second\n"),
				goEvent("output", "TestA/sub", "    --- FAIL: TestA/sub (0.00s)\n"), goEvent("fail", "TestA/sub", ""),
				goEvent("output", "TestA", "--- FAIL: TestA (0.00s)\n"), goEvent("fail", "TestA", ""),
				goEvent("run", "TestB", ""), goEvent("output", "TestB", "    b_test.go:5: passing note\n"), goEvent("pass", "TestB", ""),
			}, "\n") + "\n"},
			wantOut: "=== RUN   TestA\n=== RUN   TestA/sub\n    a_test.go:12: got 1; want 2\n    a_test.go:13: second\n    --- FAIL: TestA/sub (0.00s)\n--- FAIL: TestA (0.00s)\n" +
				"    b_test.go:5: passing note\n",
			wantCounts:   tierCounts{discovered: 3, passed: 1, failed: 2},
			wantTestsRun: map[string]int{"example.com/p": 3},
			wantFailures: []runFailure{{pkg: "example.com/p", test: "TestA/sub", lines: []string{"a_test.go:12: got 1; want 2", "a_test.go:13: second"}}},
		},
		{
			name: "a failed test keeps its first failureLines lines",
			kind: "go-json",
			writes: []string{strings.Join(append(
				[]string{goEvent("run", "TestA", "")},
				goEvent("output", "TestA", "l1\n"), goEvent("output", "TestA", "l2\n"), goEvent("output", "TestA", "l3\n"), goEvent("output", "TestA", "l4\n"), goEvent("output", "TestA", "l5\n"),
				goEvent("output", "TestA", "l6\n"), goEvent("output", "TestA", "l7\n"), goEvent("output", "TestA", "l8\n"), goEvent("output", "TestA", "l9\n"), goEvent("fail", "TestA", ""),
			), "\n") + "\n"},
			wantOut:      "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n",
			wantCounts:   tierCounts{discovered: 1, failed: 1},
			wantTestsRun: map[string]int{"example.com/p": 1},
			wantFailures: []runFailure{{pkg: "example.com/p", test: "TestA", lines: []string{"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8"}}},
		},
		{
			name: "a package whose build failed is a failure with its build output",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				buildEvent("build-output", "# example.com/p\n"), buildEvent("build-output", "x.go:3:2: undefined: y\n"), buildEvent("build-fail", ""),
				goEvent("start", "", ""), goEvent("output", "", "FAIL\texample.com/p [build failed]\n"),
				`{"Action":"fail","Package":"example.com/p","FailedBuild":"example.com/p [example.com/p.test]"}`,
			}, "\n") + "\n"},
			wantOut:      "# example.com/p\nx.go:3:2: undefined: y\nFAIL\texample.com/p [build failed]\n",
			wantTestsRun: map[string]int{"example.com/p": 0},
			wantFailures: []runFailure{{pkg: "example.com/p", lines: []string{"x.go:3:2: undefined: y"}}},
		},
		{
			name: "a package that panics outside any test is a failure with its own lines",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("start", "", ""), goEvent("output", "", "panic: init failed\n"), goEvent("output", "", "goroutine 1 [running]:\n"),
				goEvent("output", "", "FAIL\texample.com/p\t0.01s\n"), goEvent("fail", "", ""),
			}, "\n") + "\n"},
			wantOut:      "panic: init failed\ngoroutine 1 [running]:\nFAIL\texample.com/p\t0.01s\n",
			wantTestsRun: map[string]int{"example.com/p": 0},
			wantFailures: []runFailure{{pkg: "example.com/p", lines: []string{"panic: init failed", "goroutine 1 [running]:"}}},
		},
		{
			name: "a failed test whose name prefixes another failed test's name without a slash is still named",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("fail", "TestA", ""),
				goEvent("run", "TestAB", ""), goEvent("fail", "TestAB", ""),
			}, "\n") + "\n"},
			wantCounts:   tierCounts{discovered: 2, failed: 2},
			wantTestsRun: map[string]int{"example.com/p": 2},
			wantFailures: []runFailure{{pkg: "example.com/p", test: "TestA"}, {pkg: "example.com/p", test: "TestAB"}},
		},
		{
			name: "a panic's output and a build error's non-JSON lines read as text, the non-JSON ones kept as unparsed bar status lines",
			kind: "go-json",
			writes: []string{
				"# example.com/p\nx.go:1:1: undefined: y\n" + goEvent("output", "TestA", "panic: boom\n") + "\n" + goEvent("fail", "TestA", "") + "\nFAIL\texample.com/p [build failed]\n",
			},
			wantOut:      "# example.com/p\nx.go:1:1: undefined: y\npanic: boom\nFAIL\texample.com/p [build failed]\n",
			wantCounts:   tierCounts{failed: 1},
			wantTestsRun: map[string]int{"example.com/p": 0},
			wantFailures: []runFailure{{pkg: "example.com/p", test: "TestA", lines: []string{"panic: boom"}}},
			wantUnparsed: []string{"# example.com/p", "x.go:1:1: undefined: y"},
		},
		{
			name:         "a line split across two writes",
			kind:         "go-json",
			writes:       []string{splitEvent[:20], splitEvent[20:]},
			wantOut:      "=== RUN   TestA\n",
			wantCounts:   tierCounts{},
			wantTestsRun: map[string]int{"example.com/p": 0},
		},
		{
			name:         "a trailing line with no newline comes out on Flush",
			kind:         "go-json",
			writes:       []string{"ok\n", "tail"},
			held:         "tail",
			wantOut:      "ok\ntail",
			wantCounts:   tierCounts{},
			wantUnparsed: []string{"ok", "tail"},
		},
		{
			name: "package-level events alone, as an untagged TestMain gives, discover zero and count the package at zero",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("start", "", ""), goEvent("output", "", "ok  \texample.com/p\t0.01s [no tests to run]\n"), goEvent("pass", "", ""),
			}, "\n") + "\n"},
			wantOut:      "ok  \texample.com/p\t0.01s [no tests to run]\n",
			wantCounts:   tierCounts{},
			wantTestsRun: map[string]int{"example.com/p": 0},
		},
		{
			name:       "two VSTest summary lines are summed and passed through",
			kind:       "vstest",
			writes:     []string{"Starting test run\n" + vstestPassed + vstestPassed},
			wantOut:    "Starting test run\n" + vstestPassed + vstestPassed,
			wantCounts: tierCounts{discovered: 24, passed: 24},
		},
		{
			name:       "a VSTest Failed! summary",
			kind:       "vstest",
			writes:     []string{vstestFailed},
			wantOut:    vstestFailed,
			wantCounts: tierCounts{discovered: 6, passed: 3, failed: 2, skipped: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			c, err := newCounter(tt.kind, &out)
			if err != nil {
				t.Fatalf("newCounter(%q) error = %v; want nil", tt.kind, err)
			}
			for _, chunk := range tt.writes {
				if n, err := c.Write([]byte(chunk)); n != len(chunk) || err != nil {
					t.Fatalf("Write(%q) = %d, %v; want %d, nil", chunk, n, err, len(chunk))
				}
			}
			if want := strings.TrimSuffix(tt.wantOut, tt.held); out.String() != want {
				t.Errorf("output before Flush = %q; want %q", out.String(), want)
			}
			if err := c.Flush(); err != nil {
				t.Fatalf("Flush() error = %v; want nil", err)
			}
			if out.String() != tt.wantOut {
				t.Errorf("output = %q; want %q", out.String(), tt.wantOut)
			}
			if got := c.Counts(); got != tt.wantCounts {
				t.Errorf("Counts() = %+v; want %+v", got, tt.wantCounts)
			}
			if got := c.Launches(); !slices.Equal(got, tt.wantLaunches) {
				t.Errorf("Launches() = %+v; want %+v", got, tt.wantLaunches)
			}
			if got := c.TestsRun(); !maps.Equal(got, tt.wantTestsRun) {
				t.Errorf("TestsRun() = %v; want %v", got, tt.wantTestsRun)
			}
			if diff := cmp.Diff(tt.wantFailures, c.Failures(), cmp.AllowUnexported(runFailure{})); diff != "" {
				t.Errorf("Failures() mismatch (-want +got):\n%s", diff)
			}
			if got := c.Unparsed(); !slices.Equal(got, tt.wantUnparsed) {
				t.Errorf("Unparsed() = %q; want %q", got, tt.wantUnparsed)
			}
		})
	}

	t.Run("an unknown kind is an error", func(t *testing.T) {
		t.Parallel()

		if _, err := newCounter("tap", &bytes.Buffer{}); err == nil {
			t.Error("newCounter(\"tap\") error = nil; want an error")
		}
	})
}
