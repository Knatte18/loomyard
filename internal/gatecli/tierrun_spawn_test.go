//go:build !windows

// tierrun_spawn_test.go covers the tier runs that start a POSIX shell and a tool such as cp.
// They cover a vstest tier's verdict on the results file the gate asks for, and the env line of a tier run.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// linesOf returns the lines of the file at path, or none when the file does not exist.
func linesOf(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// TestRunTierReadsTheVSTestResultsFile pins a vstest tier's verdict on the TRX file the gate asks for.
// The command records its args and the results directory it finds among them, and copies a fixture TRX file there.
// It then prints a console summary, which a row sets to disagree with the file, and exits.
func TestRunTierReadsTheVSTestResultsFile(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	trx := func(results, runInfos string) string {
		return `<?xml version="1.0" encoding="utf-8"?>
<TestRun xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>` + results + `</Results>
  <ResultSummary outcome="Completed"><RunInfos>` + runInfos + `</RunInfos></ResultSummary>
</TestRun>
`
	}
	const passed = `<UnitTestResult testName="Ns.Tests.Kept" outcome="Passed" />`
	const failed = `<UnitTestResult testName="Ns.Tests.Broken" outcome="Failed"><Output><ErrorInfo>
<Message>Assert.That failed
  Expected: 1</Message><StackTrace>   at Ns.Tests.Broken() in Tests.cs:line 9</StackTrace></ErrorInfo></Output></UnitTestResult>`
	failedOther := strings.ReplaceAll(failed, "Broken", "Other")
	const otherOutcomes = `<UnitTestResult testName="Ns.Tests.Errored" outcome="Error" />
<UnitTestResult testName="Ns.Tests.TimedOut" outcome="Timeout" />
<UnitTestResult testName="Ns.Tests.Aborted" outcome="Aborted" />
<UnitTestResult testName="Ns.Tests.NotRun" outcome="NotExecuted" />`
	const crash = `<RunInfo outcome="Error"><Text>The active test run was aborted. Reason: Test host process crashed : Fatal error.
The test running when the crash occurred: Ns.Tests.Kept</Text></RunInfo>`
	const warning = `<RunInfo outcome="Warning"><Text>No test matches the given testcase filter</Text></RunInfo>`
	crashLine := "gate test: the test results file of tier0 reports: The active test run was aborted. Reason: Test host process crashed : Fatal error.; The test running when the crash occurred: Ns.Tests.Kept"
	const (
		detailedConsole = `echo 'Total tests: 1'; echo '     Passed: 1'`
		summaryOfNine   = `echo 'Passed!  - Failed:     0, Passed:     9, Skipped:     0, Total:     9, Duration: 1 s - a.dll (net8.0)'`
		summaryOfFour   = `echo 'Passed!  - Failed:     0, Passed:     3, Skipped:     1, Total:     4, Duration: 1 s - a.dll (net8.0)'`
	)
	refusal := func(word string) string {
		return fmt.Sprintf("gate test: the words after \"--\" hold %s, which the gate sets itself because tier0 reads its results file; way forward: drop %s from the words after \"--\", then re-run the same command", word, word)
	}
	tests := []struct {
		name string
		// files is the results file each invocation's command copies, "" for an invocation whose command copies none.
		files []string
		// console is the shell text that prints the console summary; empty means detailedConsole.
		console string
		// args are the call's own words after `--`; nil means a detailed console logger.
		args         []string
		exit         int
		wantRecord   gateslot.RunRecord
		wantFailures []runFailure
		// wantLine is the line stderr, the log and the causes hold, or "" when stderr stays empty and no cause is named.
		wantLine string
		// wantRefusal is the refusal that ends the run before any command starts.
		wantRefusal string
	}{
		{
			name:       "ADetailedConsoleLoggerLeavesTheCountsToTheResultsFile",
			files:      []string{trx(passed, "")},
			wantRecord: gateslot.RunRecord{Discovered: 1, Passed: 1},
		},
		{
			name:         "AFailedTestIsReadFromTheResultsFile",
			files:        []string{trx(passed+failed, "")},
			exit:         1,
			wantRecord:   gateslot.RunRecord{ExitCode: 1, Discovered: 2, Passed: 1, Failed: 1},
			wantFailures: []runFailure{{test: "Ns.Tests.Broken", lines: []string{"Assert.That failed", "Expected: 1", "at Ns.Tests.Broken() in Tests.cs:line 9"}}},
		},
		{
			name:         "EveryOutcomeButPassedCountsAsFailedOrSkipped",
			files:        []string{trx(otherOutcomes, "")},
			exit:         1,
			wantRecord:   gateslot.RunRecord{ExitCode: 1, Discovered: 4, Failed: 3, Skipped: 1},
			wantFailures: []runFailure{{test: "Ns.Tests.Errored"}, {test: "Ns.Tests.TimedOut"}, {test: "Ns.Tests.Aborted"}},
		},
		{
			name:       "ACrashedTestHostNamesItsCauseAndNotAWarning",
			files:      []string{trx("", crash+warning)},
			exit:       1,
			wantRecord: gateslot.RunRecord{ExitCode: 1},
			wantLine:   crashLine,
		},
		{
			name:       "APassingRunKeepsItsRunMessagesOutOfTheCauses",
			files:      []string{trx(passed, crash)},
			wantRecord: gateslot.RunRecord{Discovered: 1, Passed: 1},
		},
		{
			name:       "EachInvocationReplacesOnlyItsOwnConsoleCounts",
			files:      []string{trx(passed+failed, ""), trx(passed+failedOther, "")},
			console:    summaryOfNine,
			wantRecord: gateslot.RunRecord{Discovered: 4, Passed: 2, Failed: 2},
			wantFailures: []runFailure{
				{test: "Ns.Tests.Broken", lines: []string{"Assert.That failed", "Expected: 1", "at Ns.Tests.Broken() in Tests.cs:line 9"}},
				{test: "Ns.Tests.Other", lines: []string{"Assert.That failed", "Expected: 1", "at Ns.Tests.Other() in Tests.cs:line 9"}},
			},
		},
		{
			name:       "AnInvocationThatWritesNoFileKeepsTheConsoleCounts",
			files:      []string{""},
			console:    summaryOfFour,
			wantRecord: gateslot.RunRecord{Discovered: 4, Passed: 3, Skipped: 1},
		},
		{
			name:       "AMalformedFileKeepsTheConsoleCounts",
			files:      []string{"<TestRun"},
			console:    summaryOfFour,
			wantRecord: gateslot.RunRecord{Discovered: 4, Passed: 3, Skipped: 1},
		},
		{
			name:        "ACallThatSetsTheResultsDirectoryIsRefused",
			files:       []string{""},
			args:        []string{"--results-directory", "/elsewhere"},
			wantRefusal: refusal("--results-directory"),
		},
		{
			name:        "ACallThatSetsATRXLoggerIsRefused",
			files:       []string{""},
			args:        []string{"--logger", "trx;LogFileName=mine.trx"},
			wantRefusal: refusal("--logger trx;LogFileName=mine.trx"),
		},
		{
			name:        "AJoinedTRXLoggerIsRefused",
			files:       []string{""},
			args:        []string{"--logger:trx"},
			wantRefusal: refusal("--logger:trx"),
		},
		{
			name:        "AJoinedResultsDirectoryIsRefused",
			files:       []string{""},
			args:        []string{"--results-directory=/elsewhere"},
			wantRefusal: refusal("--results-directory=/elsewhere"),
		},
		{
			name:        "AShortTRXLoggerIsRefused",
			files:       []string{""},
			args:        []string{"-l", "trx"},
			wantRefusal: refusal("-l trx"),
		},
		{
			name:       "ARuntimeOptionIsNotARefusedWord",
			files:      []string{trx(passed, "")},
			args:       []string{"-r", "linux-x64"},
			wantRecord: gateslot.RunRecord{Discovered: 1, Passed: 1},
		},
		{
			name:       "ALastConsoleLineWithNoNewlineIsReadBeforeTheFileReplacesTheCounts",
			files:      []string{trx(passed, "")},
			console:    `printf 'Passed!  - Failed:     0, Passed:     9, Skipped:     0, Total:     9, Duration: 1 s - a.dll (net8.0)'`,
			wantRecord: gateslot.RunRecord{Discovered: 1, Passed: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			argsLog, seenLog := filepath.Join(dir, "args"), filepath.Join(dir, "results-dirs")
			var invocations []invocation
			for i, content := range tt.files {
				one := invocation{dir: dir}
				if content != "" {
					fixture := filepath.Join(dir, fmt.Sprintf("fixture%d.trx", i))
					if err := os.WriteFile(fixture, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
					one.targets = []string{fixture}
				}
				invocations = append(invocations, one)
			}
			console, args := tt.console, tt.args
			if console == "" {
				console = detailedConsole
			}
			if args == nil {
				args = []string{"--logger", "console;verbosity=detailed"}
			}
			command := fmt.Sprintf(`printf '%%s\n' {args} >> %[1]q; set -- {args}; while [ $# -gt 0 ] && [ "$1" != --results-directory ]; do shift; done; printf '%%s\n' "$2" >> %[2]q; fixture={targets}; if [ -n "$fixture" ]; then cp "$fixture" "$2/gate_net8.0_1.trx"; fi; %[3]s; exit %[4]d`, argsLog, seenLog, console, tt.exit)
			var stderr bytes.Buffer
			request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay, args: args}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: command, Counter: counterVSTest, Results: gateslot.ResultsTRX, TimeoutMin: 1},
				invocations: invocations,
				args:        args,
			}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if tt.wantRefusal != "" {
				if result.refusal != tt.wantRefusal {
					t.Errorf("runTier() refusal = %q; want %q", result.refusal, tt.wantRefusal)
				}
				if ran := linesOf(t, seenLog); len(ran) != 0 {
					t.Errorf("runTier() ran the command, which saw the results directories %q; want a refusal before any command", ran)
				}
				return
			}
			if result.refusal != "" {
				t.Fatalf("runTier() refusal = %q; want none", result.refusal)
			}
			got := result.record
			want := tt.wantRecord
			if got.ExitCode != want.ExitCode || got.Discovered != want.Discovered || got.Passed != want.Passed || got.Failed != want.Failed || got.Skipped != want.Skipped {
				t.Errorf("runTier() record = exit %d, discovered %d, passed %d, failed %d, skipped %d; want exit %d, discovered %d, passed %d, failed %d, skipped %d",
					got.ExitCode, got.Discovered, got.Passed, got.Failed, got.Skipped, want.ExitCode, want.Discovered, want.Passed, want.Failed, want.Skipped)
			}
			if diff := cmp.Diff(tt.wantFailures, result.failures, cmp.AllowUnexported(runFailure{})); diff != "" {
				t.Errorf("runTier() failures mismatch (-want +got):\n%s", diff)
			}
			wantStderr := ""
			var wantCauses []string
			if tt.wantLine != "" {
				wantStderr, wantCauses = tt.wantLine+"\n", []string{tt.wantLine}
				if !strings.Contains(string(result.log), tt.wantLine+"\n") {
					t.Errorf("runTier() log = %q; want it to hold %q", result.log, tt.wantLine)
				}
			}
			if stderr.String() != wantStderr {
				t.Errorf("runTier() stderr = %q; want %q", stderr.String(), wantStderr)
			}
			if diff := cmp.Diff(wantCauses, result.causes); diff != "" {
				t.Errorf("runTier() causes mismatch (-want +got):\n%s", diff)
			}

			seen := linesOf(t, seenLog)
			if len(seen) != len(tt.files) {
				t.Fatalf("the command saw the results directories %q; want one per invocation, %d", seen, len(tt.files))
			}
			var wantArgs []string
			for _, resultsDir := range seen {
				wantArgs = append(wantArgs, "--results-directory", resultsDir, "--logger", "trx;LogFilePrefix=gate", "--blame")
				wantArgs = append(wantArgs, args...)
				if _, err := os.Stat(resultsDir); !os.IsNotExist(err) {
					t.Errorf("results directory %s after the run: stat error %v; want it removed", resultsDir, err)
				}
			}
			if diff := cmp.Diff(wantArgs, linesOf(t, argsLog)); diff != "" {
				t.Errorf("the command's args mismatch (-want +got), want the gate's result words before the call's own:\n%s", diff)
			}
		})
	}
}

// TestRunTierNamesEnvThatDiffersFromTheSession pins the env line of a tier run.
// The line names each env value the session sets otherwise, with the tier's value and never the session's.
// It prints once, however many copies the run starts.
func TestRunTierNamesEnvThatDiffersFromTheSession(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const sessionValue = "/session/CommonLib"
	session := []string{"COMMONLIB_REPO=" + sessionValue, "SAME=x"}
	const differingLine = "gate test: tier1 runs with env values that differ from this session's environment: COMMONLIB_REPO=/tier/CommonLib; " +
		"a result of tier1 reflects the tier's values, not the session's; " +
		"way forward: read the result under those values, or change tiers.tier1.env with \"lym config gate\" from the prime"
	tests := []struct {
		name   string
		env    map[string]string
		copies int
		// wantLine is the line stderr and the log hold, or "" when stderr stays empty.
		wantLine string
	}{
		{
			name:     "AValueTheSessionSetsOtherwiseIsNamedWithoutTheSessionsValue",
			env:      map[string]string{"COMMONLIB_REPO": "/tier/CommonLib", "SAME": "x", "ONLY_TIER": "y"},
			wantLine: differingLine,
		},
		{
			name:     "CopiesPrintTheLineOnce",
			env:      map[string]string{"COMMONLIB_REPO": "/tier/CommonLib"},
			copies:   2,
			wantLine: differingLine,
		},
		{name: "ValuesTheSessionSharesOrLacksNameNothing", env: map[string]string{"SAME": "x", "ONLY_TIER": "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var stderr bytes.Buffer
			request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay, sessionEnv: session, copiesRun: tt.copies}
			run := tierRun{number: 1, tier: gateslot.Tier{Command: "true", Env: tt.env, Counter: counterGoJSON, TimeoutMin: 1}, invocations: []invocation{{dir: dir}}}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if result.refusal != "" || result.record.ExitCode != 0 {
				t.Fatalf("runTier() = refusal %q, exit %d; want exit 0 and no refusal", result.refusal, result.record.ExitCode)
			}
			wantStderr := ""
			if tt.wantLine != "" {
				wantStderr = tt.wantLine + "\n"
			}
			if stderr.String() != wantStderr {
				t.Errorf("runTier() stderr = %q; want %q", stderr.String(), wantStderr)
			}
			if tt.wantLine != "" && !strings.Contains(string(result.log), tt.wantLine+"\n") {
				t.Errorf("runTier() log = %q; want it to hold %q", result.log, tt.wantLine)
			}
			if strings.Contains(stderr.String(), sessionValue) || strings.Contains(string(result.log), sessionValue) {
				t.Errorf("runTier() stderr = %q, log = %q; want the session's value %q in neither", stderr.String(), result.log, sessionValue)
			}
		})
	}
}
