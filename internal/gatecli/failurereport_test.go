//go:build tier0

// failurereport_test.go covers the failure report's rendering, the failed runs it is built from and the bound on a capped call's replayed stdout.

package gatecli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

func TestRenderFailureReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runs    []failedRun
		touched []string
		want    []string
	}{
		{name: "no failed run renders nothing"},
		{
			name: "a tier's failed test, with its bare file named by its package's directory, and a package failure, then the touched files",
			runs: []failedRun{{
				label: "tier0", outcome: "exit 1", log: "/logs/tier0.log", modulePath: "example.com/m", moduleDir: "sub",
				failures: []runFailure{
					{pkg: "example.com/m/p", test: "TestA/sub", lines: []string{"a_test.go:12: got 1; want 2", "dir/x.go:3: kept as printed"}},
					{pkg: "example.com/m/q", lines: []string{"q.go:3:2: undefined: y"}},
					{pkg: "example.com/other", test: "TestB", lines: []string{"b_test.go:4: outside the module"}},
				},
			}},
			touched: []string{"sub/p/a.go", "sub/p/a_test.go"},
			want: []string{
				failureReportHeader,
				"tier0 (exit 1), log /logs/tier0.log:",
				"  example.com/m/p TestA/sub:",
				"    sub/p/a_test.go:12: got 1; want 2",
				"    dir/x.go:3: kept as printed",
				"  example.com/m/q, failed outside any test:",
				"    sub/q/q.go:3:2: undefined: y",
				"  example.com/other TestB:",
				"    b_test.go:4: outside the module",
				"gate test: the change touches sub/p/a.go, sub/p/a_test.go",
			},
		},
		{
			name: "a build error of a nested module is named by its worktree path only in a package that failed outside any test",
			runs: []failedRun{{
				label: "tier0", outcome: "exit 1", log: "/logs/tier0.log", modulePath: "example.com/m", moduleDir: "sub",
				failures: []runFailure{
					{pkg: "example.com/m/q", lines: []string{"q/q.go:3:2: undefined: y", "./root.go:4:1: syntax error", `.\win.go:6:1: undefined: w`, "/abs/z.go:5: kept as printed"}},
					{pkg: "example.com/m/p", test: "TestA", lines: []string{"x/x.go:7: kept as printed"}},
				},
			}, {
				label: "tier1", outcome: "exit 1", log: "/logs/tier1.log", modulePath: "example.com/top", moduleDir: ".",
				failures: []runFailure{{pkg: "example.com/top/q", lines: []string{"q/q.go:3:2: undefined: y"}}},
			}},
			want: []string{
				failureReportHeader,
				"tier0 (exit 1), log /logs/tier0.log:",
				"  example.com/m/q, failed outside any test:",
				"    sub/q/q.go:3:2: undefined: y",
				"    sub/root.go:4:1: syntax error",
				"    sub/win.go:6:1: undefined: w",
				"    /abs/z.go:5: kept as printed",
				"  example.com/m/p TestA:",
				"    x/x.go:7: kept as printed",
				"tier1 (exit 1), log /logs/tier1.log:",
				"  example.com/top/q, failed outside any test:",
				"    q/q.go:3:2: undefined: y",
			},
		},
		{
			name: "a run's cause lines are listed beside its failures, and alone when nothing else was read",
			runs: []failedRun{
				{
					label: "tier0", outcome: "timed out", log: "/logs/tier0.log", causes: []string{"tier0 timeout"},
					failures: []runFailure{{pkg: "example.com/m/p", test: "TestA", lines: []string{"a_test.go:1: boom"}}},
				},
				{label: "tier1", outcome: "exit 1", log: "/logs/tier1.log", causes: []string{"no tests ran"}},
			},
			want: []string{
				failureReportHeader,
				"tier0 (timed out), log /logs/tier0.log:",
				"  example.com/m/p TestA:",
				"    a_test.go:1: boom",
				"  tier0 timeout",
				"tier1 (exit 1), log /logs/tier1.log:",
				"  no tests ran",
			},
		},
		{
			name: "a run with no failure read lists its unparsed lines, or says its log holds the cause, and a lint step names its way forward",
			runs: []failedRun{
				{label: "vet tier1", outcome: "exit 1", log: "/logs/vet.log", unparsed: []string{"# example.com/m/p", "p.go:3:2: unreachable code"}},
				{label: "tier2", outcome: "timed out", log: "/logs/tier2.log"},
				{label: "lint comments", outcome: "1 finding", wayForward: "fix it", failures: []runFailure{{lines: []string{"a.go:3: // wrapped at the"}}}},
			},
			want: []string{
				failureReportHeader,
				"vet tier1 (exit 1), log /logs/vet.log:",
				"  its output:",
				"    # example.com/m/p",
				"    p.go:3:2: unreachable code",
				"tier2 (timed out), log /logs/tier2.log:",
				"  no failing test or error line was read from its output; its log holds the cause",
				"lint comments (1 finding); way forward: fix it:",
				"  a.go:3: // wrapped at the",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, renderFailureReport(tt.runs, tt.touched)); diff != "" {
				t.Errorf("renderFailureReport() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("a long report stops at its line cap, a long line is cut and the touched files are capped", func(t *testing.T) {
		t.Parallel()
		failures := make([]runFailure, failureReportLines)
		for i := range failures {
			failures[i] = runFailure{pkg: "example.com/p", test: fmt.Sprintf("Test%d", i), lines: []string{strings.Repeat("x", failureReportLineBytes+10)}}
		}
		touched := make([]string, touchedFilesListed+3)
		for i := range touched {
			touched[i] = fmt.Sprintf("f%d.go", i)
		}
		got := renderFailureReport([]failedRun{{label: "tier0", outcome: "exit 1", log: "/l", failures: failures}}, touched)

		// The header, the capped body, the more line and the touched line.
		if len(got) != failureReportLines+3 {
			t.Fatalf("renderFailureReport() = %d lines; want %d", len(got), failureReportLines+3)
		}
		wantMore := fmt.Sprintf("... %d more lines cut (tier0: %d); the logs named above hold the cut lines of a run with a log", failureReportLines+1, failureReportLines+1)
		if more := got[failureReportLines+1]; more != wantMore {
			t.Errorf("more line = %q; want %q", more, wantMore)
		}
		if cut := got[3]; cut != "    "+strings.Repeat("x", failureReportLineBytes-4)+"..." {
			t.Errorf("long line = %q; want it cut at %d bytes and marked", cut, failureReportLineBytes)
		}
		if last := got[len(got)-1]; !strings.HasSuffix(last, fmt.Sprintf("f%d.go and 3 more", touchedFilesListed-1)) {
			t.Errorf("touched line = %q; want the first %d files and the count of the rest", last, touchedFilesListed)
		}
	})

	t.Run("a lint step a tier flood cuts is named in the cut line with the lint to re-run, since it has no log", func(t *testing.T) {
		t.Parallel()
		failures := make([]runFailure, failureReportLines)
		for i := range failures {
			failures[i] = runFailure{pkg: "example.com/p", test: fmt.Sprintf("Test%d", i), lines: []string{"x"}}
		}
		lint := failedRun{label: "lint comments", outcome: "2 findings", failures: []runFailure{{lines: []string{"a.go:3: // x"}}, {lines: []string{"b.go:4: // y"}}}}
		pattern := failedRun{label: "lint pattern", outcome: "1 finding", failures: []runFailure{{lines: []string{"PATTERN.md: grew"}}}}
		vet := failedRun{label: "vet tier1", outcome: "exit 1", log: "/v", unparsed: []string{"a", "b"}}
		guard := failedRun{label: "guard tier1", outcome: "exit 1", log: "/g", unparsed: []string{"a", "b"}}
		got := renderFailureReport([]failedRun{{label: "tier0", outcome: "exit 1", log: "/l", failures: failures}, vet, guard, lint, pattern}, nil)

		// The header and the capped body, which the tier fills, then the cut line.
		if len(got) != failureReportLines+2 {
			t.Fatalf("renderFailureReport() = %d lines; want %d", len(got), failureReportLines+2)
		}
		// The tier's header and 80 failures are 1+2*80 lines, 1+2*80-80 of them cut, the vet and guard runs' 4 lines each, the comments lint's 3 lines and the pattern lint's 2 all cut.
		want := fmt.Sprintf("... %d more lines cut (tier0: %d, vet tier1: 4, guard tier1: 4, lint comments: 3, lint pattern: 2); the logs named above hold the cut lines of a run with a log; lint comments has no log, so re-run `lym loom lint-comments` for its findings; lint pattern has no log, so re-run `lym loom lint-pattern` for its findings", failureReportLines+1+4+4+3+2, failureReportLines+1)
		if cut := got[len(got)-1]; cut != want {
			t.Errorf("cut line = %q; want %q", cut, want)
		}
		if len(want) <= failureReportLineBytes {
			t.Errorf("cut line is %d bytes; want a case longer than the %d-byte line cap, which the cut line is exempt from", len(want), failureReportLineBytes)
		}
	})
}

func TestLintFailedRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lint     string
		findings []string
		err      error
		want     failedRun
		wantOK   bool
	}{
		{name: "no finding is no failed run", lint: gateslot.LintComments},
		{
			name: "findings are counted and carry the lint's way forward", lint: gateslot.LintComments, findings: []string{"a.go:3: // x", "b.go:4: // y"}, wantOK: true,
			want: failedRun{label: "lint comments", outcome: "2 findings", wayForward: lintWayForward[gateslot.LintComments], failures: []runFailure{{lines: []string{"a.go:3: // x"}}, {lines: []string{"b.go:4: // y"}}}},
		},
		{
			name: "one finding reads in the singular", lint: gateslot.LintPattern, findings: []string{"PATTERN.md: grew"}, wantOK: true,
			want: failedRun{label: "lint pattern", outcome: "1 finding", wayForward: lintWayForward[gateslot.LintPattern], failures: []runFailure{{lines: []string{"PATTERN.md: grew"}}}},
		},
		{
			name: "a lint that cannot run is a failed run naming the error", lint: gateslot.LintPattern, err: errors.New("git broke"), wantOK: true,
			want: failedRun{label: "lint pattern", outcome: "could not run", wayForward: "re-run the same command", unparsed: []string{"git broke"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lintFailedRun(tt.lint, tt.findings, tt.err)
			if ok != tt.wantOK {
				t.Fatalf("lintFailedRun() ok = %v; want %v", ok, tt.wantOK)
			}
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(failedRun{}, runFailure{})); diff != "" {
				t.Errorf("lintFailedRun() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildFailedRun_KeepsTheLastLinesOfItsOutput(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	for i := range unparsedLines + 2 {
		fmt.Fprintf(&output, "line %d\n\n", i)
	}
	got := buildFailedRun(2, false, "/logs/build.log", []byte(output.String()))
	if got.label != "build" || got.outcome != "exit 2" || got.log != "/logs/build.log" {
		t.Errorf("buildFailedRun() = %+v; want the build labelled with its exit code and log", got)
	}
	if len(got.unparsed) != unparsedLines || got.unparsed[0] != "line 2" || got.unparsed[unparsedLines-1] != fmt.Sprintf("line %d", unparsedLines+1) {
		t.Errorf("buildFailedRun() unparsed = %q; want the last %d non-empty lines", got.unparsed, unparsedLines)
	}
	if timedOut := buildFailedRun(1, true, "/l", nil); timedOut.outcome != "timed out" {
		t.Errorf("buildFailedRun(timed out) outcome = %q; want timed out", timedOut.outcome)
	}
}

func TestTierFailedRun_NamesTheModuleOfItsRunDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module := filepath.Join(root, "sub")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/m\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := tierResult{
		record:   gateslot.RunRecord{ExitCode: 1, RunDir: module},
		failures: []runFailure{{pkg: "example.com/m/p", test: "TestA"}},
		causes:   []string{"cause"},
		unparsed: []string{"output"},
	}
	got := tierFailedRun(tierRun{number: 1, vetStep: true}, result, "/logs/vet.log", root)
	want := failedRun{label: "vet tier1", outcome: "exit 1", log: "/logs/vet.log", modulePath: "example.com/m", moduleDir: "sub", failures: result.failures, causes: result.causes, unparsed: result.unparsed}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(failedRun{}, runFailure{})); diff != "" {
		t.Errorf("tierFailedRun() mismatch (-want +got):\n%s", diff)
	}
}

func TestReplayStdout(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("=== RUN   TestA\n", replayStdoutBytes/8)
	tests := []struct {
		name string
		// content is the stdout file's content; nil leaves no file.
		content    []byte
		wantOut    string
		wantStderr []string
	}{
		{name: "an absent file prints nothing"},
		{name: "a short stdout is printed whole", content: []byte("ok\n"), wantOut: "ok\n"},
		{name: "a long stdout is named on stderr, not printed", content: []byte(long), wantStderr: []string{"bytes, is too long to print and stays in "}},
		{name: "a long stdout's closing JSON envelope is printed", content: []byte(long + `{"ok":false,"error":"refused"}` + "\n"), wantOut: `{"ok":false,"error":"refused"}` + "\n", wantStderr: []string{"is too long to print"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "stdout")
			if tt.content != nil {
				if err := os.WriteFile(path, tt.content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var out, stderr bytes.Buffer
			if err := replayStdout(&out, &stderr, "test", path); err != nil {
				t.Fatalf("replayStdout() error = %v; want nil", err)
			}
			if out.String() != tt.wantOut {
				t.Errorf("replayStdout() out = %q; want %q", out.String(), tt.wantOut)
			}
			if len(tt.wantStderr) == 0 && stderr.Len() > 0 {
				t.Errorf("replayStdout() stderr = %q; want none", stderr.String())
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) || !strings.Contains(stderr.String(), path) {
					t.Errorf("replayStdout() stderr = %q; want it to hold %q and the file's path", stderr.String(), want)
				}
			}
		})
	}
}
