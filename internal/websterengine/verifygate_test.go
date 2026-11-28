package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// gateFake scripts the seams of a verify gate closure and counts what it was asked to do.
type gateFake struct {
	outcome   string
	command   string
	dirty     []string
	head      string
	commits   []string
	rejection [2]string
	// results are the verify results, consumed one per verify call.
	// The last one repeats.
	// Each call's result names its own log, `/verify/verify-<call>.log`.
	results []verifytree.Result
	// logs are the contents of the verify logs, one per verify call in call order.
	// The last one repeats.
	logs []string
	// commitLogs are the entries the commitLog seam returns.
	commitLogs []CommitLogEntry
	// commitLogErr is returned by the commitLog seam in place of commitLogs.
	commitLogErr error

	// commitLogCalls counts the commitLog seam's calls.
	commitLogCalls int
	// issuedLogs are the log paths the verify calls named, in call order.
	issuedLogs     []string
	verifyCalls    int
	rejectionBases []string
	sites          []verifytree.Site
	// recorded holds the heads recordPreFix was called with, in order.
	recorded []string
	cleared  int
	// recordErr is returned by recordPreFix.
	recordErr error
}

func (f *gateFake) seams() verifyGateSeams {
	return verifyGateSeams{
		outcome:       func() (string, error) { return f.outcome, nil },
		verifyCommand: func() (string, error) { return f.command, nil },
		dirty:         func() ([]string, error) { return f.dirty, nil },
		head:          func() (string, error) { return f.head, nil },
		fixRejection: func(base string) (string, string, error) {
			f.rejectionBases = append(f.rejectionBases, base)
			return f.rejection[0], f.rejection[1], nil
		},
		commitsSince: func(string) ([]string, error) { return f.commits, nil },
		commitLog: func(string) ([]CommitLogEntry, error) {
			f.commitLogCalls++
			if f.commitLogErr != nil {
				return nil, f.commitLogErr
			}
			return f.commitLogs, nil
		},
		verify: func(site verifytree.Site, command string) (verifytree.Result, error) {
			f.sites = append(f.sites, site)
			res := f.results[min(f.verifyCalls, len(f.results)-1)]
			f.verifyCalls++
			res.Log = fmt.Sprintf("/verify/verify-%d.log", f.verifyCalls)
			f.issuedLogs = append(f.issuedLogs, res.Log)
			return res, nil
		},
		readLog: func(path string) (string, error) {
			run := slices.Index(f.issuedLogs, path)
			if run < 0 {
				return "", fmt.Errorf("no verify call named log %s", path)
			}
			return f.logs[min(run, len(f.logs)-1)], nil
		},
		trail: func() ([]string, []string, error) {
			return []string{"c1", "c2"}, []string{"01-first", "02-second"}, nil
		},
		changedPaths: func(sha string) ([]string, error) {
			if sha == "c2" {
				return []string{"internal/a/a.go"}, nil
			}
			return []string{"internal/b/b.go"}, nil
		},
		modulePath: func() string { return testModulePath },
		recordPreFix: func(head string) error {
			f.recorded = append(f.recorded, head)
			return f.recordErr
		},
		clearPreFix: func() error {
			f.cleared++
			return nil
		},
	}
}

const failingPackageLog = "FAIL\texample.com/mod/internal/a\t0.01s\n"

func failedResult() verifytree.Result {
	return verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
}

func passedResult() verifytree.Result {
	return verifytree.Result{Status: verifytree.StatusPassed}
}

// TestVerifyGate_PassesWithoutVerifying proves the gate passes without running the verify.
// The gate passes when the outcome is stuck, even over a dirty tree, and when outcome.yaml is unreadable;
// the unreadable file is left to the run's own mapping.
// It also proves a gate-config refusal on a done outcome is returned as the gate's error.
// Such a refusal is never a pass, and no verify runs.
func TestVerifyGate_PassesWithoutVerifying(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		fake gateFake
		// outcomeErr makes the outcome seam fail.
		outcomeErr error
		// commandErr makes the verifyCommand seam fail with a gate-config refusal, which the gate returns as its error.
		commandErr error
	}{
		{name: "a stuck outcome", fake: gateFake{outcome: outcomeStuck, command: "lym gate test --wait --tier 0 --all", dirty: []string{"x.go"}}},
		{name: "a gate-config refusal", fake: gateFake{outcome: outcomeDone}, commandErr: errors.New("gate config: gates holds no list for \"final\"; fix it with \"lym config gate\" from the prime")},
		{name: "an unreadable outcome", fake: gateFake{command: "lym gate test --wait --tier 0 --all", results: []verifytree.Result{failedResult()}}, outcomeErr: errors.New("no outcome.yaml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := tt.fake
			s := f.seams()
			if tt.outcomeErr != nil {
				s.outcome = func() (string, error) { return "", tt.outcomeErr }
			}
			if tt.commandErr != nil {
				s.verifyCommand = func() (string, error) { return "", tt.commandErr }
			}
			gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, testFixPromptPath, s)

			res, err := gate()
			if tt.commandErr != nil {
				if err == nil || !errors.Is(err, tt.commandErr) || !strings.Contains(err.Error(), "lym config gate") || res.Passed {
					t.Fatalf("gate() = %+v, %v; want the gate-config refusal as the error", res, err)
				}
			} else if err != nil || !res.Passed {
				t.Fatalf("gate() = %+v, %v; want a pass", res, err)
			}
			if f.verifyCalls != 0 {
				t.Errorf("verify ran %d time(s); want none", f.verifyCalls)
			}
		})
	}
}

//testtiming:keep pins the rerun's flaky note naming the failing package and every verify site's gate label and attempt; the run-level verify-gate test observes only the pass
func TestVerifyGate_FlakyFailurePassesOnRerunWithNote(t *testing.T) {
	tiergate.Require(t, 0)
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		results: []verifytree.Result{failedResult(), passedResult()},
		logs:    []string{failingPackageLog},
	}
	notes := &VerifyGateNotes{}
	gate := newVerifyGate(t.TempDir(), 3, notes, testFixPromptPath, f.seams())

	res, err := gate()
	if err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass on rerun", res, err)
	}
	if f.verifyCalls != 2 {
		t.Errorf("verify ran %d time(s); want the run and one rerun", f.verifyCalls)
	}
	if want := []string{"example.com/mod/internal/a"}; !reflect.DeepEqual(notes.Flaky(), want) {
		t.Errorf("notes.Flaky() = %v; want %v", notes.Flaky(), want)
	}
	for _, site := range f.sites {
		if site.Label != "webster gate" || site.Attempt != 1 {
			t.Errorf("verify site = %+v; want label %q, attempt 1", site, "webster gate")
		}
	}
}

// TestVerifyGate_HungVerifyFailsAtOnceWithoutRerun pins the gate's failure on a verify that timed out or hung.
// The gate fails at once, on the first run or on the rerun,
// and the report names the timeout or the hang, the log path and the log tail.
// A failure that is no hang is still rerun.
func TestVerifyGate_HungVerifyFailsAtOnceWithoutRerun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const tierDetail = "gate tier2 exceeded timeout_min 30m0s and was killed"
	timedOut := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: tierDetail}
	exit := func(code int, detail string) verifytree.Result {
		return verifytree.Result{Status: verifytree.StatusFailed, ExitCode: code, Detail: detail}
	}
	watchdogLog := gateslot.WatchdogLine("/tmp/x/a.test", 42, 2*time.Minute) + "\nSIGQUIT: quit\n" + failingPackageLog
	tests := []struct {
		name          string
		results       []verifytree.Result
		logs          []string
		wantVerifyRun int
		// wantLog is the failed run's own log.
		wantLog string
		// wantTimedOut is set when the failure is the verify timeout.
		wantTimedOut bool
		// wantHang is the report's Hang, empty for a failure that is no hang.
		wantHang   string
		wantStacks bool
		wantWindow string
	}{
		{name: "first run times out", results: []verifytree.Result{timedOut}, logs: []string{"hung in TestSlow\n"}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantTimedOut: true},
		{name: "rerun times out", results: []verifytree.Result{failedResult(), timedOut}, logs: []string{failingPackageLog, "hung in TestSlow\n"}, wantVerifyRun: 2, wantLog: "/verify/verify-2.log", wantTimedOut: true},
		{name: "a go test timeout panic", results: []verifytree.Result{failedResult()}, logs: []string{"panic: test timed out after 10m0s\n" + failingPackageLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "panic: test timed out after", wantStacks: true},
		{name: "a SIGQUIT dump", results: []verifytree.Result{failedResult()}, logs: []string{"SIGQUIT: quit\n" + failingPackageLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "SIGQUIT: quit", wantStacks: true},
		{name: "a CPU watchdog quit", results: []verifytree.Result{failedResult()}, logs: []string{watchdogLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "CPU watchdog SIGQUIT", wantStacks: true, wantWindow: "2m0s"},
		{name: "a test binary ended by a signal", results: []verifytree.Result{failedResult()}, logs: []string{"signal: killed\n" + failingPackageLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "signal: killed"},
		{name: "an exit code of 128 or above", results: []verifytree.Result{exit(137, "")}, logs: []string{failingPackageLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "exit code 137"},
		{name: "a shell killed by a signal", results: []verifytree.Result{exit(-1, "")}, logs: []string{failingPackageLog}, wantVerifyRun: 1, wantLog: "/verify/verify-1.log", wantHang: "exit code -1"},
		{name: "a rerun that hangs", results: []verifytree.Result{failedResult(), failedResult()}, logs: []string{failingPackageLog, "SIGQUIT: quit\n" + failingPackageLog}, wantVerifyRun: 2, wantLog: "/verify/verify-2.log", wantHang: "SIGQUIT: quit", wantStacks: true},
		{name: "a shell that could not start is no hang", results: []verifytree.Result{exit(-1, "no shell")}, logs: []string{failingPackageLog}, wantVerifyRun: 2, wantLog: "/verify/verify-2.log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reports := t.TempDir()
			f := &gateFake{
				outcome: outcomeDone,
				command: "go test ./...",
				head:    "head0",
				results: tt.results,
				logs:    tt.logs,
			}
			notes := &VerifyGateNotes{}
			gate := newVerifyGate(reports, 3, notes, testFixPromptPath, f.seams())

			res, err := gate()
			if err != nil || res.Passed || res.Terminal {
				t.Fatalf("gate() = %+v, %v; want a plain failure", res, err)
			}
			if f.verifyCalls != tt.wantVerifyRun {
				t.Errorf("verify ran %d time(s); want %d", f.verifyCalls, tt.wantVerifyRun)
			}
			report, err := readVerifyGateReport(VerifyGateReportPath(reports))
			if err != nil {
				t.Fatalf("readVerifyGateReport() error = %v", err)
			}
			if report.LogPath != tt.wantLog {
				t.Errorf("report log path = %q; want %q", report.LogPath, tt.wantLog)
			}
			if tt.wantTimedOut {
				if report.TimedOut != tierDetail || !strings.Contains(report.LogTail, "hung in TestSlow") || len(report.Failures) != 0 || report.Hang != "" {
					t.Errorf("report = %+v; want the timeout and the log tail, with no failures and no hang", report)
				}
				for _, want := range []string{"timed out: " + tierDetail, "not re-run", tt.wantLog, "hung in TestSlow"} {
					if !strings.Contains(res.Findings, want) {
						t.Errorf("findings = %q; want %q in it", res.Findings, want)
					}
				}
				return
			}
			if report.Hang != tt.wantHang || report.HangStacks != tt.wantStacks || report.WatchdogWindow != tt.wantWindow || report.TimedOut != "" {
				t.Errorf("report hang = (%q, stacks %v, window %q, timed out %q); want (%q, %v, %q) and no timeout", report.Hang, report.HangStacks, report.WatchdogWindow, report.TimedOut, tt.wantHang, tt.wantStacks, tt.wantWindow)
			}
			if got := failureIDs(report.Failures); !slices.Equal(got, []string{"example.com/mod/internal/a"}) {
				t.Errorf("report failures = %v; want the identity the log parses to", got)
			}
			if tt.wantHang == "" {
				return
			}
			if flaky := notes.Flaky(); len(flaky) != 0 {
				t.Errorf("notes.Flaky() = %v; want none for a hang", flaky)
			}
			if !strings.Contains(res.Findings, "not re-run") || !strings.Contains(res.Findings, tt.wantHang) {
				t.Errorf("findings = %q; want them to say the step was not re-run and name %q", res.Findings, tt.wantHang)
			}
		})
	}
}

// TestVerifyGate_RunnerCrashFailsTerminalWithoutFixer pins the gate's terminal failure on a crashed test runner.
// The gate fails at once, on the first run or the rerun, and names the crashed runs.
// It leaves no pre-fix head and no failure report, and its stuck reason ends in reentry alone.
func TestVerifyGate_RunnerCrashFailsTerminalWithoutFixer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const crashDetail = "gate test runner crashed with no failing test: tier1 (exit 134, SIGABRT)"
	crashed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 134, RunnerCrash: true, Detail: crashDetail}
	tests := []struct {
		name          string
		results       []verifytree.Result
		wantVerifyRun int
	}{
		{name: "first run crashes", results: []verifytree.Result{crashed}, wantVerifyRun: 1},
		{name: "rerun crashes", results: []verifytree.Result{failedResult(), crashed}, wantVerifyRun: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reports := t.TempDir()
			f := &gateFake{outcome: outcomeDone, command: "go test ./...", head: "head0", results: tt.results, logs: []string{failingPackageLog}}
			gate := newVerifyGate(reports, 3, &VerifyGateNotes{}, testFixPromptPath, f.seams())

			res, err := gate()

			if err != nil || res.Passed || !res.Terminal {
				t.Fatalf("gate() = %+v, %v; want a terminal failure", res, err)
			}
			if f.verifyCalls != tt.wantVerifyRun {
				t.Errorf("verify ran %d time(s); want %d", f.verifyCalls, tt.wantVerifyRun)
			}
			for _, want := range []string{crashDetail, "no fixer runs", f.issuedLogs[len(f.issuedLogs)-1]} {
				if !strings.Contains(res.Findings, want) {
					t.Errorf("findings = %q; want %q in it", res.Findings, want)
				}
			}
			if len(f.recorded) != 0 {
				t.Errorf("pre-fix heads recorded = %v; want none for a crash", f.recorded)
			}
			if _, err := os.Stat(VerifyGateReportPath(reports)); err == nil {
				t.Error("a verify-gate report was written; want none, since no fixer reads it")
			}
			reason := verifyGateStuckReason(reports, &shuttleengine.GateOutcome{Reason: res.Findings}, "lym webster run")
			if !strings.HasSuffix(reason, "way forward: lym webster run") || strings.Contains(reason, "reset --to pre-fix") || strings.Contains(reason, "\n") || !strings.Contains(reason, crashDetail) {
				t.Errorf("verifyGateStuckReason() = %q; want one line naming the crash and ending in reentry alone", reason)
			}
		})
	}
}

//testtiming:keep pins the failure report's attempt, cap, failures, card hint, fix commits and log path, and the findings naming them; the run-level verify-gate test observes only the failure
func TestVerifyGate_FailedEvaluationWritesReport(t *testing.T) {
	tiergate.Require(t, 0)
	reports := t.TempDir()
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		head:    "head0",
		results: []verifytree.Result{failedResult()},
		logs:    []string{failingPackageLog},
	}
	gate := newVerifyGate(reports, 3, &VerifyGateNotes{}, testFixPromptPath, f.seams())

	first, err := gate()
	if err != nil || first.Passed || first.Terminal {
		t.Fatalf("first gate() = %+v, %v; want a plain failure", first, err)
	}
	f.commits = []string{"fix1", "fix2"}
	second, err := gate()
	if err != nil || second.Passed {
		t.Fatalf("second gate() = %+v, %v; want a failure", second, err)
	}

	report, err := readVerifyGateReport(VerifyGateReportPath(reports))
	if err != nil {
		t.Fatalf("readVerifyGateReport() error = %v", err)
	}
	if report.Attempt != 2 || report.Cap != 3 {
		t.Errorf("report attempt/cap = %d/%d; want 2/3", report.Attempt, report.Cap)
	}
	if len(report.Failures) != 1 || report.Failures[0].ID != "example.com/mod/internal/a" {
		t.Errorf("report failures = %+v; want the failing package", report.Failures)
	}
	if want := []string{"02-second"}; !reflect.DeepEqual(report.Hint, want) {
		t.Errorf("report hint = %v; want %v", report.Hint, want)
	}
	if want := []string{"fix1", "fix2"}; !reflect.DeepEqual(report.FixCommits, want) {
		t.Errorf("report fix commits = %v; want %v", report.FixCommits, want)
	}
	// The second evaluation's rerun is the fourth verify call, and the report names its log.
	if report.LogPath != "/verify/verify-4.log" {
		t.Errorf("report log path = %q; want the log of the run it reports on, /verify/verify-4.log", report.LogPath)
	}
	for _, want := range []string{"example.com/mod/internal/a", "02-second", "evaluation 2; the session may be re-prompted 3 time(s)"} {
		if !strings.Contains(second.Findings, want) {
			t.Errorf("findings = %q; want %q in it", second.Findings, want)
		}
	}
	if !strings.HasPrefix(second.Findings, verifyGateWayForward(FixerFork, testFixPromptPath)+"\n") {
		t.Errorf("findings = %q; want them to open with the way forward naming %s", second.Findings, testFixPromptPath)
	}
}

// TestVerifyGate_RoutesFailuresToFixer pins which fixer each failure goes to.
// The report, the findings and the re-prompt line name that fixer.
// The failure that reaches the cap and the one after a fixer that added no commit go cold,
// and every failure after a cold one stays cold.
func TestVerifyGate_RoutesFailuresToFixer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		cap  int
		// commits are the fix commits above the pre-fix head at each failure, one entry per evaluation.
		commits [][]string
		want    []string
	}{
		{name: "one fork then the cold fixer at the cap", cap: 2, commits: [][]string{nil, {"fix1"}}, want: []string{FixerFork, FixerCold}},
		{name: "forks below the cap while fixers commit", cap: 3, commits: [][]string{nil, {"fix1"}, {"fix1", "fix2"}}, want: []string{FixerFork, FixerFork, FixerCold}},
		{name: "a fixer that committed nothing escalates and routing stays cold", cap: 4, commits: [][]string{nil, nil, {"fix1"}}, want: []string{FixerFork, FixerCold, FixerCold}},
		{name: "a cap of one goes cold at once", cap: 1, commits: [][]string{nil}, want: []string{FixerCold}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reports := t.TempDir()
			f := &gateFake{
				outcome: outcomeDone,
				command: "go test ./...",
				head:    "head0",
				results: []verifytree.Result{failedResult()},
				logs:    []string{failingPackageLog},
			}
			notes := &VerifyGateNotes{}
			gate := newVerifyGate(reports, tt.cap, notes, testFixPromptPath, f.seams())
			reprompt := verifyGateReprompt(testFixPromptPath, notes)

			for i, want := range tt.want {
				f.commits = tt.commits[i]
				res, err := gate()
				if err != nil || res.Passed {
					t.Fatalf("gate() #%d = %+v, %v; want a failure", i+1, res, err)
				}
				report, err := readVerifyGateReport(VerifyGateReportPath(reports))
				if err != nil {
					t.Fatalf("readVerifyGateReport() error = %v", err)
				}
				if report.Fixer != want {
					t.Errorf("failure #%d report fixer = %q; want %q", i+1, report.Fixer, want)
				}
				wayForward := verifyGateWayForward(want, testFixPromptPath)
				if !strings.HasPrefix(res.Findings, wayForward+"\n") {
					t.Errorf("failure #%d findings = %q; want them to open with %q", i+1, res.Findings, wayForward)
				}
				if got, wantLine := reprompt("/findings"), "Gate findings recorded at /findings — "+wayForward; got != wantLine {
					t.Errorf("failure #%d re-prompt = %q; want %q", i+1, got, wantLine)
				}
			}
		})
	}
}

// TestVerifyGate_NotesFixCommitsOnce pins that the gate reads the commit log only once a pre-fix head is recorded.
// The notes keep each commit once across evaluations.
// They list the `fix:` commit with its departure, but not a plain commit.
// A failing commit log read is the gate's error.
func TestVerifyGate_NotesFixCommitsOnce(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fix := CommitLogEntry{SHA: "fix1", Subject: "fix: repair a", Departures: []string{"PATTERN-x — differs"}}
	plain := CommitLogEntry{SHA: "other1", Subject: "chore: tidy"}
	f := &gateFake{
		outcome:    outcomeDone,
		command:    "go test ./...",
		head:       "head0",
		results:    []verifytree.Result{failedResult(), failedResult(), passedResult()},
		logs:       []string{failingPackageLog},
		commitLogs: []CommitLogEntry{fix, plain},
	}
	notes := &VerifyGateNotes{}
	gate := newVerifyGate(t.TempDir(), 3, notes, testFixPromptPath, f.seams())

	if res, err := gate(); err != nil || res.Passed {
		t.Fatalf("first gate() = %+v, %v; want a failure", res, err)
	}
	if f.commitLogCalls != 0 {
		t.Errorf("commit log read %d time(s) before any pre-fix head; want none", f.commitLogCalls)
	}
	for i := 0; i < 2; i++ {
		if res, err := gate(); err != nil || !res.Passed {
			t.Fatalf("gate() after the fix = %+v, %v; want a pass", res, err)
		}
	}
	if f.commitLogCalls != 2 {
		t.Errorf("commit log read %d time(s); want one per evaluation after the failure", f.commitLogCalls)
	}
	if got, want := notes.FixCommits(), []CommitLogEntry{fix}; !reflect.DeepEqual(got, want) {
		t.Errorf("notes.FixCommits() = %+v; want %+v once", got, want)
	}

	failing := &gateFake{
		outcome:      outcomeDone,
		command:      "go test ./...",
		head:         "head0",
		results:      []verifytree.Result{failedResult()},
		logs:         []string{failingPackageLog},
		commitLogErr: errors.New("log unreadable"),
	}
	second := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, testFixPromptPath, failing.seams())
	if res, err := second(); err != nil || res.Passed {
		t.Fatalf("second gate's first gate() = %+v, %v; want a failure", res, err)
	}
	if _, err := second(); err == nil || !strings.Contains(err.Error(), "log unreadable") {
		t.Fatalf("gate() error = %v; want the commit log failure", err)
	}
}

// TestVerifyGate_DirtyTreeRecordsPreFixHead walks one fake through dirty-tree failures:
// a dirty tree fails without verifying or checking fix commits, and reports the dirty paths.
// The pre-fix head is recorded once per closure and overwritten by a new closure.
// The first clean pass checks fix commits from the recorded head and clears the record.
//
//testtiming:keep pins the pre-fix head recorded once per closure, overwritten by a new closure and cleared only on a pass, and the dirty report; the covering parent-merge gate test only passes a clean tree
func TestVerifyGate_DirtyTreeRecordsPreFixHead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	reports := t.TempDir()
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		head:    "head0",
		dirty:   []string{"loose.go"},
		results: []verifytree.Result{passedResult()},
	}
	gate := newVerifyGate(reports, 5, &VerifyGateNotes{}, testFixPromptPath, f.seams())

	for i := 0; i < 2; i++ {
		if res, err := gate(); err != nil || res.Passed {
			t.Fatalf("gate() on a dirty tree = %+v, %v; want a failure", res, err)
		}
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s) on a dirty tree; want none", f.verifyCalls)
	}
	if len(f.rejectionBases) != 0 {
		t.Fatalf("commit check ran %v before any pass; want none", f.rejectionBases)
	}
	report, err := readVerifyGateReport(VerifyGateReportPath(reports))
	if err != nil {
		t.Fatalf("readVerifyGateReport() error = %v", err)
	}
	if want := []string{"loose.go"}; !reflect.DeepEqual(report.Dirty, want) {
		t.Errorf("report dirty = %v; want %v", report.Dirty, want)
	}
	if want := []string{"head0"}; !reflect.DeepEqual(f.recorded, want) {
		t.Errorf("recorded heads after two failures = %v; want %v once", f.recorded, want)
	}

	f.head = "head1"
	second := newVerifyGate(t.TempDir(), 5, &VerifyGateNotes{}, testFixPromptPath, f.seams())
	if res, err := second(); err != nil || res.Passed {
		t.Fatalf("second closure's gate() = %+v, %v; want a failure", res, err)
	}
	if want := []string{"head0", "head1"}; !reflect.DeepEqual(f.recorded, want) {
		t.Errorf("recorded heads = %v; want the new closure to overwrite with %v", f.recorded, want)
	}
	if f.cleared != 0 {
		t.Errorf("clearPreFix ran %d time(s) before any pass; want none", f.cleared)
	}

	f.dirty = nil
	if res, err := second(); err != nil || !res.Passed {
		t.Fatalf("gate() after the tree was cleaned = %+v, %v; want a pass", res, err)
	}
	if want := []string{"head1"}; !reflect.DeepEqual(f.rejectionBases, want) {
		t.Errorf("commit check bases = %v; want the recorded pre-fix head %v", f.rejectionBases, want)
	}
	if f.cleared != 1 {
		t.Errorf("clearPreFix ran %d time(s) on a pass; want 1", f.cleared)
	}
}

func TestVerifyGate_PersistFailureIsTheGatesError(t *testing.T) {
	tiergate.Require(t, 0)
	f := &gateFake{
		outcome:   outcomeDone,
		command:   "go test ./...",
		head:      "head0",
		dirty:     []string{"loose.go"},
		recordErr: errors.New("disk full"),
	}
	gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, testFixPromptPath, f.seams())

	if _, err := gate(); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("gate() error = %v; want the persist failure", err)
	}
}

//testtiming:keep pins a rejected fix commit failing the gate terminally without verifying, its findings naming the commit, the reason and the pre-fix head and never telling Merriam to move HEAD; the covering merge test runs against real git and checks only the terminal failure
func TestVerifyGate_RejectedFixCommitFailsTerminal(t *testing.T) {
	tiergate.Require(t, 0)
	f := &gateFake{
		outcome:   outcomeDone,
		command:   "go test ./...",
		head:      "head0",
		dirty:     []string{"loose.go"},
		rejection: [2]string{"mergeabc", "it merged a commit that is not on the run's parent branch"},
		results:   []verifytree.Result{passedResult()},
	}
	gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, testFixPromptPath, f.seams())
	if res, err := gate(); err != nil || res.Passed {
		t.Fatalf("first gate() = %+v, %v; want a dirty failure", res, err)
	}

	f.dirty = nil
	res, err := gate()
	if err != nil {
		t.Fatalf("second gate() error = %v", err)
	}
	if res.Passed || !res.Terminal {
		t.Fatalf("second gate() = %+v; want a Terminal failure", res)
	}
	for _, want := range []string{"mergeabc", "not on the run's parent branch", "head0"} {
		if !strings.Contains(res.Findings, want) {
			t.Errorf("findings = %q; want %q in it", res.Findings, want)
		}
	}
	if strings.Contains(strings.ToLower(res.Findings), "move head") {
		t.Errorf("findings = %q; want no instruction to move HEAD", res.Findings)
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s) after a rejected fix commit; want none", f.verifyCalls)
	}
}

func TestVerifyGateStuckReason_TerminalRejectionEndsInResetToPreFix(t *testing.T) {
	tiergate.Require(t, 0)
	gate := &shuttleengine.GateOutcome{Reason: "Commit mergeabc is rejected.\nIt is not on the parent branch."}
	for _, tc := range []struct{ reentry, want string }{
		{"lym webster run", "way forward: 1) lym webster reset --to pre-fix; 2) lym webster run"},
		{"re-step the Webster row", "way forward: 1) lym webster reset --to pre-fix; 2) re-step the Webster row"},
	} {
		got := verifyGateStuckReason(t.TempDir(), gate, tc.reentry)
		if !strings.HasSuffix(got, tc.want) || strings.Contains(got, "\n") {
			t.Errorf("verifyGateStuckReason(%q) = %q; want one line ending %q", tc.reentry, got, tc.want)
		}
	}
}

// TestVerifyGateStuckReason_NamesTheFailureAndAWayForward pins the stuck reason of a verify gate.
// The gate spent its re-prompts.
// It names the verify command and what failed, and ends in a way forward.
// It quotes the output tail of a failure that named no test.
func TestVerifyGateStuckReason_NamesTheFailureAndAWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		log  = "/wt/.lym/verify/verify.log"
		cmd  = "lym gate test --gate webster"
		lead = "verify gate did not pass after 2 of 2 re-prompt(s): "
		fix  = "way forward: 1) read " + log + " and commit a fix on the task branch; 2) lym webster run"
	)
	tests := []struct {
		name   string
		report VerifyGateReport
		want   string
	}{
		{
			name:   "a timed-out report names the timeout, that nothing was re-run and the log",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, TimedOut: "gate tier2 exceeded timeout_min 30m0s and was killed", LogPath: log},
			want:   lead + "`" + cmd + "`: the verify command timed out (gate tier2 exceeded timeout_min 30m0s and was killed) and was not re-run, see " + log + "; " + fix,
		},
		{
			name:   "a SIGQUIT report names the hang, the stacks and the log",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, Hang: "SIGQUIT: quit", HangStacks: true, LogPath: log},
			want:   lead + "`" + cmd + "`: the verify command hung (SIGQUIT: quit) and was not re-run; its goroutine stacks are in " + log + "; " + fix,
		},
		{
			name:   "a watchdog report names the window and its own way forward with the re-entry",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, Hang: "CPU watchdog SIGQUIT", HangStacks: true, WatchdogWindow: "2m0s", LogPath: log},
			want: lead + "`" + cmd + "`: the verify command hung (CPU watchdog SIGQUIT) and was not re-run; its goroutine stacks are in " + log +
				"; the gate test's CPU watchdog quit a test binary busy for 2m0s: if that package is CPU-bound but healthy, " + gateslot.BusyQuitWayForward + ", then lym webster run",
		},
		{
			name:   "an exit-code report names the code and holds no stacks",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, Hang: "exit code 137", LogPath: log},
			want:   lead + "`" + cmd + "`: the verify command was killed or exited with exit code 137 and was not re-run, see " + log + "; " + fix,
		},
		{
			name:   "a report with failing identities names them and the log",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, ExitCode: 1, Failures: []VerifyFailure{{ID: "example.com/mod/internal/a"}, {ID: "example.com/mod/internal/b.TestX"}}, LogPath: log},
			want:   lead + "`" + cmd + "`: failing example.com/mod/internal/a, example.com/mod/internal/b.TestX, see " + log + "; " + fix,
		},
		{
			name:   "a failure that named no test says so and quotes the output tail",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, ExitCode: 2, LogTail: "gate: tier0\nbuild failed: x.go:3: undefined: y", LogPath: log},
			want: lead + "`" + cmd + "`: the verify command exited with code 2 and its output named no failing test, so the gate itself may have erred; " +
				"output tail: gate: tier0 build failed: x.go:3: undefined: y; see " + log + "; " + fix,
		},
		{
			name:   "an empty output says so instead of quoting a tail",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, ExitCode: 3, LogPath: log},
			want:   lead + "`" + cmd + "`: the verify command exited with code 3 and its output named no failing test, so the gate itself may have erred; its output was empty; see " + log + "; " + fix,
		},
		{
			name:   "an output tail is cut on a rune boundary",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, ExitCode: 1, LogTail: "xé" + strings.Repeat("a", verifyGateStuckTextBytes-1), LogPath: log},
			want: lead + "`" + cmd + "`: the verify command exited with code 1 and its output named no failing test, so the gate itself may have erred; " +
				"output tail: " + strings.Repeat("a", verifyGateStuckTextBytes-1) + "; see " + log + "; " + fix,
		},
		{
			name:   "a long entry findings text is cut to its first bytes on a rune boundary",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Entry: "drift", Findings: strings.Repeat("a", verifyGateStuckTextBytes-1) + "étail"},
			want: lead + `the run-level "drift" entry failed: ` + strings.Repeat("a", verifyGateStuckTextBytes-1) + `; ` +
				`way forward: 1) fix what the "drift" entry's findings name and commit it on the task branch; 2) lym webster run`,
		},
		{
			name:   "a long output tail is cut to its last bytes",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Command: cmd, ExitCode: 1, LogTail: strings.Repeat("a", 500) + "END", LogPath: log},
			want: lead + "`" + cmd + "`: the verify command exited with code 1 and its output named no failing test, so the gate itself may have erred; " +
				"output tail: " + strings.Repeat("a", verifyGateStuckTextBytes-3) + "END; see " + log + "; " + fix,
		},
		{
			name:   "a dirty tree found before any verify names the paths and no command",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Dirty: []string{"a.go", "b.txt"}},
			want:   lead + "the worktree held uncommitted paths a.go, b.txt; way forward: 1) commit or remove those paths on the task branch; 2) lym webster run",
		},
		{
			name:   "a run-level entry's failure names the entry and its findings",
			report: VerifyGateReport{Attempt: 3, Cap: 2, Entry: "drift", Findings: "cell a.txt:3 moved\nsee expected.yaml"},
			want: lead + `the run-level "drift" entry failed: cell a.txt:3 moved see expected.yaml; ` +
				`way forward: 1) fix what the "drift" entry's findings name and commit it on the task branch; 2) lym webster run`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reportsDir := t.TempDir()
			if err := WriteVerifyGateReport(VerifyGateReportPath(reportsDir), tt.report); err != nil {
				t.Fatal(err)
			}
			gate := &shuttleengine.GateOutcome{Attempts: 2, Entries: []shuttleengine.GateEntryOutcome{{Name: verifyGateName, Attempts: 2, State: shuttleengine.GateEntryFailed}}}

			got := verifyGateStuckReason(reportsDir, gate, "lym webster run")

			if got != tt.want {
				t.Errorf("verifyGateStuckReason() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestVerifyGateStuckReason_CountsRepromptsAgainstTheLimit pins the stuck reason's opening.
// It counts the failing entry's own re-prompts against the entry's limit, so the two agree,
// and a run that ended with re-prompts left says why.
// With no readable report it counts the re-prompts alone and still ends in the re-entry.
func TestVerifyGateStuckReason_CountsRepromptsAgainstTheLimit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	failed := func(name string, sent int) shuttleengine.GateEntryOutcome {
		return shuttleengine.GateEntryOutcome{Name: name, Attempts: sent, State: shuttleengine.GateEntryFailed}
	}
	tests := []struct {
		name       string
		gate       shuttleengine.GateOutcome
		noReport   bool
		wantPrefix string
		wantSuffix string
	}{
		{
			name:       "a spent budget counts every re-prompt against the limit",
			gate:       shuttleengine.GateOutcome{Attempts: 3, Entries: []shuttleengine.GateEntryOutcome{failed(verifyGateName, 3)}},
			wantPrefix: "verify gate did not pass after 3 of 3 re-prompt(s): ",
		},
		{
			name: "the failing entry's own re-prompts count, not the run's total",
			gate: shuttleengine.GateOutcome{Attempts: 5, Entries: []shuttleengine.GateEntryOutcome{
				{Name: verifyGateName, Attempts: 2, State: shuttleengine.GateEntryPassed}, failed("drift", 3),
			}},
			wantPrefix: "verify gate did not pass after 3 of 3 re-prompt(s): ",
		},
		{
			name:       "a gate that ended before any re-prompt says why",
			gate:       shuttleengine.GateOutcome{EndedEarly: "its re-prompt could not be sent: pane gone", Entries: []shuttleengine.GateEntryOutcome{failed(verifyGateName, 0)}},
			wantPrefix: "verify gate failed its first check and the session was never re-prompted, since its re-prompt could not be sent: pane gone: ",
		},
		{
			name:       "a gate that ended with re-prompts left counts them and says why",
			gate:       shuttleengine.GateOutcome{Attempts: 1, EndedEarly: "the session ended", Entries: []shuttleengine.GateEntryOutcome{failed(verifyGateName, 1)}},
			wantPrefix: "verify gate did not pass after 1 of 3 re-prompt(s) and the run ended with re-prompts left, since the session ended: ",
		},
		{
			name:       "a gate with no failed entry counts its own re-prompts",
			gate:       shuttleengine.GateOutcome{Attempts: 2},
			wantPrefix: "verify gate did not pass after 2 of 3 re-prompt(s): ",
		},
		{
			name:       "an unreadable report counts the re-prompts and ends in the re-entry",
			gate:       shuttleengine.GateOutcome{Attempts: 2, Entries: []shuttleengine.GateEntryOutcome{failed(verifyGateName, 2)}},
			noReport:   true,
			wantPrefix: "verify gate did not pass after 2 re-prompt(s); its report could not be read: ",
			wantSuffix: "; way forward: lym webster run",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reportsDir := t.TempDir()
			if !tt.noReport {
				report := VerifyGateReport{Attempt: 4, Cap: 3, Command: "lym gate test --gate webster", Failures: []VerifyFailure{{ID: "example.com/mod/internal/a"}}, LogPath: "/verify.log"}
				if err := WriteVerifyGateReport(VerifyGateReportPath(reportsDir), report); err != nil {
					t.Fatal(err)
				}
			}

			got := verifyGateStuckReason(reportsDir, &tt.gate, "lym webster run")

			if !strings.HasPrefix(got, tt.wantPrefix) || !strings.HasSuffix(got, tt.wantSuffix) || strings.Contains(got, "\n") {
				t.Errorf("verifyGateStuckReason() = %q; want one line opening %q and ending %q", got, tt.wantPrefix, tt.wantSuffix)
			}
		})
	}
}

// TestVerifyGate_ReportKeepsWhatTheFailedArrivalObserved pins the report of a failed verify.
// A log that parses to no failing identity keeps the verify command, the exit code and the log tail,
// so the stuck reason can show what failed.
// A log that names tests keeps them and no tail.
// A dirty tree found on a later arrival carries no command, though an earlier arrival rendered one.
func TestVerifyGate_ReportKeepsWhatTheFailedArrivalObserved(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const noTestOutput = "gate: tier0 could not start: tool missing\n"
	const command = "lym gate test --gate webster"
	tests := []struct {
		name   string
		output string
		// dirtyOnSecondArrival makes the tree dirty for a second arrival, which is the one reported.
		dirtyOnSecondArrival bool
		wantCommand          string
		wantExitCode         int
		wantLogTail          string
		wantLogPath          string
		wantFailureIDs       []string
		wantDirty            []string
	}{
		{
			name: "a failure naming no test keeps the command, exit code and output tail", output: noTestOutput,
			wantCommand: command, wantExitCode: 2, wantLogTail: strings.TrimSpace(noTestOutput), wantLogPath: "/verify/verify-2.log",
		},
		{
			name: "a failure naming tests keeps them and no output tail", output: failingPackageLog,
			wantCommand: command, wantExitCode: 2, wantLogPath: "/verify/verify-2.log", wantFailureIDs: []string{"example.com/mod/internal/a"},
		},
		{
			name: "a dirty tree on a later arrival carries no command", output: noTestOutput, dirtyOnSecondArrival: true,
			wantDirty: []string{"loose.go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reports := t.TempDir()
			f := &gateFake{
				outcome: outcomeDone,
				command: command,
				head:    "head0",
				results: []verifytree.Result{{Status: verifytree.StatusFailed, ExitCode: 2}},
				logs:    []string{tt.output},
			}
			gate := newVerifyGate(reports, 3, &VerifyGateNotes{}, testFixPromptPath, f.seams())

			if res, err := gate(); err != nil || res.Passed || res.Terminal {
				t.Fatalf("gate() = %+v, %v; want a plain failure", res, err)
			}
			if tt.dirtyOnSecondArrival {
				f.dirty = []string{"loose.go"}
				if res, err := gate(); err != nil || res.Passed || res.Terminal {
					t.Fatalf("second gate() = %+v, %v; want a plain failure", res, err)
				}
			}

			report, err := readVerifyGateReport(VerifyGateReportPath(reports))
			if err != nil {
				t.Fatalf("readVerifyGateReport() error = %v", err)
			}
			if report.Command != tt.wantCommand || report.ExitCode != tt.wantExitCode || report.LogTail != tt.wantLogTail || report.LogPath != tt.wantLogPath {
				t.Errorf("report command, exit code, log tail and log = (%q, %d, %q, %q); want (%q, %d, %q, %q)",
					report.Command, report.ExitCode, report.LogTail, report.LogPath, tt.wantCommand, tt.wantExitCode, tt.wantLogTail, tt.wantLogPath)
			}
			if got := failureIDs(report.Failures); !slices.Equal(got, tt.wantFailureIDs) {
				t.Errorf("report failures = %v; want %v", got, tt.wantFailureIDs)
			}
			if !slices.Equal(report.Dirty, tt.wantDirty) {
				t.Errorf("report dirty = %v; want %v", report.Dirty, tt.wantDirty)
			}
		})
	}
}

func TestReadModulePath(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	if got := readModulePath(dir); got != "" {
		t.Errorf("readModulePath() with no go.mod = %q; want empty", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// header\nmodule example.com/mod\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if got := readModulePath(dir); got != testModulePath {
		t.Errorf("readModulePath() = %q; want %q", got, testModulePath)
	}
}
