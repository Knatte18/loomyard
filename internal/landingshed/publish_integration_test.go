//go:build integration

// publish_integration_test.go drives Publish's merge-in half against a real pair built by the
// standard hub fixture (internal/hubforge), with only the GitHub client faked -- an httptest server
// standing in for the real service, mirroring the in-package unit tier's own publishGitHubServer.
// No conflict is staged in this scenario: the merge-in half is the point, and the byte-identical
// warp/weft conflict shape is already covered exhaustively by internal/fabricengine's own
// mergein_integration_test.go and this file's own sibling (finalize_integration_test.go). No test
// in this file contacts a real service or a real model.

package landingshed_test

import (
	"context"
	"fmt"
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
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// failingShuttle fails the test outright if Run is ever called -- this scenario stages no conflict,
// so the real conflict-resolution session must never spawn.
type failingShuttle struct{ t *testing.T }

func (f failingShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	f.t.Fatal("fake shuttle Run() called; want the clean merge-in to need no conflict-resolution session")
	return shuttleengine.Result{}, nil
}

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

// writeSummaryLanding writes a well-formed summary artifact at path -- this package's own local copy
// of the in-package unit tier's identically-shaped helper.
func writeSummaryLanding(t *testing.T, path, title, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf("# %s\n\n%s\n", title, body)), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// TestPublish_MergesInCleanlyBeforeCreatingPullRequest drives Publish against a real pair: the task
// worktree catches up with the parent branch (a clean, non-conflicting merge-in), pushes (a no-op
// closure -- push mechanics are internal/gitrepo's own tier's job), and only then queries and creates
// against the faked GitHub client, which itself asserts the pair is clean and carries the parent
// branch's own content before it ever answers the create call.
func TestPublish_MergesInCleanlyBeforeCreatingPullRequest(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	taskWorktree := h.PrimeWorktree()

	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "-b", "task-branch")
	// Advance main (the parent branch) with a clean commit while the task branch stays behind, so
	// the merge-in step below has genuine, non-conflicting content to bring in.
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "main")
	commitOnCurrentBranchLanding(t, taskWorktree, "parent-progress.txt", "parent progress\n", "main: progress")
	gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "task-branch")

	finalSummaryPath := summaryparser.Path(t.TempDir())
	writeSummaryLanding(t, finalSummaryPath, "Task summary", "Task body.")

	server := newPublishIntegrationGitHubServer(t, taskWorktree)
	server.install(t)

	var pushed bool
	deps := landingshed.Deps{
		WorktreeRoot:     taskWorktree,
		TaskBranch:       "task-branch",
		ParentBranch:     "main",
		FinalSummaryPath: finalSummaryPath,
		StencilsDir:      t.TempDir(),
		ScratchDir:       filepath.Join(t.TempDir(), "scratch"),
		OriginURL:        "https://github.com/acme/proj.git",
		PushBranch:       func() error { pushed = true; return nil },
		OpenFabric:       func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskWorktree), nil },
		Shuttle:          failingShuttle{t: t},
		Config: landingshed.Config{
			RequirePRToBase:    []string{"main"},
			Conflict:           "claude:test-model",
			ConflictTimeoutMin: 1,
		},
	}

	p, err := landingshed.NewPublish(deps)
	if err != nil {
		t.Fatalf("NewPublish() error = %v; want nil", err)
	}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	// Stuck, not Done: a pull request awaiting human review is the correct terminal verdict right
	// after a successful create call, per Publish's own design.
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q (pull request created; awaiting review)", outcome, shedengine.Stuck)
	}
	if !pushed {
		t.Error("push closure never called; want the task branch pushed before the create call")
	}
	if !server.createChecked {
		t.Error("the create call never landed; want the clean-and-current assertion to have run")
	}
}
