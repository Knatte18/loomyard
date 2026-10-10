//go:build integration

// publish_integration_test.go drives Publish's merge-in half against a real pair built by the
// standard hub fixture (internal/hubforge), with only the GitHub client faked -- an httptest server
// standing in for the real service, mirroring the in-package unit tier's own publishGitHubServer.
// No conflict is staged in this scenario: the merge-in half is the point, and the byte-identical
// conflict shape across both sides is already covered exhaustively by internal/fabricengine's own
// mergein_integration_test.go and this file's own sibling (finalize_integration_test.go). No test
// in this file contacts a real service or a real model.

package landingshed_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// publishIntegrationGitHubServer is a scripted httptest server standing in for the GitHub API: its
// List handler reports no existing pull request, and its Create handler asserts the pair is clean
// and carries the parent branch's own content before it ever responds -- the "left clean and
// current before the create call is made" assertion this file exists for.
type publishIntegrationGitHubServer struct {
	server        *httptest.Server
	t             *testing.T
	taskWorktree  string
	beforeCreate  func()
	createChecked bool
}

func newPublishIntegrationGitHubServer(t *testing.T, taskWorktree string) *publishIntegrationGitHubServer {
	t.Helper()
	s := &publishIntegrationGitHubServer{t: t, taskWorktree: taskWorktree}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[]"))
		case http.MethodPost:
			s.createChecked = true
			if out := gitkit.GitStatusPorcelain(t, s.taskWorktree); out != "" {
				t.Errorf("task worktree git status --porcelain before the create call = %q; want clean", out)
			}
			if _, err := os.Stat(filepath.Join(s.taskWorktree, "parent-progress.txt")); err != nil {
				t.Errorf("parent-progress.txt missing from the task worktree before the create call: %v; want the merge-in to have already landed the parent branch's own content", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"number":1,"state":"open"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

// install swaps landingshed.NewGitHubClient with a closure returning a real go-github client pointed
// at s's server, restoring the original on test cleanup -- mirrors the in-package unit tier's own
// publishGitHubServer.install.
func (s *publishIntegrationGitHubServer) install(t *testing.T) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(s.server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed

	orig := landingshed.NewGitHubClient
	landingshed.NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { landingshed.NewGitHubClient = orig })
}

// newPublishDepsAt builds Publish's Deps over the task worktree at taskWorktree: its real pair opener, a fake conflict-resolution session that must never spawn, and a require-PR config for main.
// The caller sets the push closure.
func newPublishDepsAt(t *testing.T, taskWorktree string) landingshed.Deps {
	t.Helper()
	deps := landingshed.NewTestDeps(t)
	deps.WorktreeRoot = taskWorktree
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskWorktree), nil }
	// These scenarios stage no conflict, so the conflict-resolution session must never spawn.
	deps.Shuttle = &shedfake.MergeShuttle{RunFn: func(shuttleengine.Spec) (shuttleengine.Result, error) {
		t.Fatal("fake shuttle Run() called; want the clean merge-in to need no conflict-resolution session")
		return shuttleengine.Result{}, nil
	}}
	deps.Config = landingshed.Config{
		RequirePRToBase:    []string{"main"},
		Conflict:           "claude:test-model",
		ConflictTimeoutMin: 1,
	}
	return deps
}

// TestPublish_MergesInCleanlyBeforeCreatingPullRequest drives Publish against a real pair: the task
// worktree catches up with the parent branch (a clean, non-conflicting merge-in), pushes (a no-op
// closure -- push mechanics are internal/gitrepo's own tier's job), and only then queries and creates
// against the faked GitHub client, which itself asserts the pair is clean and carries the parent
// branch's own content before it ever answers the create call.
func TestPublish_MergesInCleanlyBeforeCreatingPullRequest(t *testing.T) {
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	taskWorktree := h.PrimeWorktree()

	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "-b", "task-branch")
	// Advance main (the parent branch) with a clean commit while the task branch stays behind, so
	// the merge-in step below has genuine, non-conflicting content to bring in.
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "main")
	gitkit.CommitFile(t, taskWorktree, "parent-progress.txt", "parent progress\n", "main: progress")
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "task-branch")

	var pushed bool
	deps := newPublishDepsAt(t, taskWorktree)
	deps.PushBranch = func() error { pushed = true; return nil }

	server := newPublishIntegrationGitHubServer(t, taskWorktree)
	server.install(t)

	p, err := landingshed.NewPublish(deps)
	if err != nil {
		t.Fatalf("NewPublish() error = %v; want nil", err)
	}

	// Done: the next row is the PR-Gate, which owns the review wait.
	shedfake.RequireOutcome(t, p, shedengine.Done)
	if !pushed {
		t.Error("push closure never called; want the task branch pushed before the create call")
	}
	if !server.createChecked {
		t.Error("the create call never landed; want the clean-and-current assertion to have run")
	}
}

// TestPublish_RejectedPushNamesRemoteTipThenResumesAfterMerge drives both halves of one scenario over a real pair with its origin:
// the remote task branch holds a commit the local branch lacks, so the real push is rejected and Publish stops Stuck naming the real remote tip and a count of 1;
// after the way forward's merge of origin/<task-branch> in the task worktree, a re-run pushes and reaches the GitHub step.
func TestPublish_RejectedPushNamesRemoteTipThenResumesAfterMerge(t *testing.T) {
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	taskWorktree := h.PrimeWorktree()

	// The remote task branch gets a commit the local one never sees: push it, then rewind the local branch.
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "-b", "task-branch")
	base := gitkit.RevParse(t, taskWorktree, "HEAD")
	remoteOnly := gitkit.CommitFile(t, taskWorktree, "remote-only.txt", "remote only\n", "task-branch: pushed from elsewhere")
	gitkit.Git(t, taskWorktree, "push", "origin", "task-branch")
	gitkit.Git(t, taskWorktree, "reset", "--hard", base)
	// The parent advances too, so the merge-in lands a commit the remote task branch lacks and the push cannot fast-forward.
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "main")
	gitkit.CommitFile(t, taskWorktree, "parent-progress.txt", "parent progress\n", "main: progress")
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "task-branch")

	deps := newPublishDepsAt(t, taskWorktree)
	fabricHandle := openFabricAtLanding(t, taskWorktree)
	deps.PushBranch = func() error {
		_, err := fabricHandle.PushBranch(fabricengine.SyncOptions{})
		return err
	}
	deps.RemoteOnlyCommits = fabricHandle.RemoteOnlyCommits
	p, err := landingshed.NewPublish(deps)
	if err != nil {
		t.Fatalf("NewPublish() error = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	wantReason := "push rejected by the remote after the merge-in against parent branch \"main\"; the remote task branch is at " + remoteOnly +
		" and holds 1 commit(s) the local branch lacks; way forward: run `git merge origin/task-branch` in the task worktree, then resume the run with `lyx loom start`"
	if ptr.Reason != wantReason {
		t.Errorf("stuck reason = %q; want %q", ptr.Reason, wantReason)
	}

	gitkit.Git(t, taskWorktree, "merge", "--no-edit", "origin/task-branch")
	server := newPublishIntegrationGitHubServer(t, taskWorktree)
	server.install(t)

	shedfake.RequireOutcome(t, p, shedengine.Done)
	if !server.createChecked {
		t.Error("the create call never landed; want the resumed Publish to push and reach the GitHub step")
	}
}
