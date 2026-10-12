// publish_verify_test.go covers the push-time `publish_verify` and the clean-tree check Publish.Call wires before the push, against fake verifytree seams.
// The merge-in, the plan's verify and their checks belong to Merge-In and are covered in mergein_test.go.
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

// newPublishVerifyFixture builds the fixture with planCommand as the plan's verify command and publishCommand as landing config's `publish_verify`.
// An empty publishCommand leaves `publish_verify` unset.
func newPublishVerifyFixture(t *testing.T, planCommand, publishCommand string) *publishVerifyFixture {
	t.Helper()
	fx := &publishVerifyFixture{gate: newGateFixture(t, planCommand, nil)}
	deps := newTestDeps(t)
	deps.PushBranch = func() error { fx.pushed = true; return nil }
	deps.Config.PublishVerify = publishCommand
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

// TestPublishVerify_Proceeds pins that an unset `publish_verify` runs no verify, and that a passing one and one the verified-tree record skips each let the push run.
// The verify is asked once at site Publish with the plan's command as the base command, and the clean-tree check after it is made.
func TestPublishVerify_Proceeds(t *testing.T) {
	const planCommand, publishCommand = "go test ./...", "go test -tags tmux ./..."
	tests := []struct {
		name        string
		publishVerb string
		result      verifytree.Result
		wantVerify  int
	}{
		{"no publish_verify runs no verify", "", verifytree.Result{Status: verifytree.StatusPassed}, 0},
		{"pass", publishCommand, verifytree.Result{Status: verifytree.StatusPassed}, 1},
		{"verified tree skips", publishCommand, verifytree.Result{Status: verifytree.StatusSkipped}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, planCommand, tt.publishVerb)
			fx.gate.fake.result = tt.result
			outcome, _, err := fx.call(t)
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if !fx.pushed {
				t.Error("push did not run")
			}
			if fx.gate.fake.verifyCalls != tt.wantVerify {
				t.Errorf("verify calls = %d; want %d", fx.gate.fake.verifyCalls, tt.wantVerify)
			}
			if tt.wantVerify == 1 {
				if got, want := fx.gate.fake.site, (verifytree.Site{Label: "Publish", BaseCommand: planCommand}); got != want {
					t.Errorf("publish_verify site = %+v; want %+v", got, want)
				}
				if fx.gate.fake.command != publishCommand {
					t.Errorf("verified command = %q; want %q", fx.gate.fake.command, publishCommand)
				}
			}
			if fx.gate.fake.dirtyCalls != 1 {
				t.Errorf("clean-tree checks = %d; want 1", fx.gate.fake.dirtyCalls)
			}
		})
	}
}

// TestPublishVerify_HaltsOnVerifyResult pins that a failed `publish_verify`, one that could not start and one that reports a dirty tree each end Stuck with a reason naming the cause, before anything is pushed and before GitHub is reached.
func TestPublishVerify_HaltsOnVerifyResult(t *testing.T) {
	tests := []struct {
		name       string
		result     verifytree.Result
		wantInReas []string
	}{
		{"failed", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 4, Log: "/verify/verify-2.log"}, []string{"publish_verify", `"main"`, "exit code 4", "/verify/verify-2.log"}},
		{"dirty result", verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"late.txt"}}, []string{"late.txt"}},
		{"could not start", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "no shell"}, []string{"no shell"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, "go test ./...", "go test -tags tmux ./...")
			fx.gate.fake.result = tt.result
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

// TestPublishVerify_DirtyTreeHalts pins that a tree a passing `publish_verify` left dirty ends Stuck naming the paths, before anything is pushed and before GitHub is reached.
func TestPublishVerify_DirtyTreeHalts(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", "go test -tags tmux ./...")
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
	if fx.gate.fake.verifyCalls != 1 {
		t.Errorf("verify calls = %d; want 1", fx.gate.fake.verifyCalls)
	}
}

// TestPublishVerify_HardErrorsAreReturned pins that a failing clean-tree check, an unreadable plan command and a cancellation are returned as errors rather than Stuck verdicts, with nothing pushed.
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
			name: "verify command closure error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.p.gate.command = func() (string, error) { return "", errors.New("plan unreadable") }
				return context.Background()
			},
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
			fx := newPublishVerifyFixture(t, "go test ./...", "go test -tags tmux ./...")
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
	fx := newPublishVerifyFixture(t, "go test ./...", "go test -tags tmux ./...")
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
// A failed publish_verify writes it with its kind, tests, the failing run's own log, HEAD and the merge commit of Merge-In's merge record.
// A dirty tree or a timeout writes none, and a passing Publish removes a present one before the push.
// It stays serial (no t.Parallel): failOnGitHubClient swaps the package-level NewGitHubClient, which is process-global state.
func TestPublishVerify_FailureRecord(t *testing.T) {
	const planCommand, publishCommand = "go test ./...", "go test -tags tmux ./..."
	const toldWayForward = `run "lyx darn goto", then "lyx darn resume", in the task worktree; the told route`
	failed := verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	wantTests := []verifytree.FailedTest{{Package: "example.com/m/a", Test: "TestA"}}
	tests := []struct {
		name    string
		results map[string]verifytree.Result
		// record is the merge record on disk; nil leaves none.
		record    *MergeRecord
		wantKind  verifytree.FailureKind
		wantMerge string
		// blockRecord pre-creates the record path as a directory, so the write fails.
		blockRecord bool
		// wantGoto expects the Stuck reason to name the goto to Webster-Burler-Review as the way forward.
		wantGoto bool
	}{
		{"publish_verify fails", map[string]verifytree.Result{publishCommand: failed}, &MergeRecord{MergeCommit: "merged-head"}, verifytree.FailureKindPublishVerify, "merged-head", false, true},
		{"an up-to-date merge names no merge commit", map[string]verifytree.Result{publishCommand: failed}, &MergeRecord{UpToDate: true}, verifytree.FailureKindPublishVerify, "", false, true},
		{"no merge record names no merge commit", map[string]verifytree.Result{publishCommand: failed}, nil, verifytree.FailureKindPublishVerify, "", false, true},
		{"dirty tree writes none", map[string]verifytree.Result{publishCommand: {Status: verifytree.StatusDirty, Dirty: []string{"a.txt"}}}, nil, "", "", false, false},
		{"timeout writes none", map[string]verifytree.Result{publishCommand: {Status: verifytree.StatusFailed, ExitCode: -1, TimedOut: true}}, nil, "", "", false, false},
		{"a failed record write names no goto", map[string]verifytree.Result{publishCommand: failed}, nil, "", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, planCommand, publishCommand)
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
					if log != "verify output" {
						t.Errorf("failingTests got log %q; want the verify log", log)
					}
					return wantTests
				},
				head: fx.p.deps.TaskHead,
			}
			fx.gate.fake.resultByCommand = tt.results
			writeVerifyLog(t, fx.gate, "verify output")
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
				Kind: tt.wantKind, Row: "Publish", Tests: wantTests, LogPath: failingLog, Head: "merged-head", MergeCommit: tt.wantMerge,
				Target: "Told-Row", WrittenAt: gateMarkStart.UTC().Format(time.RFC3339),
			}
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("record = %+v, %v; want %+v", got, ok, want)
			}
		})
	}

	t.Run("a passing publish removes a present record", func(t *testing.T) {
		for _, publishVerb := range []string{"", publishCommand} {
			fx := newPublishVerifyFixture(t, planCommand, publishVerb)
			fx.p.gate.recorder = &failureRecorder{}
			if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := verifytree.WritePublishFailure(fx.gate.paths, verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify}); err != nil {
				t.Fatal(err)
			}

			if outcome, _, err := fx.call(t); err != nil || outcome != shedengine.Done {
				t.Fatalf("publish_verify %q: Call() = %q, %v; want Done, nil", publishVerb, outcome, err)
			}
			if _, ok, _ := verifytree.ReadPublishFailure(fx.gate.paths); ok {
				t.Errorf("publish_verify %q: record still present after a passing Publish", publishVerb)
			}
		}
	})
}
