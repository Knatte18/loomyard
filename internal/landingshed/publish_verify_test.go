// publish_verify_test.go covers the clean-tree checks and the post-merge verify gate as Publish.Call wires them around the parent merge-in and before the push,
// against fake verifytree seams and the package's fake resolver.
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// The clean-tree checks Publish makes, by their zero-based position in a Call.
const (
	publishCleanBeforeMergeIn = iota
	publishCleanAfterMergeIn
	publishCleanAfterVerify
)

// publishVerifyFixture is a Publish over a real gate with fake verifytree seams, a fake resolver and a push closure that records whether it ran.
type publishVerifyFixture struct {
	p      *Publish
	gate   *gateFixture
	res    *recordingResolver
	pushed bool
	order  []string
}

func newPublishVerifyFixture(t *testing.T, command string, alreadyUpToDate bool) *publishVerifyFixture {
	t.Helper()
	fx := &publishVerifyFixture{gate: newGateFixture(t, command, nil)}
	deps := newTestDeps(t)
	deps.PushBranch = func() error { fx.pushed = true; return nil }
	fx.res = &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved, AlreadyUpToDate: alreadyUpToDate}}
	writeSummary(t, deps.DescriptionPath, "Title", "Body")
	fx.p = &Publish{deps: deps, resolver: fx.res, gate: fx.gate.gate}
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

// TestPublishVerify_Proceeds pins that a passing verify, one the verified-tree record skips and one after a no-op merge-in
// each let the push run, with the verify asked once at site Publish -- the producer asks after a no-op merge-in too,
// leaving the skip decision to the record -- and the three clean-tree checks made.
func TestPublishVerify_Proceeds(t *testing.T) {
	tests := []struct {
		name            string
		alreadyUpToDate bool
		result          verifytree.Result
	}{
		{"pass", false, verifytree.Result{Status: verifytree.StatusPassed}},
		{"verified tree skips", true, verifytree.Result{Status: verifytree.StatusSkipped}},
		{"no-op merge-in", true, verifytree.Result{Status: verifytree.StatusPassed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, "go test ./...", tt.alreadyUpToDate)
			fx.gate.fake.result = tt.result
			outcome, _, err := fx.call(t)
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if !fx.pushed {
				t.Error("push did not run")
			}
			if fx.gate.fake.verifyCalls != 1 || fx.gate.fake.site.Label != "Publish" {
				t.Errorf("verify calls=%d site=%+v; want 1 call at site Publish", fx.gate.fake.verifyCalls, fx.gate.fake.site)
			}
			if fx.gate.fake.dirtyCalls != 3 {
				t.Errorf("clean-tree checks = %d; want 3", fx.gate.fake.dirtyCalls)
			}
		})
	}
}

// TestPublishVerify_HaltsOnVerifyResult pins that a failed verify, a verify that could not start and a verify that
// reports a dirty tree each end Stuck with a reason naming the cause, before anything is pushed and before GitHub is reached.
func TestPublishVerify_HaltsOnVerifyResult(t *testing.T) {
	tests := []struct {
		name       string
		result     verifytree.Result
		wantInReas []string
	}{
		{"failed", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3}, []string{`"main"`, "exit code 3", "{Log}"}},
		{"dirty result", verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"late.txt"}}, []string{"late.txt"}},
		{"could not start", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "no shell"}, []string{"no shell"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, "go test ./...", false)
			fx.gate.fake.result = tt.result
			failOnGitHubClient(t)

			outcome, reason, err := fx.call(t)
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			for _, want := range tt.wantInReas {
				want = strings.ReplaceAll(want, "{Log}", fx.gate.paths.Log)
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if fx.pushed {
				t.Error("push ran after a halted verify")
			}
			if fx.gate.fake.dirtyCalls != 2 {
				t.Errorf("clean-tree checks = %d; want 2, the check after the verify never running", fx.gate.fake.dirtyCalls)
			}
		})
	}
}

// TestPublishVerify_DirtyTreeHalts pins that a dirty tree at each of the three points ends Stuck naming the paths,
// before anything is pushed and before GitHub is reached.
func TestPublishVerify_DirtyTreeHalts(t *testing.T) {
	cases := []struct {
		name       string
		point      int
		wantPoint  string
		wantResolv bool
		wantVerify int
	}{
		{"before the merge-in", publishCleanBeforeMergeIn, "before the merge-in", false, 0},
		{"after the merge-in", publishCleanAfterMergeIn, "after the merge-in", true, 0},
		{"after the verify", publishCleanAfterVerify, "after the verify", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newPublishVerifyFixture(t, "go test ./...", false)
			fx.gate.fake.dirtyAt(tc.point, "stray.txt", "gen/out.go")
			failOnGitHubClient(t)

			outcome, reason, err := fx.call(t)
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
			}
			for _, want := range []string{tc.wantPoint, "stray.txt", "gen/out.go"} {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if fx.pushed {
				t.Error("PushBranch ran on a dirty tree")
			}
			if fx.res.called != tc.wantResolv {
				t.Errorf("resolver called = %v; want %v", fx.res.called, tc.wantResolv)
			}
			if fx.gate.fake.verifyCalls != tc.wantVerify {
				t.Errorf("verify calls = %d; want %d", fx.gate.fake.verifyCalls, tc.wantVerify)
			}
		})
	}
}

func TestPublishVerify_ResolverStuck(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.res.result = mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "cannot resolve"}
	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck || reason != "cannot resolve" {
		t.Fatalf("Call() = %q, %q, %v; want Stuck with the resolver's reason", outcome, reason, err)
	}
	if fx.gate.fake.verifyCalls != 0 || fx.pushed {
		t.Errorf("verify calls=%d pushed=%v; want none", fx.gate.fake.verifyCalls, fx.pushed)
	}
}

func TestPublishVerify_EmptyCommand(t *testing.T) {
	fx := newPublishVerifyFixture(t, "", false)
	buf := logcapture.Capture(t)
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if !fx.pushed || fx.gate.fake.verifyCalls != 0 {
		t.Errorf("pushed=%v verify calls=%d; want true, 0", fx.pushed, fx.gate.fake.verifyCalls)
	}
	if !strings.Contains(buf.String(), "WARN") {
		t.Errorf("log %q; want a WARN line", buf.String())
	}
	if fx.gate.fake.dirtyCalls != 3 {
		t.Errorf("clean-tree checks = %d; want 3 even with no verify command", fx.gate.fake.dirtyCalls)
	}
}

// TestPublishVerify_HardErrorsAreReturned pins that a failing clean-tree check, an unreadable verify command and a cancellation
// are returned as errors rather than Stuck verdicts, with nothing pushed.
func TestPublishVerify_HardErrorsAreReturned(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fx *publishVerifyFixture) context.Context
		// wantErrHas is a substring of the returned error, when one is required.
		wantErrHas string
		// wantResolverIdle requires the resolver never to have been called.
		wantResolverIdle bool
	}{
		{
			name: "clean-tree check error",
			setup: func(fx *publishVerifyFixture) context.Context {
				fx.gate.fake.dirtyErr = errors.New("git exploded")
				return context.Background()
			},
			wantErrHas: "git exploded", wantResolverIdle: true,
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
			fx := newPublishVerifyFixture(t, "go test ./...", false)
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
			if tt.wantResolverIdle && fx.res.called {
				t.Error("resolver called; want the failing clean-tree check to stop first")
			}
		})
	}
}

func TestPublishVerify_NoPRRequiredRunsNoVerify(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.p.deps.Config.RequirePRToBase = []string{"other-base"}
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if fx.gate.fake.verifyCalls != 0 || fx.gate.fake.dirtyCalls != 0 {
		t.Errorf("verify calls=%d clean-tree checks=%d; want 0 and 0", fx.gate.fake.verifyCalls, fx.gate.fake.dirtyCalls)
	}
}
