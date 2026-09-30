// prgate_test.go covers PRGate against a fake GitHub server swapped in through NewGitHubClient,
// row by row through the decision table.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
)

const prGateHead = "aaaa111"

// prGateFixture is a PRGate over a fake server, with both record paths under a temp dir.
type prGateFixture struct {
	gate  *PRGate
	deps  Deps
	order []string
	srv   *publishGitHubServer
}

func newPRGateFixture(t *testing.T, listBody string) *prGateFixture {
	t.Helper()
	f := &prGateFixture{}
	deps := newTestDeps(t)
	dir := t.TempDir()
	deps.ApprovalPath = filepath.Join(dir, "approval.json")
	deps.RejectionPath = filepath.Join(dir, "rejection.json")
	deps.TaskHead = func() (string, error) { return prGateHead, nil }
	f.deps = deps
	f.srv = newPublishGitHubServer(t, &f.order)
	f.srv.listBody = listBody
	f.srv.install(t)
	return f
}

func (f *prGateFixture) call(t *testing.T) (shedengine.Outcome, shedengine.OutputPointer, error) {
	t.Helper()
	g, err := NewPRGate(f.deps)
	if err != nil {
		t.Fatalf("NewPRGate() error = %v", err)
	}
	return g.Call(context.Background())
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func openPR(number int, head string) string {
	return fmt.Sprintf(`[{"number":%d,"state":"open","html_url":"https://github.com/acme/proj/pull/%d","head":{"sha":%q}}]`, number, number, head)
}

func writeApprovalAt(t *testing.T, path string, number int, head string) {
	t.Helper()
	if err := WriteApproval(path, Approval{PRNumber: number, HeadSHA: head, ApprovedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("WriteApproval: %v", err)
	}
}

func writeRejectionAt(t *testing.T, path string, number int, head string) {
	t.Helper()
	if err := WriteRejection(path, Rejection{PRNumber: number, HeadSHA: head, RejectedAt: "2026-01-01T00:00:00Z", Findings: "fix it"}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
}

func wantVerdict(t *testing.T, f *prGateFixture, want shedengine.Outcome, reasonHas ...string) {
	t.Helper()
	outcome, ptr, err := f.call(t)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != want {
		t.Fatalf("Call() outcome = %q (%s); want %q", outcome, ptr.Reason, want)
	}
	for _, s := range reasonHas {
		if !strings.Contains(ptr.Reason, s) {
			t.Errorf("reason %q does not contain %q", ptr.Reason, s)
		}
	}
}

func TestNewPRGate_Refusals(t *testing.T) {
	ok := func() Deps {
		d := newTestDeps(t)
		d.ApprovalPath = "a"
		d.RejectionPath = "r"
		d.TaskHead = func() (string, error) { return "", nil }
		return d
	}
	cases := map[string]func(*Deps){
		"ApprovalPath":  func(d *Deps) { d.ApprovalPath = "" },
		"RejectionPath": func(d *Deps) { d.RejectionPath = "" },
		"TaskHead":      func(d *Deps) { d.TaskHead = nil },
	}
	for field, mutate := range cases {
		d := ok()
		mutate(&d)
		_, err := NewPRGate(d)
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("NewPRGate without %s: error = %v; want one naming it", field, err)
		}
	}
	if _, err := NewPRGate(ok()); err != nil {
		t.Errorf("NewPRGate(valid) error = %v", err)
	}
}

func TestPRGate_ParentNotRequired_DoneNoGitHubCall(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	f.deps.Config.RequirePRToBase = []string{"other"}
	wantVerdict(t, f, shedengine.Done)
	if len(f.order) != 0 {
		t.Errorf("GitHub calls = %v; want none", f.order)
	}
}

func TestPRGate_MergedOutranksAnyRecord(t *testing.T) {
	f := newPRGateFixture(t, `[{"number":3,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"sha":"zzz"}}]`)
	writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead)
	wantVerdict(t, f, shedengine.Done)
}

func TestPRGate_NoPR_Awaiting(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	wantVerdict(t, f, shedengine.Awaiting, "reopen", "abandon")
}

func TestPRGate_ClosedOutranksMatchingApproval(t *testing.T) {
	f := newPRGateFixture(t, fmt.Sprintf(`[{"number":3,"state":"closed","head":{"sha":%q}}]`, prGateHead))
	writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
	wantVerdict(t, f, shedengine.Awaiting, "closed", "abandon")
}

func TestPRGate_MalformedRecords_Awaiting(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	if err := writeFile(f.deps.ApprovalPath, "{not json"); err != nil {
		t.Fatal(err)
	}
	wantVerdict(t, f, shedengine.Awaiting, f.deps.ApprovalPath, "lyx loom approve")

	f = newPRGateFixture(t, openPR(3, prGateHead))
	if err := writeFile(f.deps.RejectionPath, "{}"); err != nil {
		t.Fatal(err)
	}
	wantVerdict(t, f, shedengine.Awaiting, f.deps.RejectionPath, "lyx loom reject")
}

func TestPRGate_MatchingApproval_Done(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
	wantVerdict(t, f, shedengine.Done)
}

func TestPRGate_MatchingRejection_Stuck(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead)
	wantVerdict(t, f, shedengine.Stuck, "#3", prGateHead, "rework")
}

func TestPRGate_MatchingApprovalOutranksStaleRejection(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
	writeRejectionAt(t, f.deps.RejectionPath, 3, "old")
	wantVerdict(t, f, shedengine.Done)
}

func TestPRGate_StaleRecords_AwaitingNamesMismatch(t *testing.T) {
	cases := []struct {
		name     string
		prHead   string
		prNumber int
		record   func(*testing.T, *prGateFixture)
		want     string
	}{
		{"stale approval, PR head moved", "bbb", 3, func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) }, "pull request head is now bbb"},
		{"stale rejection, PR head moved", "bbb", 3, func(t *testing.T, f *prGateFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead) }, "pull request head is now bbb"},
		{"local head moved", prGateHead, 3, func(t *testing.T, f *prGateFixture) {
			writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
			f.deps.TaskHead = func() (string, error) { return "ccc", nil }
		}, "local task head is now ccc"},
		{"PR number changed", prGateHead, 4, func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) }, "open pull request is now #4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newPRGateFixture(t, openPR(c.prNumber, c.prHead))
			c.record(t, f)
			wantVerdict(t, f, shedengine.Awaiting, c.want, "lyx loom approve", "lyx loom reject")
		})
	}
}

func TestPRGate_NoRecord_AwaitingEndsWithURL(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	_, ptr, err := f.call(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(ptr.Reason, "https://github.com/acme/proj/pull/3") {
		t.Errorf("reason %q does not end with the PR URL", ptr.Reason)
	}
	for _, s := range []string{"lyx loom approve", "lyx loom reject"} {
		if !strings.Contains(ptr.Reason, s) {
			t.Errorf("reason %q does not name %q", ptr.Reason, s)
		}
	}
}

func TestPRGate_TransientQueryFailure_ReturnedAsError(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	f.srv.listStatus = http.StatusServiceUnavailable
	outcome, _, err := f.call(t)
	if err == nil {
		t.Fatalf("Call() error = nil, outcome %q; want a transient error", outcome)
	}
	if shedtransient.Class(err) != shedengine.TransientGitHubAPI {
		t.Errorf("Class(err) = %q; want %q", shedtransient.Class(err), shedengine.TransientGitHubAPI)
	}
}

func TestPRGate_NonTransientQueryFailure_Awaiting(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	f.srv.listStatus = http.StatusNotFound
	wantVerdict(t, f, shedengine.Awaiting, "query existing pull request")
}

func TestPRGate_GitHubClientUnavailable_Awaiting(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	installFailingGitHubClientFactory(t, errors.New("no token"))
	wantVerdict(t, f, shedengine.Awaiting, "github client unavailable")
}

func TestPRGate_UnusableOrigin_Awaiting(t *testing.T) {
	f := newPRGateFixture(t, "[]")
	f.deps.OriginURL = "not a url"
	wantVerdict(t, f, shedengine.Awaiting, "origin URL unusable")
}

func TestPRGate_TaskHeadError_Returned(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
	f.deps.TaskHead = func() (string, error) { return "", errors.New("boom") }
	if _, _, err := f.call(t); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Call() error = %v; want the TaskHead error", err)
	}
}

func TestPRGate_CancelledContext_IsError(t *testing.T) {
	f := newPRGateFixture(t, openPR(3, prGateHead))
	g, err := NewPRGate(f.deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := g.Call(ctx); err == nil {
		t.Error("Call() under a cancelled context: error = nil; want cancellation error")
	}
}
