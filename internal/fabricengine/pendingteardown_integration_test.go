// pendingteardown_integration_test.go covers what the teardown child leaves of its pending-teardown record.
// It runs the child's body against a real git worktree.

package fabricengine

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunTeardownChild_SettlesThePendingRecordOnceItEnds covers what the child does with the record its parent wrote.
// A child that ends with no remote steps, or whose job ran, removes it, so status and prune name nothing.
// A child whose remote step failed keeps it and writes the failed step's way forward into it.
func TestRunTeardownChild_SettlesThePendingRecordOnceItEnds(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	const slug = "pair"
	taskTip := strings.Repeat("b", 40)
	taskTag := fabriclayout.ArchiveTagName(slug, taskTip)
	encode := func(t *testing.T, job remoteTeardownJob) string {
		t.Helper()
		raw, err := json.Marshal(job)
		if err != nil {
			t.Fatalf("encode the job: %v", err)
		}
		return string(raw)
	}
	tests := []struct {
		name string
		// unreachableOrigin gives the worktree an origin that does not answer, so the job's first read of it fails.
		unreachableOrigin bool
		// input returns the job the parent hands the child, given the worktree.
		input func(t *testing.T, worktree string) string
		// wantWayForward returns the way forward of the record the child keeps; nil means the child removes the record.
		wantWayForward func(worktree string) string
	}{
		{name: "NoRemoteSteps", input: func(t *testing.T, worktree string) string { return "" }},
		{name: "AJobThatRan", input: func(t *testing.T, worktree string) string {
			return encode(t, remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug, Worktree: worktree, WarpBranch: slug})
		}},
		{
			name:              "AJobWhoseRemoteStepFailedKeepsTheRecordWithItsWayForward",
			unreachableOrigin: true,
			input: func(t *testing.T, worktree string) string {
				return encode(t, remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug, Worktree: worktree, WarpBranch: slug, TaskTip: taskTip})
			},
			wantWayForward: func(worktree string) string {
				return "push the archive tag with `git -C " + worktree + " push origin refs/tags/" + taskTag + ":refs/tags/" + taskTag +
					"`, then run the command that discarded the task branch again"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			worktree := filepath.Join(hub, "code")
			if err := os.MkdirAll(worktree, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", worktree, err)
			}
			gitkit.Git(t, worktree, "init", "-b", "main")
			if tt.unreachableOrigin {
				gitkit.Git(t, worktree, "remote", "add", "origin", filepath.Join(hub, "no-such-origin"))
			}
			if err := writePendingTeardown(hub, remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug}, time.Now()); err != nil {
				t.Fatalf("writePendingTeardown() error = %v; want nil", err)
			}

			if err := RunTeardownChild(hub, slug, false, time.Second, strings.NewReader(tt.input(t, worktree)), io.Discard); err != nil {
				t.Fatalf("RunTeardownChild() error = %v; want nil", err)
			}

			if tt.wantWayForward == nil {
				if _, err := os.Stat(pendingTeardownPath(hub, slug)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("pending teardown record stat error = %v; want the record removed", err)
				}
				return
			}
			pending, err := PendingTeardowns(hub)
			if err != nil {
				t.Fatalf("PendingTeardowns() error = %v; want nil", err)
			}
			if want := tt.wantWayForward(worktree); len(pending) != 1 || pending[0].Slug != slug || pending[0].WayForward != want {
				t.Errorf("PendingTeardowns() = %+v; want one record of %s with the way forward %q", pending, slug, want)
			}
		})
	}
}
