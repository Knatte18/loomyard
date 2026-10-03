// publish_verify_test.go covers the clean-tree checks and the post-merge verify gate as Publish.Call wires them around the parent merge-in and before the push,
// against fake verifytree seams and the package's fake resolver.

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

func TestPublishVerify_Pass(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
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
}

// TestPublishVerify_VerifiedTreeSkipsAndProceeds pins that a verify the record skips still lets the push run.
func TestPublishVerify_VerifiedTreeSkipsAndProceeds(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", true)
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusSkipped}
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if !fx.pushed {
		t.Error("push did not run")
	}
}

// TestPublishVerify_VerifyRunsOnEveryMergeIn pins that the producer asks verify after a no-op merge-in too,
// leaving the skip decision to the verified-tree record.
func TestPublishVerify_VerifyRunsOnEveryMergeIn(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", true)
	if _, _, err := fx.call(t); err != nil {
		t.Fatal(err)
	}
	if fx.gate.fake.verifyCalls != 1 {
		t.Errorf("verify calls = %d; want 1", fx.gate.fake.verifyCalls)
	}
}

func TestPublishVerify_Fail(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3}
	failOnGitHubClient(t)

	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	for _, want := range []string{`"main"`, "exit code 3", fx.gate.paths.Log} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
	if fx.pushed {
		t.Error("push ran after a failed verify")
	}
	if fx.gate.fake.dirtyCalls != 2 {
		t.Errorf("clean-tree checks = %d; want 2, the check after the verify never running", fx.gate.fake.dirtyCalls)
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

func TestPublishVerify_DirtyResultFromVerifyHalts(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"late.txt"}}
	failOnGitHubClient(t)
	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck || !strings.Contains(reason, "late.txt") {
		t.Fatalf("Call() = %q, %q, %v; want Stuck naming late.txt", outcome, reason, err)
	}
	if fx.pushed {
		t.Error("push ran on a dirty tree")
	}
}

func TestPublishVerify_CleanCheckErrorIsNotStuck(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.dirtyErr = errors.New("git exploded")
	outcome, _, err := fx.call(t)
	if err == nil || outcome == shedengine.Stuck || !strings.Contains(err.Error(), "git exploded") {
		t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
	}
	if fx.pushed || fx.res.called {
		t.Errorf("pushed=%v resolver called=%v; want neither", fx.pushed, fx.res.called)
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

func TestPublishVerify_ClosureError(t *testing.T) {
	fx := newPublishVerifyFixture(t, "", false)
	fx.p.gate.command = func() (string, error) { return "", errors.New("plan unreadable") }
	outcome, _, err := fx.call(t)
	if err == nil || outcome == shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
	}
	if fx.pushed {
		t.Error("push ran after a closure error")
	}
}

func TestPublishVerify_CouldNotStart(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "no shell"}
	failOnGitHubClient(t)
	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	if !strings.Contains(reason, "no shell") {
		t.Errorf("reason %q lacks the spawn error", reason)
	}
	if fx.pushed {
		t.Error("push ran after a spawn failure")
	}
}

func TestPublishVerify_Cancelled(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	ctx, cancel := context.WithCancel(context.Background())
	fx.gate.fake.onVerify = cancel
	fx.gate.fake.verifyErr = context.Canceled
	outcome, _, err := fx.p.Call(ctx)
	if err == nil || outcome == shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
	}
	if fx.pushed {
		t.Error("push ran after a cancellation")
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
