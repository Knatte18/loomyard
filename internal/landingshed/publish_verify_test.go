// publish_verify_test.go covers the push-time landing and publish gate calls and the clean-tree check Publish.Call wires before the push, against fake verifytree seams.
// The merge-in, the first landing call and their checks belong to Merge-In and are covered in mergein_test.go.
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// publishVerifyFixture is a Publish over a real gate with fake verifytree seams and a push closure that records whether it ran.
type publishVerifyFixture struct {
	p      *Publish
	gate   *gateFixture
	pushed bool
	order  []string
}

// newPublishVerifyFixture builds the fixture with landingCommand as the `landing` gate's call and publishCommand as the `publish` gate's call.
// An empty publishCommand is the call an empty `publish` list renders, and every pass names gateBaseCall as its base command.
func newPublishVerifyFixture(t *testing.T, landingCommand, publishCommand string) *publishVerifyFixture {
	t.Helper()
	fx := &publishVerifyFixture{gate: newGateFixture(t, landingCommand, nil)}
	fx.gate.gate.publishCommand = func() (string, error) { return publishCommand, nil }
	fx.gate.gate.baseCommand = func() (string, error) { return gateBaseCall, nil }
	deps := newTestDeps(t)
	deps.PushBranch = func() error { fx.pushed = true; return nil }
	deps.GateTiers = fixedGateTiers
	deps.MergeRecordPath = MergeRecordPath(t.TempDir())
	writeSummary(t, deps.DescriptionPath, "Title", "Body")
	fx.p = &Publish{deps: deps, gate: fx.gate.gate}
	srv := newPublishGitHubServer(t, &fx.order)
	srv.install(t)
	return fx
}

func (fx *publishVerifyFixture) call(t *testing.T) (shedengine.Outcome, string, error) {
	t.Helper()
	outcome, ptr, err := fx.p.Call(context.Background())
	return outcome, ptr.Reason, err
}

// failOnGitHubClient installs a factory that fails the test if Publish reaches GitHub.
func failOnGitHubClient(t *testing.T) {
	t.Helper()
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) {
		t.Error("NewGitHubClient was called; want no GitHub access after a halted gate")
		return nil, errors.New("must not be called")
	}
	t.Cleanup(func() { NewGitHubClient = orig })
}

// TestPublishVerify_Proceeds pins that Publish runs the landing call and then the publish call, both naming the base command, and that a passing run and one the verified-tree record skips each let the push run.
// An empty publish call runs only the landing call.
// A skip seam answering true takes the path of an empty `publish` list and removes a present Publish failure record, one answering false or absent runs the publish call, and the seam is asked afresh at each Call.
// A darn tree-scoped diff whose command is empty runs nothing and passes.
// The clean-tree check after the calls is made.
func TestPublishVerify_Proceeds(t *testing.T) {
	answer := func(skip bool) func() (bool, error) {
		return func() (bool, error) { return skip, nil }
	}
	tests := []struct {
		name           string
		landingCommand string
		publishCommand string
		result         verifytree.Result
		// seam is the told skip seam; nil leaves it unwired.
		seam func() (bool, error)
		// withRecord writes a Publish failure record before the call and expects none after it.
		withRecord   bool
		wantCommands []string
	}{
		{"an empty publish call runs only the landing call", gateLandingCall, "", verifytree.Result{Status: verifytree.StatusPassed}, nil, false, []string{gateLandingCall}},
		{"pass", gateLandingCall, gatePublishCall, verifytree.Result{Status: verifytree.StatusPassed}, nil, false, []string{gateLandingCall, gatePublishCall}},
		{"verified tree skips", gateLandingCall, gatePublishCall, verifytree.Result{Status: verifytree.StatusSkipped}, nil, false, []string{gateLandingCall, gatePublishCall}},
		{"a seam answering true runs only the landing call and clears the record", gateLandingCall, gatePublishCall, verifytree.Result{Status: verifytree.StatusPassed}, answer(true), true, []string{gateLandingCall}},
		{"a seam answering false runs the publish call", gateLandingCall, gatePublishCall, verifytree.Result{Status: verifytree.StatusPassed}, answer(false), false, []string{gateLandingCall, gatePublishCall}},
		{"an empty tree-scoped command with the publish gate skipped runs nothing and clears the record", "", gatePublishCall, verifytree.Result{Status: verifytree.StatusPassed}, answer(true), true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, tt.landingCommand, tt.publishCommand)
			fx.gate.fake.result = tt.result
			fx.p.deps.PublishSkip = tt.seam
			if tt.withRecord {
				fx.p.gate.recorder = &failureRecorder{}
				if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := verifytree.WritePublishFailure(fx.gate.paths, verifytree.PublishFailure{Kind: verifytree.FailureKindLanding}); err != nil {
					t.Fatal(err)
				}
			}
			outcome, _, err := fx.call(t)
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if !fx.pushed {
				t.Error("push did not run")
			}
			if !reflect.DeepEqual(fx.gate.fake.commands, tt.wantCommands) {
				t.Errorf("verified commands = %q; want %q", fx.gate.fake.commands, tt.wantCommands)
			}
			for i, got := range fx.gate.fake.sites {
				if want := (verifytree.Site{Label: "Publish", BaseCommand: gateBaseCall}); got != want {
					t.Errorf("site of call %d = %+v; want %+v", i+1, got, want)
				}
			}
			if fx.gate.fake.dirtyCalls != 1 {
				t.Errorf("clean-tree checks = %d; want 1", fx.gate.fake.dirtyCalls)
			}
			if tt.withRecord {
				if got, ok, _ := verifytree.ReadPublishFailure(fx.gate.paths); ok {
					t.Errorf("record = %+v; want none after a skipped publish gate", got)
				}
			}
		})
	}

	t.Run("a seam is asked afresh at each Call", func(t *testing.T) {
		fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
		fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusPassed}
		answers := []bool{true, false}
		fx.p.deps.PublishSkip = func() (bool, error) {
			skip := answers[0]
			answers = answers[1:]
			return skip, nil
		}
		for call, wantVerify := range []int{1, 3} {
			if outcome, _, err := fx.call(t); err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() #%d = %q, %v; want Done, nil", call+1, outcome, err)
			}
			if fx.gate.fake.verifyCalls != wantVerify {
				t.Errorf("verify calls after Call #%d = %d; want %d", call+1, fx.gate.fake.verifyCalls, wantVerify)
			}
		}
	})
}

// TestPublishVerify_HaltsOnVerifyResult pins that a failed landing or publish call, one that could not start and one that reports a dirty tree each end Stuck with a reason naming the cause and the failing gate, before anything is pushed and before GitHub is reached.
func TestPublishVerify_HaltsOnVerifyResult(t *testing.T) {
	tests := []struct {
		name string
		// command is the call the result applies to.
		command    string
		result     verifytree.Result
		wantInReas []string
	}{
		{"failed publish call", gatePublishCall, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 4, Log: "/verify/verify-2.log"}, []string{"publish gate", `"main"`, "exit code 4", "/verify/verify-2.log"}},
		{"failed landing call", gateLandingCall, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3, Log: "/verify/verify-1.log"}, []string{"landing gate", `"main"`, "exit code 3", "/verify/verify-1.log"}},
		{"dirty result", gatePublishCall, verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"late.txt"}}, []string{"late.txt"}},
		{"could not start", gatePublishCall, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "no shell"}, []string{"no shell"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
			fx.gate.fake.resultByCommand = map[string]verifytree.Result{tt.command: tt.result}
			failOnGitHubClient(t)

			outcome, reason, err := fx.call(t)
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			for _, want := range tt.wantInReas {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if fx.pushed {
				t.Error("push ran after a halted verify")
			}
			if fx.gate.fake.dirtyCalls != 0 {
				t.Errorf("clean-tree checks = %d; want 0, the check after the verify never running", fx.gate.fake.dirtyCalls)
			}
		})
	}
}

// TestPublishVerify_DirtyTreeHalts pins that a tree passing landing and publish calls left dirty ends Stuck naming the paths, before anything is pushed and before GitHub is reached.
func TestPublishVerify_DirtyTreeHalts(t *testing.T) {
	fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
	fx.gate.fake.dirtyAt(0, "stray.txt", "gen/out.go")
	failOnGitHubClient(t)

	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	for _, want := range []string{"after the verify", "stray.txt", "gen/out.go"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
	if fx.pushed {
		t.Error("PushBranch ran on a dirty tree")
	}
	if fx.gate.fake.verifyCalls != 2 {
		t.Errorf("verify calls = %d; want 2, the landing and the publish call", fx.gate.fake.verifyCalls)
	}
}

// TestPublishVerify_HardErrorsAreReturned pins that a failing clean-tree check, an unreadable landing or publish command, a failing skip seam and a cancellation are returned as errors rather than Stuck verdicts, with nothing pushed.
func TestPublishVerify_HardErrorsAreReturned(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fx *publishVerifyFixture) context.Context
		// wantErrHas is a substring of the returned error, when one is required.
		wantErrHas string
	}{
		{
			name: "clean-tree check error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.gate.fake.dirtyErr = errors.New("git exploded")
				return context.Background()
			},
			wantErrHas: "git exploded",
		},
		{
			name: "landing command closure error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.p.gate.command = func() (string, error) { return "", errors.New("gate config refused") }
				return context.Background()
			},
			wantErrHas: "gate config refused",
		},
		{
			name: "publish command closure error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.p.gate.publishCommand = func() (string, error) { return "", errors.New("publish list refused") }
				return context.Background()
			},
			wantErrHas: "publish list refused",
		},
		{
			name: "skip seam error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.p.deps.PublishSkip = func() (bool, error) { return false, errors.New("tree scope unreadable") }
				return context.Background()
			},
			wantErrHas: "tree scope unreadable",
		},
		{
			name: "cancellation",
			setup: func(fx *publishVerifyFixture) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				fx.gate.fake.onVerify = cancel
				fx.gate.fake.verifyErr = context.Canceled
				return ctx
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
			ctx := tt.setup(fx)
			outcome, _, err := fx.p.Call(ctx)
			if err == nil || outcome == shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
			}
			if tt.wantErrHas != "" && !strings.Contains(err.Error(), tt.wantErrHas) {
				t.Errorf("error %q lacks %q", err, tt.wantErrHas)
			}
			if fx.pushed {
				t.Error("push ran after a returned error")
			}
		})
	}
}

func TestPublishVerify_NoPRRequiredRunsNoVerify(t *testing.T) {
	fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
	fx.p.deps.Config.RequirePRToBase = []string{"other-base"}
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if fx.gate.fake.verifyCalls != 0 || fx.gate.fake.dirtyCalls != 0 {
		t.Errorf("verify calls=%d clean-tree checks=%d; want 0 and 0", fx.gate.fake.verifyCalls, fx.gate.fake.dirtyCalls)
	}
}

// writeVerifyLog makes every passed or failed fake verify write log to its own log path and name it on its result, as a real verify run does.
func writeVerifyLog(t *testing.T, gate *gateFixture, log string) {
	t.Helper()
	gate.fake.writeLog = func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
			t.Error(err)
		}
	}
}

// TestPublishVerify_FailureRecord pins the Publish failure record.
// A failed landing or publish call writes it with the gate as its kind, the failing tier, tests, the failing run's own log, HEAD and the merge commit of Merge-In's merge record.
// The tier is the log's last failing tier summary line, else the highest tier of the failing gate's list.
// A dirty tree or a timeout writes none, and a passing Publish removes a present one before the push.
// It stays serial (no t.Parallel): failOnGitHubClient swaps the package-level NewGitHubClient, which is process-global state.
func TestPublishVerify_FailureRecord(t *testing.T) {
	const toldWayForward = `run "lyx darn goto", then "lyx darn resume", in the task worktree; the told route`
	const plainLog = "verify output"
	tierZeroLog := plainLog + "\n" + gateslot.TierSummaryLine(gateslot.RunRecord{Tier: 0, ExitCode: 1}, "/x/tier0.log") + "\n"
	failed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	wantTests := []verifytree.FailedTest{{Package: "example.com/m/a", Test: "TestA"}}
	tests := []struct {
		name    string
		results map[string]verifytree.Result
		// log is the failing run's log.
		log string
		// record is the merge record on disk; nil leaves none.
		record    *MergeRecord
		wantKind  verifytree.FailureKind
		wantTier  int
		wantMerge string
		// blockRecord pre-creates the record path as a directory, so the write fails.
		blockRecord bool
		// wantGoto expects the Stuck reason to name the goto to Webster-Burler-Review as the way forward.
		wantGoto bool
	}{
		{"the publish call fails with no tier line", map[string]verifytree.Result{gatePublishCall: failed}, plainLog, &MergeRecord{MergeCommit: "merged-head"}, verifytree.FailureKindPublish, 2, "merged-head", false, true},
		{"the landing call fails with no tier line", map[string]verifytree.Result{gateLandingCall: failed}, plainLog, &MergeRecord{MergeCommit: "merged-head"}, verifytree.FailureKindLanding, 1, "merged-head", false, true},
		{"the landing call fails on a tier line", map[string]verifytree.Result{gateLandingCall: failed}, tierZeroLog, &MergeRecord{MergeCommit: "merged-head"}, verifytree.FailureKindLanding, 0, "merged-head", false, true},
		{"an up-to-date merge names no merge commit", map[string]verifytree.Result{gatePublishCall: failed}, plainLog, &MergeRecord{UpToDate: true}, verifytree.FailureKindPublish, 2, "", false, true},
		{"no merge record names no merge commit", map[string]verifytree.Result{gatePublishCall: failed}, plainLog, nil, verifytree.FailureKindPublish, 2, "", false, true},
		{"dirty tree writes none", map[string]verifytree.Result{gatePublishCall: {Status: verifytree.StatusDirty, Dirty: []string{"a.txt"}}}, plainLog, nil, "", 0, "", false, false},
		{"timeout writes none", map[string]verifytree.Result{gatePublishCall: {Status: verifytree.StatusFailed, ExitCode: -1, TimedOut: true}}, plainLog, nil, "", 0, "", false, false},
		{"a failed record write names no goto", map[string]verifytree.Result{gatePublishCall: failed}, plainLog, nil, "", 0, "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, gateLandingCall, gatePublishCall)
			fx.p.deps.TaskHead = func() (string, error) { return "merged-head", nil }
			if tt.record != nil {
				if err := WriteMergeRecord(fx.p.deps.MergeRecordPath, *tt.record); err != nil {
					t.Fatal(err)
				}
			}
			fx.p.gate.failedWayForward = toldWayForward
			fx.p.gate.failureTarget = "Told-Row"
			fx.p.gate.recorder = &failureRecorder{
				failingTests: func(log string) []verifytree.FailedTest {
					if log != tt.log {
						t.Errorf("failingTests got log %q; want the verify log %q", log, tt.log)
					}
					return wantTests
				},
				head: fx.p.deps.TaskHead,
			}
			fx.gate.fake.resultByCommand = tt.results
			writeVerifyLog(t, fx.gate, tt.log)
			failOnGitHubClient(t)

			if tt.blockRecord {
				if err := os.MkdirAll(fx.gate.paths.PublishFailure, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			outcome, reason, err := fx.call(t)
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			if tt.wantGoto {
				if !strings.HasSuffix(reason, "way forward: "+toldWayForward) {
					t.Errorf("reason %q; want it to end with the told way-forward clause", reason)
				}
				if strings.Contains(reason, "fix forward on the task branch") {
					t.Errorf("reason %q; want no hand-fix clause beside the goto", reason)
				}
			} else if strings.Contains(reason, "goto") {
				t.Errorf("reason %q; want no goto", reason)
			}
			if tt.blockRecord {
				if info, err := os.Stat(fx.gate.paths.PublishFailure); err != nil || !info.IsDir() {
					t.Errorf("record path = %v, %v; want the blocking directory left, no record", info, err)
				}
				return
			}
			got, ok, err := verifytree.ReadPublishFailure(fx.gate.paths)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantKind == "" {
				if ok {
					t.Fatalf("record = %+v; want none", got)
				}
				return
			}
			// The failing verify is the last call, since Publish stops at it.
			failingLog := filepath.Join(fx.gate.paths.Dir, fmt.Sprintf("verify-%d.log", fx.gate.fake.verifyCalls))
			want := verifytree.PublishFailure{
				Kind: tt.wantKind, Tier: tt.wantTier, Row: "Publish", Tests: wantTests, LogPath: failingLog, Head: "merged-head", MergeCommit: tt.wantMerge,
				Target: "Told-Row", WrittenAt: gateMarkStart.UTC().Format(time.RFC3339),
			}
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("record = %+v, %v; want %+v", got, ok, want)
			}
		})
	}

	t.Run("a passing publish removes a present record", func(t *testing.T) {
		for _, publishCommand := range []string{"", gatePublishCall} {
			fx := newPublishVerifyFixture(t, gateLandingCall, publishCommand)
			fx.p.gate.recorder = &failureRecorder{}
			if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := verifytree.WritePublishFailure(fx.gate.paths, verifytree.PublishFailure{Kind: verifytree.FailureKindLanding}); err != nil {
				t.Fatal(err)
			}

			if outcome, _, err := fx.call(t); err != nil || outcome != shedengine.Done {
				t.Fatalf("publish call %q: Call() = %q, %v; want Done, nil", publishCommand, outcome, err)
			}
			if _, ok, _ := verifytree.ReadPublishFailure(fx.gate.paths); ok {
				t.Errorf("publish call %q: record still present after a passing Publish", publishCommand)
			}
		}
	})
}
