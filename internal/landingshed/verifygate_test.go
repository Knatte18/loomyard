// verifygate_test.go covers verifyGate.clean and verifyGate.check branch by branch against fake verifytree seams,
// so no test spawns git or a shell.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// fakeVerifyTree scripts both verifytree seams and records every call the gate makes through them.
type fakeVerifyTree struct {
	dirtyCalls  int
	dirtyScript [][]string
	dirtyErr    error

	verifyCalls int
	paths       verifytree.Paths
	site        verifytree.Site
	command     string
	result      verifytree.Result
	verifyErr   error
	onVerify    func()

	// commands and sites record every verify call in order.
	commands []string
	sites    []verifytree.Site
	// resultByCommand overrides result for a command it names.
	resultByCommand map[string]verifytree.Result
	// writeLog, when set, makes each passed or failed call write `verify-<call>.log` to the verify directory.
	writeLog func(path string)
}

// dirtyAt scripts the dirty paths the i-th (zero-based) clean-tree check reports.
// Every other check reports a clean tree.
func (f *fakeVerifyTree) dirtyAt(i int, paths ...string) {
	for len(f.dirtyScript) <= i {
		f.dirtyScript = append(f.dirtyScript, nil)
	}
	f.dirtyScript[i] = paths
}

func (f *fakeVerifyTree) dirty(string) ([]string, error) {
	i := f.dirtyCalls
	f.dirtyCalls++
	if f.dirtyErr != nil {
		return nil, f.dirtyErr
	}
	if i < len(f.dirtyScript) {
		return f.dirtyScript[i], nil
	}
	return nil, nil
}

func (f *fakeVerifyTree) verify(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error) {
	f.verifyCalls++
	f.paths = p
	f.site = site
	f.command = command
	f.commands = append(f.commands, command)
	f.sites = append(f.sites, site)
	if f.onVerify != nil {
		f.onVerify()
	}
	result := f.result
	if scripted, ok := f.resultByCommand[command]; ok {
		result = scripted
	}
	if f.writeLog != nil && (result.Status == verifytree.StatusPassed || result.Status == verifytree.StatusFailed) {
		result.Log = filepath.Join(p.Dir, fmt.Sprintf("verify-%d.log", f.verifyCalls))
		f.writeLog(result.Log)
	}
	return result, f.verifyErr
}

// gateFixture holds a gate for the row of gateKey, built over t.TempDir() paths with fake verifytree seams.
// Its command closure counts its calls and records the key each call was made with.
type gateFixture struct {
	gate         verifyGate
	fake         *fakeVerifyTree
	closureCalls int
	closureKeys  []string
	paths        verifytree.Paths
}

func newGateFixture(t *testing.T, gateKey, command string, closureErr error) *gateFixture {
	t.Helper()
	root := t.TempDir()
	f := &gateFixture{
		fake:  &fakeVerifyTree{result: verifytree.Result{Status: verifytree.StatusPassed}},
		paths: verifytree.NewPaths(root, filepath.Join(root, "verify")),
	}
	f.gate = verifyGate{
		key: gateKey,
		command: func(key string) (string, error) {
			f.closureCalls++
			f.closureKeys = append(f.closureKeys, key)
			return command, closureErr
		},
		tiers:  fixedGateTiers,
		paths:  f.paths,
		dirty:  f.fake.dirty,
		verify: f.fake.verify,
		now:    func() time.Time { return gateMarkStart },
	}
	return f
}

// The calls the fixtures render for the gates, standing in for what the hub's gate config renders.
const (
	gateLandingCall = "lym gate test --wait --tier 0 --tier 1 --all"
	gateBaseCall    = "lym gate test --wait --tier 0 --all"
)

// fixedGateTierLists holds the tier list for each fixture gate; one gate's highest tier differs from its first.
var fixedGateTierLists = map[string][]int{"parent-align": {0, 1}, "finalize": {0, 1}, "webster": {0}}

// fixedGateTiers returns a copy of the tier list for a named gate, or an error for an unknown gate.
func fixedGateTiers(gate string) ([]int, error) {
	list, ok := fixedGateTierLists[gate]
	if !ok {
		return nil, fmt.Errorf("no fixed tier list for gate %q", gate)
	}
	return slices.Clone(list), nil
}

// gateMarkStart is the fixed instant the fixture's gate stamps on its wait mark.
var gateMarkStart = time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)

// waitMarkCall is one call the gate made to its wait-mark callback.
type waitMarkCall struct {
	label string
	start time.Time
}

// TestVerifyGate_WaitMark pins the mark around every verify.
// The mark is cleared, then set to `verify <producer>` at the start time,
// and it is cleared again on every way out of the verify.
// A failing callback or a nil one leaves the verdict unchanged and sets no mark.
func TestVerifyGate_WaitMark(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cases := []struct {
		name       string
		result     verifytree.Result
		verifyErr  error
		markErr    error
		noCallback bool
		wantReason string
		wantErr    string
	}{
		{name: "pass", result: verifytree.Result{Status: verifytree.StatusPassed}},
		{name: "skip", result: verifytree.Result{Status: verifytree.StatusSkipped}},
		{name: "failure", result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}, wantReason: "landing gate failed"},
		{name: "timeout", result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true}, wantReason: "timed out"},
		{name: "dirty", result: verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"a.txt"}}, wantReason: "a.txt"},
		{name: "error", verifyErr: errors.New("exec failed"), wantErr: "exec failed"},
		{name: "cancel", verifyErr: context.Canceled, wantErr: "context canceled"},
		{name: "failing callback changes no verdict", result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}, markErr: errors.New("reed gone"), wantReason: "landing gate failed"},
		{name: "nil callback", result: verifytree.Result{Status: verifytree.StatusPassed}, noCallback: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newGateFixture(t, "finalize", "true", nil)
			f.fake.result = tc.result
			f.fake.verifyErr = tc.verifyErr
			var calls []waitMarkCall
			if !tc.noCallback {
				f.gate.waitMark = func(label string, start time.Time) error {
					calls = append(calls, waitMarkCall{label, start})
					return tc.markErr
				}
			}
			reason, _, err := f.gate.check(context.Background(), "Publish", "main")

			if !strings.Contains(reason, tc.wantReason) || (tc.wantReason == "" && reason != "") {
				t.Errorf("reason = %q, want it to contain %q", reason, tc.wantReason)
			}
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
			if tc.noCallback {
				return
			}
			want := []waitMarkCall{{"", time.Time{}}, {"verify Publish", gateMarkStart}, {"", time.Time{}}}
			if len(calls) != len(want) {
				t.Fatalf("mark calls = %+v, want %+v", calls, want)
			}
			for i := range want {
				if calls[i] != want[i] {
					t.Errorf("mark call %d = %+v, want %+v", i, calls[i], want[i])
				}
			}
		})
	}
}

// TestVerifyGate_CheckMapsVerifyResult pins how each verifytree status maps to the gate's verdict.
// It also pins what the verify seam receives.
// That is the configured command, the gate's paths and the calling producer's site label.
func TestVerifyGate_CheckMapsVerifyResult(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// failedLog is the run's own log every failed result names, which the reason must quote.
	const failedLog = "/verify/verify-4.log"

	cases := []struct {
		name    string
		command string
		site    string
		result  verifytree.Result
		// logBody, when set, is written to a temp file whose path becomes the result's Log.
		logBody string
		// wantReasonExact, when set, is the whole reason for the log path the result names.
		wantReasonExact func(logPath string) string
		wantReasonHas   []string
		wantErrHas      string
		// staleRecord, when set, leaves a landing failure record from an earlier failed call that the verdict must remove.
		staleRecord bool
		// wantCrashed is the crash flag the check returns beside its reason.
		wantCrashed bool
	}{
		{name: "pass", command: "go test ./...", site: "Publish", result: verifytree.Result{Status: verifytree.StatusPassed}},
		{name: "site label follows the producer", command: "true", site: "Finalize", result: verifytree.Result{Status: verifytree.StatusPassed}},
		{name: "skipped proceeds", command: "true", site: "Finalize", result: verifytree.Result{Status: verifytree.StatusSkipped, Tree: "abc"}},
		{name: "skipped over a recorded pass removes a present failure record", command: "true", site: "Finalize", result: verifytree.Result{Status: verifytree.StatusSkipped, Tree: "abc"}, staleRecord: true},
		{
			name: "failed", command: "false", site: "Publish",
			result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3, Log: failedLog},
			wantReasonExact: func(logPath string) string {
				return `landing gate failed after merging parent branch "main" (exit code 3); output: ` + logPath +
					`; fix forward on the task branch, then resume`
			},
		},
		{
			name: "failed after a watchdog quit names the window and busy_quit_sec", command: "false", site: "Publish",
			result:        verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			logBody:       "ok  \texample.com/mod/b\t0.1s\n" + gateslot.WatchdogLine("/tmp/x/a.test", 42, 2*time.Minute) + "\nSIGQUIT: quit\n",
			wantReasonHas: []string{"exit code 1", "2m0s", "busy_quit_sec", `"lym config gate" from the prime`, "fix forward on the task branch, then resume"},
		},
		{
			name: "could not start", command: "true", site: "Publish",
			result:        verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "exec: sh not found", Log: failedLog},
			wantReasonHas: []string{"could not start", "exec: sh not found", `"main"`},
		},
		{
			name: "timed out", command: "true", site: "Publish",
			result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: "gate tier2 exceeded timeout_min 30m0s and was killed", Log: failedLog},
			wantReasonExact: func(logPath string) string {
				return `landing gate timed out after merging parent branch "main": gate tier2 exceeded timeout_min 30m0s and was killed; output: ` + logPath +
					`; fix the hanging test on the task branch, or, for a tier that is slow but healthy, rewrite the tier whole from the prime with ` +
					"`lym config gate --set 'tiers.tier<N>={...}'`" +
					`, carrying every key of the tier and the raised timeout_min, since a dotted key under the tier writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it; then resume`
			},
		},
		{
			name: "build timed out names the build key and the hanging build", command: "true", site: "Publish",
			result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: "gate build exceeded timeout_min 5m0s and was killed", Log: failedLog},
			wantReasonExact: func(logPath string) string {
				return `landing gate timed out after merging parent branch "main": gate build exceeded timeout_min 5m0s and was killed; output: ` + logPath +
					`; fix the hanging build, such as a stalled restore or compile, on the task branch, or, for a build that is slow but healthy, rewrite the build whole from the prime with ` +
					"`lym config gate --set 'build={...}'`" +
					`, carrying every key of the build and the raised timeout_min, since a dotted key under the build writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it; then resume`
			},
		},
		{
			name: "runner crash is a halt reason naming the crashed runs and no fixer", command: "true", site: "Finalize",
			result:        verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 134, RunnerCrash: true, Detail: "gate test runner crashed with no failing test: tier1 (exit 134, SIGABRT)", Log: failedLog},
			wantReasonHas: []string{LandingStopPrefix, "test runner crashed", "tier1 (exit 134, SIGABRT)", "no fixer runs", "`lym loom resume`"},
			wantCrashed:   true,
		},
		{
			name: "dirty result names the paths", command: "true", site: "Publish",
			result:        verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"a.txt", "b.txt"}},
			wantReasonHas: []string{"a.txt", "b.txt"},
		},
		{name: "unknown status is an error", command: "true", site: "Publish", result: verifytree.Result{Status: "bogus"}, wantErrHas: "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.result
			if tc.logBody != "" {
				result.Log = filepath.Join(t.TempDir(), "verify.log")
				if err := os.WriteFile(result.Log, []byte(tc.logBody), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			hold := verifytree.NewPauseHold(pauserecord.Geometry{}, nil)
			f := newGateFixture(t, "finalize", tc.command, nil)
			f.gate.hold = hold
			f.fake.result = result
			if tc.staleRecord {
				f.gate.recorder = &failureRecorder{}
				if err := os.MkdirAll(f.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := verifytree.WriteLandingFailure(f.paths, verifytree.LandingFailure{Kind: verifytree.FailureKindLanding}); err != nil {
					t.Fatal(err)
				}
			}
			reason, crashed, err := f.gate.check(context.Background(), tc.site, "main")
			if crashed != tc.wantCrashed {
				t.Errorf("check() crashed = %v; want %v", crashed, tc.wantCrashed)
			}
			if tc.staleRecord {
				if _, found, readErr := verifytree.ReadLandingFailure(f.paths); readErr != nil || found {
					t.Errorf("ReadLandingFailure() = (_, %v, %v); want the stale record removed", found, readErr)
				}
			}

			if f.fake.verifyCalls != 1 || f.fake.command != tc.command || f.fake.paths != f.paths || f.fake.site != (verifytree.Site{Label: tc.site, Hold: hold}) {
				t.Errorf("verify got calls=%d command=%q paths=%+v site=%+v", f.fake.verifyCalls, f.fake.command, f.fake.paths, f.fake.site)
			}
			if !slices.Equal(f.closureKeys, []string{"finalize"}) {
				t.Errorf("command closure keys = %v; want [finalize], the key the gate was built for", f.closureKeys)
			}

			if tc.wantErrHas != "" {
				if reason != "" || err == nil || !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Fatalf("got (%q, %v), want an error naming %q", reason, err, tc.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantReasonExact != nil {
				if want := tc.wantReasonExact(result.Log); reason != want {
					t.Fatalf("reason = %q, want %q", reason, want)
				}
				return
			}
			if len(tc.wantReasonHas) == 0 && reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			for _, want := range tc.wantReasonHas {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if result.Status == verifytree.StatusFailed && tc.wantReasonHas != nil && !strings.Contains(reason, result.Log) {
				t.Errorf("reason %q lacks the log path %q", reason, result.Log)
			}
		})
	}
}

// TestVerifyGate_RerunsATestFailureOnce pins the rerun of a gate told onFlakyPass before it judges a failure.
// It pins which failures rerun, the command the rerun runs and the flaky failures reported.
// It also pins the log each outcome of the rerun is judged on, and that an error of the rerun is the gate's error.
func TestVerifyGate_RerunsATestFailureOnce(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	report := func(lines ...string) string {
		return gateslot.FailureReportHeader + "\n" + strings.Join(lines, "\n") + "\n" + gateslot.FailedRunsPrefix + "tier0 (exit 1)\n"
	}
	goFailure := report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/a TestA:", "    a_test.go:3: got 1; want 2")
	dotnetFailure := report("tier0 (exit 1), log /runs/a.log:", "  Ns.Tests.EnumeratesTree:", "    System.UnauthorizedAccessException : denied")
	buildFailure := report("build (exit 2), log /runs/b.log:", "  its output:", "    main.go:1: syntax error")
	outsideAnyTest := report("tier0 (exit 1), log /runs/a.log:", "  example.com/m/b"+gateslot.ReportOutsideAnyTestSuffix+":", "    b.go:3: undefined: x")
	goRerun, err := gateslot.FailedTestsCall(runtime.GOOS, []gateslot.FailedTest{{Tier: 0, Package: "example.com/m/a", Test: "TestA"}})
	if err != nil {
		t.Fatal(err)
	}
	failed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	passed := verifytree.Result{Status: verifytree.StatusPassed}
	timedOut := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: "gate tier0 exceeded timeout_min 30m0s and was killed"}
	crashed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 134, RunnerCrash: true, Detail: "tier0 (exit 134, SIGABRT)"}
	cannotStart := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "exec: sh not found"}
	skipped := verifytree.Result{Status: verifytree.StatusSkipped}

	tests := []struct {
		name string
		// log is every verify call's log; empty leaves the failed result with no log.
		log string
		// results are the verify calls' results in order.
		results []verifytree.Result
		// noSink builds the gate without onFlakyPass.
		noSink bool
		// unreadableLog puts a directory where each verify call's log belongs, so no log can be read.
		unreadableLog bool
		// rerunErr is the error the second verify call returns.
		rerunErr     error
		wantCommands []string
		wantFlaky    []string
		// wantReasonLog is the 1-based verify call whose log the reason names, 0 for a pass and -1 for a failure with no log.
		wantReasonLog int
		wantCrashed   bool
	}{
		{name: "a passing verify runs no rerun", log: goFailure, results: []verifytree.Result{passed}, wantCommands: []string{gateLandingCall}},
		{
			name: "a Go test failure reruns its tests alone, and a pass there is flaky and passes the gate", log: goFailure,
			results: []verifytree.Result{failed, passed}, wantCommands: []string{gateLandingCall, goRerun}, wantFlaky: []string{"tier0 example.com/m/a TestA"},
		},
		{
			name: "a failure naming no Go test reruns the whole call", log: dotnetFailure,
			results: []verifytree.Result{failed, passed}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantFlaky: []string{"tier0 Ns.Tests.EnumeratesTree"},
		},
		{
			name: "a failure the rerun repeats fails the gate on the rerun's log", log: dotnetFailure,
			results: []verifytree.Result{failed, failed}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantReasonLog: 2,
		},
		{name: "a build failure is not rerun", log: buildFailure, results: []verifytree.Result{failed}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{name: "a timed-out failure is not rerun", log: goFailure, results: []verifytree.Result{timedOut}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{name: "a gate told no flaky sink runs no rerun", log: goFailure, results: []verifytree.Result{failed}, noSink: true, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{name: "a runner crash is not rerun", log: goFailure, results: []verifytree.Result{crashed}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1, wantCrashed: true},
		{name: "a shell that could not start is not rerun", log: goFailure, results: []verifytree.Result{cannotStart}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{name: "a failure with no log is not rerun", results: []verifytree.Result{failed}, wantCommands: []string{gateLandingCall}, wantReasonLog: -1},
		{name: "a failure whose log cannot be read is not rerun", unreadableLog: true, results: []verifytree.Result{failed}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{name: "a log with no failure report is not rerun", log: "FAIL\n", results: []verifytree.Result{failed}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1},
		{
			name: "a package that failed outside any test is not rerun", log: outsideAnyTest,
			results: []verifytree.Result{failed}, wantCommands: []string{gateLandingCall}, wantReasonLog: 1,
		},
		{
			name: "a rerun that times out fails the gate on the rerun's log", log: dotnetFailure,
			results: []verifytree.Result{failed, timedOut}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantReasonLog: 2,
		},
		{
			name: "a rerun whose runner crashes halts on the rerun's log", log: dotnetFailure,
			results: []verifytree.Result{failed, crashed}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantReasonLog: 2, wantCrashed: true,
		},
		{
			name: "a rerun whose shell could not start fails the gate on the rerun's log", log: dotnetFailure,
			results: []verifytree.Result{failed, cannotStart}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantReasonLog: 2,
		},
		{
			name: "a rerun the verify cannot run is the gate's error", log: dotnetFailure,
			results: []verifytree.Result{failed}, rerunErr: errors.New("verify record unreadable"), wantCommands: []string{gateLandingCall, gateLandingCall},
		},
		{
			name: "a rerun skipped over a recorded pass is flaky and passes the gate", log: dotnetFailure,
			results: []verifytree.Result{failed, skipped}, wantCommands: []string{gateLandingCall, gateLandingCall}, wantFlaky: []string{"tier0 Ns.Tests.EnumeratesTree"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newGateFixture(t, "finalize", gateLandingCall, nil)
			if tt.log != "" {
				writeVerifyLog(t, f, tt.log)
			}
			if tt.unreadableLog {
				f.fake.writeLog = func(path string) {
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Error(err)
					}
				}
			}
			f.fake.onVerify = func() {
				f.fake.result = tt.results[min(f.fake.verifyCalls-1, len(tt.results)-1)]
				if f.fake.verifyCalls == 2 {
					f.fake.verifyErr = tt.rerunErr
				}
			}
			var flaky []flakyPass
			if !tt.noSink {
				f.gate.onFlakyPass = func(pass flakyPass) { flaky = append(flaky, pass) }
			}

			reason, crashed, err := f.gate.check(context.Background(), "Finalize", "main")

			if !errors.Is(err, tt.rerunErr) {
				t.Fatalf("check() error = %v; want %v", err, tt.rerunErr)
			}
			if crashed != tt.wantCrashed {
				t.Errorf("check() crashed = %v; want %v", crashed, tt.wantCrashed)
			}
			if !slices.Equal(f.fake.commands, tt.wantCommands) {
				t.Errorf("verify commands = %q; want %q", f.fake.commands, tt.wantCommands)
			}
			logOf := func(call int) string { return filepath.Join(f.paths.Dir, fmt.Sprintf("verify-%d.log", call)) }
			switch {
			case tt.wantReasonLog == 0 && reason != "":
				t.Errorf("reason = %q; want a pass", reason)
			case tt.wantReasonLog < 0 && reason == "":
				t.Error("reason is empty; want the gate's failure")
			case tt.wantReasonLog > 0 && !strings.Contains(reason, logOf(tt.wantReasonLog)):
				t.Errorf("reason = %q; want it to name the log %q", reason, logOf(tt.wantReasonLog))
			}
			var gotFlaky []string
			for _, pass := range flaky {
				gotFlaky = append(gotFlaky, pass.failures...)
				if pass.log != logOf(1) {
					t.Errorf("flaky pass log = %q; want the first run's %q", pass.log, logOf(1))
				}
			}
			if !slices.Equal(gotFlaky, tt.wantFlaky) {
				t.Errorf("flaky failures = %q; want %q", gotFlaky, tt.wantFlaky)
			}
		})
	}
}

// TestVerifyGate_CheckWithoutCommandSkipsVerify pins that a gate with no command proceeds without calling verify.
// It also pins that the gate warns once when the command closure answers empty.
// With no closure wired, the gate stays silent.
//
// It stays serial because logcapture swaps the process-global logger.
func TestVerifyGate_CheckWithoutCommandSkipsVerify(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []struct {
		name    string
		command string
		noGate  bool
		wantLog bool
	}{
		{"empty command warns", "", false, true},
		{"nil command closure is silent", "true", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			f := newGateFixture(t, "finalize", tc.command, nil)
			if tc.noGate {
				f.gate.command = nil
			}
			reason, _, err := f.gate.check(context.Background(), "Publish", "main")
			if reason != "" || err != nil {
				t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
			}
			if f.fake.verifyCalls != 0 {
				t.Fatal("verify called")
			}
			logged := buf.String()
			if !tc.wantLog {
				if logged != "" {
					t.Fatalf("unexpected log output: %q", logged)
				}
				return
			}
			if !strings.Contains(logged, "WARN") || !strings.Contains(logged, "no verify command is configured") ||
				!strings.Contains(logged, "Publish") || !strings.Contains(logged, "main") {
				t.Fatalf("log = %q", logged)
			}
		})
	}
}

func TestVerifyGate_ClosureError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	f := newGateFixture(t, "finalize", "", errors.New("plan unreadable"))
	reason, _, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err == nil || !strings.Contains(err.Error(), "plan unreadable") || !strings.Contains(err.Error(), "Publish") {
		t.Fatalf("got (%q, %v)", reason, err)
	}
	if f.fake.verifyCalls != 0 {
		t.Fatal("verify called")
	}
}

func TestVerifyGate_Cancellation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	f := newGateFixture(t, "finalize", "true", nil)
	ctx, cancel := context.WithCancel(context.Background())
	f.fake.onVerify = cancel
	f.fake.verifyErr = context.Canceled
	reason, _, err := f.gate.check(ctx, "Publish", "main")
	if reason != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%q, %v), want a context.Canceled error", reason, err)
	}
}

// TestVerifyGate_Clean pins the clean-tree check.
// A clean tree gives no reason, and a dirty one names the point and the paths (capped).
// A status failure is an error naming the producer.
// The check also runs with no verify command and with a zero-value gate.
func TestVerifyGate_Clean(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	manyPaths := make([]string, dirtyPathsShown+3)
	for i := range manyPaths {
		manyPaths[i] = filepath.Join("dir", string(rune('a'+i))+".txt")
	}
	cases := []struct {
		name       string
		producer   string
		point      string
		setup      func(f *gateFixture)
		zeroGate   bool
		wantReason []string
		wantNoPath string
		wantErrHas []string
	}{
		{name: "clean tree", producer: "Publish", point: "before the merge-in", setup: func(*gateFixture) {}},
		{
			name: "dirty names the point and paths", producer: "Publish", point: "after the merge-in",
			setup:      func(f *gateFixture) { f.fake.dirtyAt(0, "x.go", "y.go") },
			wantReason: []string{"after the merge-in", "x.go", "y.go", "commit or remove"},
		},
		{
			name: "dirty paths are capped", producer: "Finalize", point: "after the verify",
			setup:      func(f *gateFixture) { f.fake.dirtyAt(0, manyPaths...) },
			wantReason: []string{"and 3 more"}, wantNoPath: manyPaths[len(manyPaths)-1],
		},
		{
			name: "status error names the producer", producer: "Publish", point: "before the merge-in",
			setup:      func(f *gateFixture) { f.fake.dirtyErr = errors.New("git exploded") },
			wantErrHas: []string{"git exploded", "Publish"},
		},
		{
			name: "runs without a verify command", producer: "Publish", point: "before the merge-in",
			setup:      func(f *gateFixture) { f.gate.command = nil; f.fake.dirtyAt(0, "x.go") },
			wantReason: []string{"x.go"},
		},
		{name: "zero-value gate skips the check", producer: "Publish", point: "before the merge-in", zeroGate: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newGateFixture(t, "finalize", "true", nil)
			if tc.setup != nil {
				tc.setup(f)
			}
			gate := f.gate
			if tc.zeroGate {
				gate = verifyGate{}
			}
			reason, err := gate.clean(tc.producer, tc.point)

			if len(tc.wantErrHas) > 0 {
				if reason != "" || err == nil {
					t.Fatalf("got (%q, %v), want an error", reason, err)
				}
				for _, want := range tc.wantErrHas {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tc.wantReason) == 0 && reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			for _, want := range tc.wantReason {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if tc.wantNoPath != "" && strings.Contains(reason, tc.wantNoPath) {
				t.Errorf("reason %q names %q; want it cut by the cap", reason, tc.wantNoPath)
			}
		})
	}
}
