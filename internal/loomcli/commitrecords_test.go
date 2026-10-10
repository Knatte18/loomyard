// commitrecords_test.go drives commitRecordsVerb over stub commitStatusDeps closures, spawning no git.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// stubRecordsDeps builds commitStatusDeps counting Commit and Push calls.
func stubRecordsDeps(active bool, probeErr, commitErr, pushErr error) (commitStatusDeps, *int, *int) {
	commits, pushes := 0, 0
	return commitStatusDeps{
		MergeActive: func() (bool, error) { return active, probeErr },
		Commit:      func(string) error { commits++; return commitErr },
		Push:        func() error { pushes++; return pushErr },
	}, &commits, &pushes
}

// TestCommitRecordsVerb_Success asserts a clean tree commits then pushes once each, and a mid-merge tree commits and pushes nothing and reports a skipped reason.
func TestCommitRecordsVerb_Success(t *testing.T) {
	tests := []struct {
		name                    string
		midMerge                bool
		wantCommits, wantPushes int
		wantOutput              []string
	}{
		{"ordinary path commits then pushes", false, 1, 1, nil},
		{"mid-merge skips", true, 0, 0, []string{`"committed":false`, "mid-merge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, commits, pushes := stubRecordsDeps(tt.midMerge, nil, nil, nil)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code != 0 {
				t.Fatalf("exit = %d; want 0; output %s", code, out.String())
			}
			if *commits != tt.wantCommits || *pushes != tt.wantPushes {
				t.Errorf("Commit/Push calls = %d/%d; want %d/%d", *commits, *pushes, tt.wantCommits, tt.wantPushes)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output = %s; want it to contain %q", out.String(), want)
				}
			}
		})
	}
}

func TestCommitRecordsVerb_ErrorEnvelopes(t *testing.T) {
	boom := errors.New("boom")
	rejected := fmt.Errorf("push side at /wt: %w", gitrepo.ErrPushRejected)
	const (
		rejectionWayForward = "way forward: merge the remote branch into the local branch in the worktree the error names"
		refusedVerbClause   = "a session lyx refuses the verb from reports status: FAILED and the orch runs it"
		transientWayForward = "way forward: transient, run lyx fabric push or re-run lyx loom commit-records; " + refusedVerbClause
		pushFailedPrefix    = "the commit landed locally but the push failed: "
	)
	tests := []struct {
		name                       string
		probeErr, commitErr, pushE error
		want                       []string
	}{
		{"probe", boom, nil, nil, []string{"probe merge state"}},
		{"commit", nil, boom, nil, []string{"commit failed"}},
		{"push rejected", nil, nil, rejected, []string{pushFailedPrefix, rejectionWayForward, refusedVerbClause}},
		{"push failed", nil, nil, boom, []string{pushFailedPrefix, transientWayForward}},
		{"push lock busy", nil, nil, fmt.Errorf("push: %w", fabricengine.ErrPushLockBusy), []string{pushFailedPrefix, transientWayForward}},
		{"rejected beside another failure", nil, nil, errors.Join(rejected, boom), []string{pushFailedPrefix, rejectionWayForward, "boom", "remote diverged"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, _ := stubRecordsDeps(false, tt.probeErr, tt.commitErr, tt.pushE)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code == 0 {
				t.Fatalf("exit = 0; want non-zero; output %s", out.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output = %s; want it to contain %q", out.String(), want)
				}
			}
			if strings.Contains(out.String(), "not pushed") {
				t.Errorf("output = %s; want no claim that the records were not pushed", out.String())
			}
			if !strings.Contains(out.String(), "way forward: ") {
				t.Errorf("output = %s; want a way forward", out.String())
			}
			// The failure was transient: the re-run the message names succeeds.
			deps, _, _ = stubRecordsDeps(false, nil, nil, nil)
			out.Reset()
			if code := commitRecordsVerb(&out, deps); code != 0 {
				t.Errorf("re-run exit = %d; want 0; output %s", code, out.String())
			}
		})
	}
}
