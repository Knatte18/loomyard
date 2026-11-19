// remoteteardown_integration_test.go covers RunTeardownChild running a removal's remote job against a real hub.
// The job pushes the weft branch's tip, tags the archive, and then deletes the branch on origin with a lease.
// The teardown lock is held through the last step and released after it.
//
// Every hub is built through hubforge.CopyHub, with the hub's RecordsBare as the weft origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunTeardownChild_RunsTheRemoteStepsInOrderUnderTheLock covers the detached half of a removal.
// The detached half runs over a weft tip that origin does not have.
// The child pushes the tip, tags it, and then deletes the branch on origin.
// It holds the teardown lock at each step and releases it on return.
// A failed archive tag stops the later steps and leaves the pushed branch in place;
// it mails one notice and still releases the lock.
// The rows set package-level hooks and install the process's push notifier.
// So neither the test nor its rows are parallel.
func TestRunTeardownChild_RunsTheRemoteStepsInOrderUnderTheLock(t *testing.T) {
	tiergate.Require(t, 1)

	tests := []struct {
		name                   string
		tagPushError           error
		wantEvents             []string
		wantWeftBranchOnOrigin bool
		wantNotices            int
	}{
		{"ArchiveThenLeasedDelete", nil, []string{"tag", "delete"}, false, 0},
		{"FailedArchiveSkipsTheLaterSteps", errors.New("tag push refused"), []string{"tag"}, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const slug = "child-order"
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(l))
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			weftBranch := fabricengine.RecordsBranchName(cfg.BranchPrefix + slug)
			tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "_lym/record.txt", "run record\n", "record")
			wantTag := wantArchiveTag(slug, tip)
			// A removal hands the remote steps over after its local teardown.
			// The sibling worktree and the local branch are therefore gone when the child runs.
			weftRoot := mustRecordsRepoRoot(t, l)
			gitkit.MustRun(t, weftRoot, "git", "worktree", "remove", "--force", h.PairRecordsSibling(slug))
			gitkit.MustRun(t, weftRoot, "git", "branch", "-D", weftBranch)
			lockPath := fabricengine.TeardownLockPath(l.HubPath, slug)

			lockHeld := func() bool {
				held, acquired, err := lock.TryAcquireReadLock(lockPath)
				if err != nil {
					t.Errorf("TryAcquireReadLock() error = %v", err)
				}
				if acquired {
					_ = held.Release()
				}
				return !acquired
			}
			var notices []string
			t.Cleanup(fabricengine.InstallPushNotifier(func(hubPath string, notice fabricengine.PushNotice) (bool, error) {
				notices = append(notices, notice.Line)
				return true, nil
			}))
			var events []string
			fabricengine.SetArchiveTagPushHookForTest(t, func(tagRef string) error {
				events = append(events, "tag")
				if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != tip {
					t.Errorf("origin weft branch at the tag push = %s; want the tip %s pushed first", got, tip)
				}
				if !lockHeld() {
					t.Errorf("teardown lock free at the tag push; want it held")
				}
				return tt.tagPushError
			})
			fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
				events = append(events, "delete")
				if got := tagTargetAt(t, h.RecordsBare, wantTag); got != tip {
					t.Errorf("origin tag %s at the delete points at %q; want the tip %s pushed first", wantTag, got, tip)
				}
				if !lockHeld() {
					t.Errorf("teardown lock free at the last remote step; want it held")
				}
			})

			job, err := json.Marshal(map[string]any{
				"kind":                  "remove",
				"slug":                  slug,
				"worktree":              l.WorktreePath(),
				"weft_branch":           weftBranch,
				"weft_tip":              tip,
				"delete_weft_on_origin": true,
			})
			if err != nil {
				t.Fatalf("marshal the job: %v", err)
			}
			var signal strings.Builder
			if err := fabricengine.RunTeardownChild(l.HubPath, slug, false, time.Second, bytes.NewReader(job), &signal); err != nil {
				t.Fatalf("RunTeardownChild() error = %v; want nil", err)
			}

			if !slices.Equal(events, tt.wantEvents) {
				t.Errorf("remote steps = %v; want %v", events, tt.wantEvents)
			}
			onOrigin, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/heads/" + weftBranch}, h.RecordsBare)
			if err != nil {
				t.Fatalf("read the weft branch on origin: %v", err)
			}
			if got := strings.TrimSpace(onOrigin); (got != "") != tt.wantWeftBranchOnOrigin || (tt.wantWeftBranchOnOrigin && got != tip) {
				t.Errorf("origin weft branch = %q; want present %v at %s", got, tt.wantWeftBranchOnOrigin, tip)
			}
			if lockHeld() {
				t.Errorf("teardown lock still held after RunTeardownChild returned; want it released")
			}
			if len(notices) != tt.wantNotices || (tt.wantNotices > 0 && !strings.Contains(notices[0], slug)) {
				t.Errorf("notices = %q; want %d, naming %q", notices, tt.wantNotices, slug)
			}
		})
	}
}
