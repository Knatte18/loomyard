package landingshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const awaitingPRURL = "https://github.com/owner/repo/pull/7"

// publishOutcomeFor runs one Publish against a fake GitHub server with the given list and create
// bodies and returns its outcome and reason.
func publishOutcomeFor(t *testing.T, listBody, createBody string) (shedengine.Outcome, string) {
	t.Helper()
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	var order []string
	srv := newPublishGitHubServer(t, &order)
	srv.listBody = listBody
	if createBody != "" {
		srv.createBody = createBody
	}
	srv.install(t)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	return outcome, ptr.Reason
}

func TestPublish_FreshPR_ReturnsAwaitingWithURL(t *testing.T) {
	outcome, reason := publishOutcomeFor(t, "[]", `{"number":7,"state":"open","html_url":"`+awaitingPRURL+`"}`)
	if outcome != shedengine.Awaiting {
		t.Errorf("outcome = %q; want %q", outcome, shedengine.Awaiting)
	}
	if !strings.HasSuffix(reason, awaitingPRURL) {
		t.Errorf("reason = %q; want it to end with the PR URL", reason)
	}
}

func TestPublish_OpenUnapprovedPR_ReturnsAwaitingWithURL(t *testing.T) {
	outcome, reason := publishOutcomeFor(t, `[{"number":7,"state":"open","html_url":"`+awaitingPRURL+`"}]`, "")
	if outcome != shedengine.Awaiting {
		t.Errorf("outcome = %q; want %q", outcome, shedengine.Awaiting)
	}
	if !strings.HasSuffix(reason, awaitingPRURL) {
		t.Errorf("reason = %q; want it to end with the PR URL", reason)
	}
}

func TestPublish_ClosedUnmergedPR_StaysStuck(t *testing.T) {
	outcome, reason := publishOutcomeFor(t, `[{"number":7,"state":"closed","html_url":"`+awaitingPRURL+`"}]`, "")
	if outcome != shedengine.Stuck {
		t.Errorf("outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if reason == "" {
		t.Error("reason is empty; want the closed-unmerged reason")
	}
}

func TestPublish_AwaitingOrCancelled_CancelledContextIsError(t *testing.T) {
	p := &Publish{deps: newTestDeps(t)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, _, err := p.awaitingOrCancelled(ctx, "waiting")
	if err == nil {
		t.Fatalf("awaitingOrCancelled() = %q, nil; want the cancellation error", outcome)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v; want it to wrap context.Canceled", err)
	}
}
