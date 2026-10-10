package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/google/go-github/v75/github"
)

// rejectRouting is a two-row recipe whose gate row carries a budget of two.
func rejectRouting() shedengine.Routing {
	return shedengine.Routing{
		Entry: loomshed.NamePRGate,
		Producers: []shedengine.ProducerDef{
			{Name: loomshed.NamePRGate, Segment: "PR-Review", MaxBounces: 2, OnStuck: loomshed.NamePRRework},
			{Name: loomshed.NamePRRework, Segment: "PR-Review"},
		},
	}
}

// rejectCalls records the order of the record-mutating seams and the rejections written.
type rejectCalls struct {
	order   []string
	written []landingshed.Rejection
}

// rejectFixture builds rejectDeps that pass every check at an awaiting gate, plus the call record;
// each test breaks one seam.
func rejectFixture() (rejectDeps, *rejectCalls) {
	calls := &rejectCalls{}
	d := rejectDeps{
		readStatus: func() (shedengine.Status, bool, error) {
			return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate}, true, nil
		},
		branches: func() (string, string, string, error) {
			return "task", "main", "https://github.com/acme/widgets.git", nil
		},
		findPR: func(context.Context, string, string, string, string) (*github.PullRequest, error) {
			return &github.PullRequest{
				Number:  github.Ptr(7),
				State:   github.Ptr("open"),
				HTMLURL: github.Ptr("https://github.com/acme/widgets/pull/7"),
				Head:    &github.PullRequestBranch{SHA: github.Ptr("abc123")},
			}, nil
		},
		headSHA:        func() (string, error) { return "abc123", nil },
		readReviewFile: func(string) (string, error) { return "fix the thing\n", nil },
		removeApproval: func() error {
			calls.order = append(calls.order, "remove")
			return nil
		},
		writeRejection: func(r landingshed.Rejection) error {
			calls.order = append(calls.order, "write")
			calls.written = append(calls.written, r)
			return nil
		},
		routing:          rejectRouting(),
		reworkRow:        loomshed.NamePRRework,
		rejectionPending: func() (bool, error) { return false, nil },
		now:              func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
	}
	return d, calls
}

// darnRejectRouting is the darn recipe's PR-Review pair: the gate bounces to the Darn row.
func darnRejectRouting() shedengine.Routing {
	return shedengine.Routing{
		Entry: loomshed.NamePRGate,
		Producers: []shedengine.ProducerDef{
			{Name: loomshed.NamePRGate, Segment: "PR-Review", MaxBounces: 2, OnStuck: loomshed.NameDarn},
			{Name: loomshed.NameDarn, Segment: "PR-Review"},
		},
	}
}

// asDarn turns d into the deps of a darn run, whose rework row is Darn and whose rejection record is pending as given.
func asDarn(d *rejectDeps, pending bool) {
	d.routing = darnRejectRouting()
	d.reworkRow = loomshed.NameDarn
	d.rejectionPending = func() (bool, error) { return pending, nil }
}

func TestReworkRowFor(t *testing.T) {
	for recipe, want := range map[string]string{
		"":                 loomshed.NamePRRework,
		shedrun.RecipeLoom: loomshed.NamePRRework,
		shedrun.RecipeDarn: loomshed.NameDarn,
	} {
		if got := reworkRowFor(recipe); got != want {
			t.Errorf("reworkRowFor(%q) = %q; want %q", recipe, got, want)
		}
	}
}

// stuckHistory returns n Stuck entries for the gate row.
func stuckHistory(n int) []shedengine.HistoryEntry {
	var h []shedengine.HistoryEntry
	for i := 0; i < n; i++ {
		h = append(h, shedengine.HistoryEntry{Producer: loomshed.NamePRGate, Outcome: shedengine.Stuck})
	}
	return h
}

func TestRejectVerb_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*rejectDeps)
		wantMsg string
	}{
		{
			name: "NoStatusFile",
			mutate: func(d *rejectDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) { return shedengine.Status{}, false, nil }
			},
			wantMsg: "no status file",
		},
		{
			name: "RunningAtGate",
			mutate: func(d *rejectDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePRGate}, true, nil
				}
			},
			wantMsg: "not awaiting or blocked",
		},
		{
			name: "AwaitingAtOtherProducer",
			mutate: func(d *rejectDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePublish}, true, nil
				}
			},
			wantMsg: "not awaiting or blocked",
		},
		{
			name: "AwaitingAtRework",
			mutate: func(d *rejectDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRRework}, true, nil
				}
			},
			wantMsg: "not awaiting or blocked",
		},
		{
			name: "DarnBlockedWithNoRejectionPending",
			mutate: func(d *rejectDeps) {
				asDarn(d, false)
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NameDarn}, true, nil
				}
			},
			wantMsg: "no rejection pending",
		},
		{
			name: "DarnRunAtPRReworkNamesBothRows",
			mutate: func(d *rejectDeps) {
				asDarn(d, true)
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NamePRRework}, true, nil
				}
			},
			wantMsg: "blocked at PR-Gate, nor blocked at Darn",
		},
		{
			name: "BudgetExhaustedAtGate",
			mutate: func(d *rejectDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate, History: stuckHistory(2)}, true, nil
				}
			},
			wantMsg: "lyx loom approve",
		},
		{
			name: "OriginUnusable",
			mutate: func(d *rejectDeps) {
				d.branches = func() (string, string, string, error) { return "task", "main", "not a url", nil }
			},
			wantMsg: "origin URL unusable",
		},
		{
			name: "NoPullRequest",
			mutate: func(d *rejectDeps) {
				d.findPR = func(context.Context, string, string, string, string) (*github.PullRequest, error) { return nil, nil }
			},
			wantMsg: "no pull request",
		},
		{
			name: "PullRequestNotOpen",
			mutate: func(d *rejectDeps) {
				d.findPR = func(context.Context, string, string, string, string) (*github.PullRequest, error) {
					return &github.PullRequest{Number: github.Ptr(7), State: github.Ptr("closed")}, nil
				}
			},
			wantMsg: "not open",
		},
		{
			name: "HeadDiffers",
			mutate: func(d *rejectDeps) {
				d.headSHA = func() (string, error) { return "def456", nil }
			},
			wantMsg: "def456",
		},
		{
			name: "ReviewFileUnreadable",
			mutate: func(d *rejectDeps) {
				d.readReviewFile = func(string) (string, error) { return "", errors.New("no such file") }
			},
			wantMsg: "no such file",
		},
		{
			name: "ReviewFileEmpty",
			mutate: func(d *rejectDeps) {
				d.readReviewFile = func(string) (string, error) { return " \n\t\n", nil }
			},
			wantMsg: "is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, calls := rejectFixture()
			tt.mutate(&d)
			var out bytes.Buffer
			if code := rejectVerb(context.Background(), &out, d, "review.md"); code != 1 {
				t.Fatalf("exit = %d; want 1; out = %s", code, out.String())
			}
			if n := strings.Count(out.String(), "\n"); n != 1 {
				t.Errorf("emitted %d envelopes; want exactly one: %q", n, out.String())
			}
			if !strings.Contains(out.String(), tt.wantMsg) {
				t.Errorf("output %q does not contain %q", out.String(), tt.wantMsg)
			}
			if len(calls.order) != 0 {
				t.Errorf("mutated records %v on a refusal; want none", calls.order)
			}
		})
	}
}

// TestRejectVerb_Success asserts a rejection at the awaiting gate, and a replacement rejection at the blocked rework row past the gate's budget, removes the approval, writes the rejection and prints the resume envelope.
func TestRejectVerb_Success(t *testing.T) {
	tests := []struct {
		name   string
		status shedengine.Status
		// darn makes the deps a darn run's with a rejection pending.
		darn bool
	}{
		{"AwaitingGate", shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate}, false},
		// The budget is not consulted at the rework row, so a record is replaced past it.
		{"BlockedReworkPastBudget", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NamePRRework, History: stuckHistory(2)}, false},
		{"DarnAwaitingGate", shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate}, true},
		{"DarnBlockedWithRejectionPending", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NameDarn, History: stuckHistory(2)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, calls := rejectFixture()
			if tt.darn {
				asDarn(&d, true)
			}
			d.readStatus = func() (shedengine.Status, bool, error) { return tt.status, true, nil }
			var out bytes.Buffer
			if code := rejectVerb(context.Background(), &out, d, "review.md"); code != 0 {
				t.Fatalf("exit = %d; want 0; out = %s", code, out.String())
			}
			if got := strings.Join(calls.order, ","); got != "remove,write" {
				t.Errorf("call order = %q; want approval removed before the rejection is written", got)
			}
			want := landingshed.Rejection{PRNumber: 7, HeadSHA: "abc123", RejectedAt: "2026-09-30T12:00:00Z", Findings: "fix the thing\n"}
			if len(calls.written) != 1 || calls.written[0] != want {
				t.Fatalf("written = %+v; want [%+v]", calls.written, want)
			}
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope %q: %v", out.String(), err)
			}
			keys := map[string]bool{"pr_number": true, "pr_url": true, "head_sha": true, "resume": true}
			for k := range env {
				if k == "ok" {
					continue
				}
				if !keys[k] {
					t.Errorf("unexpected envelope key %q in %v", k, env)
				}
				delete(keys, k)
			}
			if len(keys) != 0 {
				t.Errorf("envelope %v is missing keys %v", env, keys)
			}
			if env["resume"] != "lyx loom start" || env["head_sha"] != "abc123" || env["pr_number"] != float64(7) {
				t.Errorf("envelope = %v; want resume, head_sha and pr_number", env)
			}
		})
	}
}
