// publish_verify_test.go covers the post-merge verify gate as Publish.Call wires it between the parent merge-in and the push,
// against a fake runner and the package's fake resolver.

package landingshed

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// publishVerifyFixture is a Publish over a real gate with a fake runner, a fake resolver and a push closure that records whether it ran.
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
		t.Error("NewGitHubClient was called; want no GitHub access after a failed verify")
		return nil, errors.New("must not be called")
	}
	t.Cleanup(func() { NewGitHubClient = orig })
}

func TestPublishVerify_TreeChangedPass(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Awaiting {
		t.Fatalf("Call() = %q, %v; want Awaiting, nil", outcome, err)
	}
	if !fx.pushed {
		t.Error("push did not run")
	}
	if fx.gate.markerExists(t) {
		t.Error("marker present after a passing verify")
	}
}

func TestPublishVerify_TreeChangedFail(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.code = 3
	fx.gate.fake.onRun = func() { _, _ = io.WriteString(fx.gate.fake.out, "FAIL line\n") }
	failOnGitHubClient(t)

	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	for _, want := range []string{`"main"`, "exit code 3", fx.gate.output} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
	if fx.pushed {
		t.Error("push ran after a failed verify")
	}
	if !fx.gate.markerExists(t) {
		t.Error("marker absent after a failed verify")
	}
	got, rerr := os.ReadFile(fx.gate.output)
	if rerr != nil || !strings.Contains(string(got), "FAIL line") {
		t.Errorf("output file = %q, %v; want the runner's line", got, rerr)
	}
}

func TestPublishVerify_UpToDateNoMarker(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", true)
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Awaiting {
		t.Fatalf("Call() = %q, %v; want Awaiting, nil", outcome, err)
	}
	if fx.gate.fake.calls != 0 {
		t.Errorf("runner called %d times; want 0", fx.gate.fake.calls)
	}
	if !fx.pushed {
		t.Error("push did not run")
	}
}

func TestPublishVerify_UpToDateWithMarker(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		fx := newPublishVerifyFixture(t, "go test ./...", true)
		fx.gate.seedMarker(t)
		outcome, _, err := fx.call(t)
		if err != nil || outcome != shedengine.Awaiting {
			t.Fatalf("Call() = %q, %v; want Awaiting, nil", outcome, err)
		}
		if fx.gate.fake.calls != 1 || !fx.pushed || fx.gate.markerExists(t) {
			t.Errorf("calls=%d pushed=%v marker=%v; want 1, true, false", fx.gate.fake.calls, fx.pushed, fx.gate.markerExists(t))
		}
	})
	t.Run("fail", func(t *testing.T) {
		fx := newPublishVerifyFixture(t, "go test ./...", true)
		fx.gate.seedMarker(t)
		fx.gate.fake.code = 1
		failOnGitHubClient(t)
		outcome, _, err := fx.call(t)
		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
		}
		if fx.pushed {
			t.Error("push ran after a failed verify")
		}
	})
}

func TestPublishVerify_ResolverStuck(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.res.result = mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "cannot resolve"}
	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck || reason != "cannot resolve" {
		t.Fatalf("Call() = %q, %q, %v; want Stuck with the resolver's reason", outcome, reason, err)
	}
	if fx.gate.fake.calls != 0 || fx.gate.markerExists(t) || fx.pushed {
		t.Errorf("calls=%d marker=%v pushed=%v; want none", fx.gate.fake.calls, fx.gate.markerExists(t), fx.pushed)
	}
}

func TestPublishVerify_EmptyCommand(t *testing.T) {
	t.Run("no marker", func(t *testing.T) {
		fx := newPublishVerifyFixture(t, "", false)
		buf := captureLogOutput(t)
		outcome, _, err := fx.call(t)
		if err != nil || outcome != shedengine.Awaiting {
			t.Fatalf("Call() = %q, %v; want Awaiting, nil", outcome, err)
		}
		if !fx.pushed || fx.gate.markerExists(t) || fx.gate.fake.calls != 0 {
			t.Errorf("pushed=%v marker=%v calls=%d; want true, false, 0", fx.pushed, fx.gate.markerExists(t), fx.gate.fake.calls)
		}
		if !strings.Contains(buf.String(), "WARN") {
			t.Errorf("log %q; want a WARN line", buf.String())
		}
	})
	t.Run("with marker", func(t *testing.T) {
		fx := newPublishVerifyFixture(t, "", true)
		fx.gate.seedMarker(t)
		buf := captureLogOutput(t)
		outcome, _, err := fx.call(t)
		if err != nil || outcome != shedengine.Awaiting {
			t.Fatalf("Call() = %q, %v; want Awaiting, nil", outcome, err)
		}
		if !fx.pushed || fx.gate.markerExists(t) || fx.gate.fake.calls != 0 {
			t.Errorf("pushed=%v marker=%v calls=%d; want true, false, 0", fx.pushed, fx.gate.markerExists(t), fx.gate.fake.calls)
		}
		logged := buf.String()
		if !strings.Contains(logged, "WARN") || !strings.Contains(logged, fx.gate.marker) {
			t.Errorf("log %q; want a WARN line naming the marker path", logged)
		}
	})
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

func TestPublishVerify_MarkerWriteFailure(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fx.p.gate.pendingPath = filepath.Join(blocker, "sub", "verify-pending")
	outcome, _, err := fx.call(t)
	if err == nil || outcome == shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
	}
	if fx.pushed || fx.gate.fake.calls != 0 {
		t.Errorf("pushed=%v calls=%d; want false, 0", fx.pushed, fx.gate.fake.calls)
	}
}

func TestPublishVerify_SpawnFailure(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.gate.fake.err = errors.New("no shell")
	failOnGitHubClient(t)
	outcome, reason, err := fx.call(t)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	if !strings.Contains(reason, "no shell") {
		t.Errorf("reason %q lacks the spawn error", reason)
	}
	if fx.pushed || !fx.gate.markerExists(t) {
		t.Errorf("pushed=%v marker=%v; want false, true", fx.pushed, fx.gate.markerExists(t))
	}
}

func TestPublishVerify_Cancelled(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	ctx, cancel := context.WithCancel(context.Background())
	fx.gate.fake.onRun = cancel
	fx.gate.fake.err = context.Canceled
	outcome, _, err := fx.p.Call(ctx)
	if err == nil || outcome == shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want a returned error", outcome, err)
	}
	if fx.pushed || !fx.gate.markerExists(t) {
		t.Errorf("pushed=%v marker=%v; want false, true", fx.pushed, fx.gate.markerExists(t))
	}
}

func TestPublishVerify_NoPRRequiredRunsNoVerify(t *testing.T) {
	fx := newPublishVerifyFixture(t, "go test ./...", false)
	fx.p.deps.Config.RequirePRToBase = []string{"other-base"}
	outcome, _, err := fx.call(t)
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if fx.gate.fake.calls != 0 {
		t.Errorf("runner called %d times; want 0", fx.gate.fake.calls)
	}
}

func TestPublishVerify_ApprovalPathRunsNoVerify(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"open","head":{"sha":"aaa"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	gate := newGateFixture(t, "go test ./...", nil)
	fx.p.gate = gate.gate
	outcome, _, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if gate.fake.calls != 0 {
		t.Errorf("runner called %d times; want 0", gate.fake.calls)
	}
}
