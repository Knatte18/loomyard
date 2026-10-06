// prgate_test.go covers PRGate against a fake GitHub server swapped in through NewGitHubClient, row by row through the decision table.
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

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

// TestPRGate_Verdicts walks the gate's decision table: which pull request state and which approval or rejection record yield Done, Awaiting (with the reason's way forward) or Stuck.
// It stays serial because the fixture swaps the package-level NewGitHubClient.
func TestPRGate_Verdicts(t *testing.T) {
	closedAtHead := fmt.Sprintf(`[{"number":3,"state":"closed","head":{"sha":%q}}]`, prGateHead)
	tests := []struct {
		name     string
		listBody string
		setup    func(t *testing.T, f *prGateFixture)
		want     shedengine.Outcome
		// reasonHas lists substrings the reason carries; {ApprovalPath} and {RejectionPath} stand for the fixture's record paths.
		reasonHas []string
		// reasonSuffix is what the reason ends with, when it must end with something.
		reasonSuffix string
		wantNoGitHub bool
	}{
		{
			name: "parent not requiring a pull request is Done without a GitHub call", listBody: "[]",
			setup:        func(t *testing.T, f *prGateFixture) { f.deps.Config.RequirePRToBase = []string{"other"} },
			want:         shedengine.Done,
			wantNoGitHub: true,
		},
		{
			name: "merged outranks any record", listBody: `[{"number":3,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"sha":"zzz"}}]`,
			setup: func(t *testing.T, f *prGateFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead) },
			want:  shedengine.Done,
		},
		{name: "no pull request awaits", listBody: "[]", want: shedengine.Awaiting, reasonHas: []string{"reopen", "abandon"}},
		{
			name: "closed outranks a matching approval", listBody: closedAtHead,
			setup:     func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) },
			want:      shedengine.Awaiting,
			reasonHas: []string{"closed", "abandon"},
		},
		{
			name: "malformed approval awaits naming its path", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *prGateFixture) {
				if err := writeFile(f.deps.ApprovalPath, "{not json"); err != nil {
					t.Fatal(err)
				}
			},
			want:      shedengine.Awaiting,
			reasonHas: []string{"{ApprovalPath}", "lyx loom approve"},
		},
		{
			name: "malformed rejection awaits naming its path", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *prGateFixture) {
				if err := writeFile(f.deps.RejectionPath, "{}"); err != nil {
					t.Fatal(err)
				}
			},
			want:      shedengine.Awaiting,
			reasonHas: []string{"{RejectionPath}", "lyx loom reject"},
		},
		{
			name: "matching approval is Done", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) },
			want:  shedengine.Done,
		},
		{
			name: "matching rejection is Stuck", listBody: openPR(3, prGateHead),
			setup:     func(t *testing.T, f *prGateFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead) },
			want:      shedengine.Stuck,
			reasonHas: []string{"#3", prGateHead, "rework"},
		},
		{
			name: "matching approval outranks a stale rejection", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *prGateFixture) {
				writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
				writeRejectionAt(t, f.deps.RejectionPath, 3, "old")
			},
			want: shedengine.Done,
		},
		{
			name: "stale approval, pull request head moved", listBody: openPR(3, "bbb"),
			setup:     func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) },
			want:      shedengine.Awaiting,
			reasonHas: []string{"pull request head is now bbb", "lyx loom approve", "lyx loom reject"},
		},
		{
			name: "stale rejection, pull request head moved", listBody: openPR(3, "bbb"),
			setup:     func(t *testing.T, f *prGateFixture) { writeRejectionAt(t, f.deps.RejectionPath, 3, prGateHead) },
			want:      shedengine.Awaiting,
			reasonHas: []string{"pull request head is now bbb", "lyx loom approve", "lyx loom reject"},
		},
		{
			name: "local head moved", listBody: openPR(3, prGateHead),
			setup: func(t *testing.T, f *prGateFixture) {
				writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
				f.deps.TaskHead = func() (string, error) { return "ccc", nil }
			},
			want:      shedengine.Awaiting,
			reasonHas: []string{"local task head is now ccc", "lyx loom approve", "lyx loom reject"},
		},
		{
			name: "pull request number changed", listBody: openPR(4, prGateHead),
			setup:     func(t *testing.T, f *prGateFixture) { writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead) },
			want:      shedengine.Awaiting,
			reasonHas: []string{"open pull request is now #4", "lyx loom approve", "lyx loom reject"},
		},
		{
			name: "no record awaits and ends with the pull request URL", listBody: openPR(3, prGateHead),
			want: shedengine.Awaiting, reasonHas: []string{"lyx loom approve", "lyx loom reject"},
			reasonSuffix: "https://github.com/acme/proj/pull/3",
		},
		{
			name: "non-transient query failure awaits", listBody: "[]",
			setup:     func(t *testing.T, f *prGateFixture) { f.srv.listStatus = http.StatusNotFound },
			want:      shedengine.Awaiting,
			reasonHas: []string{"query existing pull request"},
		},
		{
			name: "unavailable GitHub client awaits", listBody: "[]",
			setup:     func(t *testing.T, f *prGateFixture) { installFailingGitHubClientFactory(t, errors.New("no token")) },
			want:      shedengine.Awaiting,
			reasonHas: []string{"github client unavailable"},
		},
		{
			name: "unusable origin awaits", listBody: "[]",
			setup:     func(t *testing.T, f *prGateFixture) { f.deps.OriginURL = "not a url" },
			want:      shedengine.Awaiting,
			reasonHas: []string{"origin URL unusable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRGateFixture(t, tt.listBody)
			if tt.setup != nil {
				tt.setup(t, f)
			}
			outcome, ptr, err := f.call(t)
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != tt.want {
				t.Fatalf("Call() outcome = %q (%s); want %q", outcome, ptr.Reason, tt.want)
			}
			paths := strings.NewReplacer("{ApprovalPath}", f.deps.ApprovalPath, "{RejectionPath}", f.deps.RejectionPath)
			for _, s := range tt.reasonHas {
				s = paths.Replace(s)
				if !strings.Contains(ptr.Reason, s) {
					t.Errorf("reason %q does not contain %q", ptr.Reason, s)
				}
			}
			if tt.reasonSuffix != "" && !strings.HasSuffix(ptr.Reason, tt.reasonSuffix) {
				t.Errorf("reason %q does not end with %q", ptr.Reason, tt.reasonSuffix)
			}
			if tt.wantNoGitHub && len(f.order) != 0 {
				t.Errorf("GitHub calls = %v; want none", f.order)
			}
		})
	}
}

// TestPRGate_ReturnedErrors pins the failures Call returns as an error rather than a verdict:
// a transient query failure (classified), a TaskHead error, and a cancellation, whether the context was already cancelled at entry or was cancelled after the pull-request query, inside TaskHead, when a matching approval would otherwise have reported Done.
// It stays serial because the fixture swaps the package-level NewGitHubClient.
func TestPRGate_ReturnedErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, f *prGateFixture) context.Context
		check func(t *testing.T, outcome shedengine.Outcome, err error)
	}{
		{
			name: "transient query failure is classified",
			setup: func(t *testing.T, f *prGateFixture) context.Context {
				f.srv.listStatus = http.StatusServiceUnavailable
				return context.Background()
			},
			check: func(t *testing.T, outcome shedengine.Outcome, err error) {
				if err == nil {
					t.Fatalf("Call() error = nil, outcome %q; want a transient error", outcome)
				}
				if shedtransient.Class(err) != shedengine.TransientGitHubAPI {
					t.Errorf("Class(err) = %q; want %q", shedtransient.Class(err), shedengine.TransientGitHubAPI)
				}
			},
		},
		{
			name: "TaskHead error is returned",
			setup: func(t *testing.T, f *prGateFixture) context.Context {
				writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
				f.deps.TaskHead = func() (string, error) { return "", errors.New("boom") }
				return context.Background()
			},
			check: func(t *testing.T, _ shedengine.Outcome, err error) {
				if err == nil || !strings.Contains(err.Error(), "boom") {
					t.Errorf("Call() error = %v; want the TaskHead error", err)
				}
			},
		},
		{
			name: "cancelled context is an error",
			setup: func(t *testing.T, f *prGateFixture) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			check: func(t *testing.T, _ shedengine.Outcome, err error) {
				if err == nil {
					t.Error("Call() under a cancelled context: error = nil; want cancellation error")
				}
			},
		},
		{
			name: "cancellation before a matching approval's Done is an error",
			setup: func(t *testing.T, f *prGateFixture) context.Context {
				writeApprovalAt(t, f.deps.ApprovalPath, 3, prGateHead)
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				f.deps.TaskHead = func() (string, error) {
					cancel()
					return prGateHead, nil
				}
				return ctx
			},
			check: func(t *testing.T, outcome shedengine.Outcome, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Call() = %q, %v; want a context.Canceled error", outcome, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRGateFixture(t, openPR(3, prGateHead))
			ctx := tt.setup(t, f)
			g, err := NewPRGate(f.deps)
			if err != nil {
				t.Fatalf("NewPRGate() error = %v", err)
			}
			outcome, _, err := g.Call(ctx)
			tt.check(t, outcome, err)
		})
	}
}
