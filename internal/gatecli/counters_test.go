// counters_test.go covers the counters a tier names: the text they pass through and the test counts they report.

package gatecli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

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
	}{
		{
			name: "a passing and a failing test",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("output", "TestA", "=== RUN   TestA\n"), goEvent("pass", "TestA", ""),
				goEvent("run", "TestB", ""), goEvent("output", "TestB", "--- FAIL: TestB (0.00s)\n"), goEvent("fail", "TestB", ""),
				goEvent("output", "", "FAIL\n"), goEvent("fail", "", ""),
			}, "\n") + "\n"},
			wantOut:    "=== RUN   TestA\n--- FAIL: TestB (0.00s)\nFAIL\n",
			wantCounts: tierCounts{discovered: 2, passed: 1, failed: 1},
		},
		{
			name:       "a skipped test",
			kind:       "go-json",
			writes:     []string{goEvent("run", "TestA", "") + "\n" + goEvent("skip", "TestA", "") + "\n"},
			wantCounts: tierCounts{discovered: 1, skipped: 1},
		},
		{
			name: "subtests count each",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("run", "TestA", ""), goEvent("run", "TestA/sub", ""), goEvent("pass", "TestA/sub", ""), goEvent("pass", "TestA", ""),
			}, "\n") + "\n"},
			wantCounts: tierCounts{discovered: 2, passed: 2},
		},
		{
			name: "a panic's output and a build error's non-JSON lines read as text",
			kind: "go-json",
			writes: []string{
				"# example.com/p\nx.go:1:1: undefined: y\n" + goEvent("output", "TestA", "panic: boom\n") + "\n" + goEvent("fail", "TestA", "") + "\nFAIL\texample.com/p [build failed]\n",
			},
			wantOut:    "# example.com/p\nx.go:1:1: undefined: y\npanic: boom\nFAIL\texample.com/p [build failed]\n",
			wantCounts: tierCounts{failed: 1},
		},
		{
			name:       "a line split across two writes",
			kind:       "go-json",
			writes:     []string{splitEvent[:20], splitEvent[20:]},
			wantOut:    "=== RUN   TestA\n",
			wantCounts: tierCounts{},
		},
		{
			name:       "a trailing line with no newline comes out on Flush",
			kind:       "go-json",
			writes:     []string{"ok\n", "tail"},
			held:       "tail",
			wantOut:    "ok\ntail",
			wantCounts: tierCounts{},
		},
		{
			name: "package-level events alone discover zero",
			kind: "go-json",
			writes: []string{strings.Join([]string{
				goEvent("start", "", ""), goEvent("output", "", "ok  \texample.com/p\t0.01s [no tests to run]\n"), goEvent("pass", "", ""),
			}, "\n") + "\n"},
			wantOut:    "ok  \texample.com/p\t0.01s [no tests to run]\n",
			wantCounts: tierCounts{},
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
		})
	}

	t.Run("an unknown kind is an error", func(t *testing.T) {
		t.Parallel()

		if _, err := newCounter("tap", &bytes.Buffer{}); err == nil {
			t.Error("newCounter(\"tap\") error = nil; want an error")
		}
	})
}
