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
	"github.com/google/go-github/v75/github"
)

// approveFixture builds approveDeps that pass every check, plus a pointer to the recorded
// approvals; each test breaks one seam.
func approveFixture() (approveDeps, *[]landingshed.Approval) {
	var written []landingshed.Approval
	d := approveDeps{
		readStatus: func() (shedengine.Status, bool, error) {
			return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NamePublish}, true, nil
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
		headSHA: func() (string, error) { return "abc123", nil },
		writeApproval: func(a landingshed.Approval) error {
			written = append(written, a)
			return nil
		},
		now: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
	}
	return d, &written
}

func TestApproveVerb_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*approveDeps)
		wantMsg string
	}{
		{
			name: "NotBlocked",
			mutate: func(d *approveDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePublish}, true, nil
				}
			},
			wantMsg: "not awaiting or blocked",
		},
		{
			name: "BlockedAtOtherProducer",
			mutate: func(d *approveDeps) {
				d.readStatus = func() (shedengine.Status, bool, error) {
					return shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: loomshed.NameFinalize}, true, nil
				}
			},
			wantMsg: "not awaiting or blocked",
		},
		{
			name: "NoPullRequest",
			mutate: func(d *approveDeps) {
				d.findPR = func(context.Context, string, string, string, string) (*github.PullRequest, error) { return nil, nil }
			},
			wantMsg: "no pull request",
		},
		{
			name: "PullRequestNotOpen",
			mutate: func(d *approveDeps) {
				d.findPR = func(context.Context, string, string, string, string) (*github.PullRequest, error) {
					return &github.PullRequest{Number: github.Ptr(7), State: github.Ptr("closed")}, nil
				}
			},
			wantMsg: "not open",
		},
		{
			name: "HeadDiffers",
			mutate: func(d *approveDeps) {
				d.headSHA = func() (string, error) { return "def456", nil }
			},
			wantMsg: "def456",
		},
		{
			name: "LookupError",
			mutate: func(d *approveDeps) {
				d.findPR = func(context.Context, string, string, string, string) (*github.PullRequest, error) {
					return nil, errors.New("boom")
				}
			},
			wantMsg: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, written := approveFixture()
			tt.mutate(&d)
			var out bytes.Buffer
			if code := approveVerb(context.Background(), &out, d); code != 1 {
				t.Fatalf("exit = %d; want 1; out = %s", code, out.String())
			}
			if !strings.Contains(out.String(), tt.wantMsg) {
				t.Errorf("output %q does not contain %q", out.String(), tt.wantMsg)
			}
			if len(*written) != 0 {
				t.Errorf("wrote %d approvals on a refusal; want none", len(*written))
			}
		})
	}
}

func TestApproveVerb_Success(t *testing.T) {
	d, written := approveFixture()
	var out bytes.Buffer
	if code := approveVerb(context.Background(), &out, d); code != 0 {
		t.Fatalf("exit = %d; want 0; out = %s", code, out.String())
	}
	want := landingshed.Approval{PRNumber: 7, HeadSHA: "abc123", ApprovedAt: "2026-09-30T12:00:00Z"}
	if len(*written) != 1 || (*written)[0] != want {
		t.Fatalf("written = %+v; want [%+v]", *written, want)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out.String(), err)
	}
	if env["resume"] != "lyx loom start" || env["head_sha"] != "abc123" || env["pr_number"] != float64(7) {
		t.Errorf("envelope = %v; want resume, head_sha and pr_number", env)
	}
	if !strings.Contains(out.String(), "pull/7") {
		t.Errorf("envelope %q does not name the PR URL", out.String())
	}
}
