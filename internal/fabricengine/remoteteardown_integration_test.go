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
	"os"
	"path/filepath"
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

	t.Run("OneRepositoryTagThenBranchThenViewRef", func(t *testing.T) {
		const slug = "child-one-repo"
		s := newOneRepositoryTeardown(t, slug)
		l, added, tip, viewRef, wantTag, job := s.hub.Location, s.added, s.tip, s.viewRef, s.wantTag, s.job
		onOrigin := func(ref string) string { return s.onOrigin(t, ref) }

		var notices []string
		t.Cleanup(fabricengine.InstallPushNotifier(func(hubPath string, notice fabricengine.PushNotice) (bool, error) {
			notices = append(notices, notice.Line)
			return true, nil
		}))
		var events []string
		fabricengine.SetArchiveTagPushHookForTest(t, func(string) error {
			events = append(events, "tag")
			return nil
		})
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			events = append(events, "delete")
			if got := tagTargetAt(t, s.hub.CodeBare, wantTag); got != tip {
				t.Errorf("origin tag %s at the branch delete points at %q; want the tip %s pushed first", wantTag, got, tip)
			}
			if got := onOrigin(viewRef); got != tip {
				t.Errorf("origin view ref at the branch delete = %q; want it still there, deleted after the branch", got)
			}
		})

		var signal strings.Builder
		if err := fabricengine.RunTeardownChild(l.HubPath, slug, false, time.Second, bytes.NewReader(job), &signal); err != nil {
			t.Fatalf("RunTeardownChild() error = %v; want nil", err)
		}

		if want := []string{"tag", "delete"}; !slices.Equal(events, want) {
			t.Errorf("remote steps = %v; want %v", events, want)
		}
		if got := onOrigin("refs/heads/" + added.Branch); got != "" {
			t.Errorf("origin branch = %q; want it deleted", got)
		}
		if got := onOrigin(viewRef); got != "" {
			t.Errorf("origin view ref = %q; want it deleted after the branch", got)
		}
		if len(notices) != 0 {
			t.Errorf("notices = %q; want none", notices)
		}
	})

	t.Run("OneRepositoryBranchThatMovedDuringTheRemovalIsKept", func(t *testing.T) {
		const slug = "child-moved"
		s := newOneRepositoryTeardown(t, slug)
		var notices []string
		t.Cleanup(fabricengine.InstallPushNotifier(func(hubPath string, notice fabricengine.PushNotice) (bool, error) {
			notices = append(notices, notice.Line)
			return true, nil
		}))
		fabricengine.SetArchiveTagPushHookForTest(t, func(string) error { return nil })
		var moved string
		fabricengine.SetRemoveBeforeWeftDeleteHookForTest(t, func() {
			other := cloneBareForTest(t, s.hub.CodeBare, s.added.Branch)
			moved = gitkit.CommitFile(t, other, "_lym/moved.txt", "moved\n", "another process moves the branch")
			gitkit.Git(t, other, "push", "origin", s.added.Branch)
		})

		var signal strings.Builder
		if err := fabricengine.RunTeardownChild(s.hub.Location.HubPath, slug, false, time.Second, bytes.NewReader(s.job), &signal); err != nil {
			t.Fatalf("RunTeardownChild() error = %v; want nil", err)
		}

		if got := s.onOrigin(t, "refs/heads/"+s.added.Branch); moved == "" || got != moved {
			t.Errorf("origin branch = %q; want it kept at the other process's tip %q", got, moved)
		}
		if len(notices) != 0 {
			t.Errorf("notices = %q; want none, since a kept branch is no failure", notices)
		}
	})

	t.Run("OneRepositoryViewRefThatCannotBeDeletedNamesTheDeleteByHand", func(t *testing.T) {
		const slug = "child-view-fail"
		s := newOneRepositoryTeardown(t, slug)
		hook := "#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in refs/heads/" + fabricengine.ViewRefPrefix + "*) echo \"view refs are locked\" >&2; exit 1;; esac\ndone\n"
		if err := os.WriteFile(filepath.Join(s.hub.CodeBare, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
			t.Fatalf("install the pre-receive hook: %v", err)
		}
		var notices []string
		t.Cleanup(fabricengine.InstallPushNotifier(func(hubPath string, notice fabricengine.PushNotice) (bool, error) {
			notices = append(notices, notice.Line)
			return true, nil
		}))
		fabricengine.SetArchiveTagPushHookForTest(t, func(string) error { return nil })

		var signal strings.Builder
		if err := fabricengine.RunTeardownChild(s.hub.Location.HubPath, slug, false, time.Second, bytes.NewReader(s.job), &signal); err != nil {
			t.Fatalf("RunTeardownChild() error = %v; want nil", err)
		}

		wantWayForward := "push origin --delete " + fabricengine.ViewRefPrefix + slug
		if len(notices) != 1 || !strings.Contains(notices[0], "the delete of its view ref on origin") || !strings.Contains(notices[0], wantWayForward) {
			t.Errorf("notices = %q; want one naming the view ref step and %q", notices, wantWayForward)
		}
		if got := s.onOrigin(t, s.viewRef); got != s.tip {
			t.Errorf("origin view ref = %q; want it left at %q", got, s.tip)
		}
	})
}

// oneRepositoryTeardown is a removed one-repository pair whose remote steps a teardown job still has to run.
// The pair's worktree and branch are gone, and its tip sits on origin's view ref.
type oneRepositoryTeardown struct {
	hub     *hubforge.Hub
	added   fabricengine.AddResult
	tip     string
	viewRef string
	wantTag string
	// job is the removal's remote teardown job, as the child reads it.
	job []byte
}

// newOneRepositoryTeardown adds the pair slug to a fresh one-repository hub and publishes its view ref.
// It then removes the pair locally.
func newOneRepositoryTeardown(t *testing.T, slug string) oneRepositoryTeardown {
	t.Helper()

	h := hubforge.NewOneRepoHub(t, ".")
	l := h.Location
	added := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	tip := gitkit.CommitFile(t, added.Path, "_lym/record.txt", "run record\n", "record")
	viewRef := "refs/heads/" + fabricengine.ViewRefPrefix + slug
	gitkit.Git(t, added.Path, "push", "origin", tip+":"+viewRef)
	// A removal hands the remote steps over after its local teardown, so the pair's worktree and branch are gone.
	gitkit.MustRun(t, l.WorktreePath(), "git", "worktree", "remove", "--force", added.Path)
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", "-D", added.Branch)
	job, err := json.Marshal(map[string]any{
		"kind":                  "remove",
		"slug":                  slug,
		"worktree":              l.WorktreePath(),
		"weft_branch":           added.Branch,
		"weft_tip":              tip,
		"delete_weft_on_origin": true,
		"delete_view_ref":       true,
	})
	if err != nil {
		t.Fatalf("marshal the job: %v", err)
	}
	return oneRepositoryTeardown{hub: h, added: added, tip: tip, viewRef: viewRef, wantTag: wantArchiveTag(slug, tip), job: job}
}

// onOrigin returns the object name ref has on origin, and "" when origin holds none.
func (s oneRepositoryTeardown) onOrigin(t *testing.T, ref string) string {
	t.Helper()
	return gitkit.Git(t, s.hub.CodeBare, "for-each-ref", "--format=%(objectname)", ref)
}
