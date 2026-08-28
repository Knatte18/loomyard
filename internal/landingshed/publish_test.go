// publish_test.go covers Publish against a faked resolver, a faked push closure recording its call
// order, and a faked GitHub client swapped in through the NewGitHubClient seam pointed at a local
// httptest server -- exactly the way internal/selfreportengine's own test does it. No test contacts
// a real service or a real model.
//
// This tier lives in the package itself rather than an external test package, which is what lets it
// substitute the unexported resolver seam directly by constructing a Publish literal with a
// recordingResolver in its resolver field -- bypassing NewPublish's own resolver construction, which
// always builds a real *mergeresolve.Resolver. On that path the told session-runner value (Shuttle)
// is never driven, since the resolver's own behaviour is covered by its own tier in batch 3.

package landingshed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// recordingResolver is the in-package fake standing in for the unexported resolver seam: it records
// whether Resolve was called and what source it was called with, and returns a scripted result/err.
type recordingResolver struct {
	called    bool
	gotSource string
	result    mergeresolve.Result
	err       error
}

func (r *recordingResolver) Resolve(ctx context.Context, source string) (mergeresolve.Result, error) {
	r.called = true
	r.gotSource = source
	return r.result, r.err
}

// newTestDeps returns a minimal Deps with sane defaults for a Publish test: a base-branch list
// requiring a pull request, a told FinalSummaryPath pointing at a directory with no summary.md yet,
// and a scratch dir under t.TempDir().
func newTestDeps(t *testing.T) Deps {
	t.Helper()
	return Deps{
		WorktreeRoot:     t.TempDir(),
		TaskBranch:       "task-branch",
		ParentBranch:     "main",
		FinalSummaryPath: summaryparser.Path(t.TempDir()),
		StencilsDir:      t.TempDir(),
		ScratchDir:       filepath.Join(t.TempDir(), "scratch"),
		OriginURL:        "https://github.com/acme/proj.git",
		Config: Config{
			RequirePRToBase:    []string{"main"},
			Squash:             true,
			Conflict:           "sonnet",
			ConflictTimeoutMin: 30,
		},
	}
}

// writeSummary writes a well-formed summary artifact at path.
func writeSummary(t *testing.T, path, title, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf("# %s\n\n%s\n", title, body)), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// publishGitHubServer is a scripted httptest server standing in for the GitHub API: it answers a
// pull-request list query and a pull-request create call, appending "list"/"create" to order (shared
// with the push closure's own "push" append) so a test can assert relative call ordering.
type publishGitHubServer struct {
	server *httptest.Server
	order  *[]string

	listStatus int
	listBody   string

	createStatus  int
	createBody    string
	createdBodies []map[string]any
}

func newPublishGitHubServer(t *testing.T, order *[]string) *publishGitHubServer {
	t.Helper()
	s := &publishGitHubServer{order: order, listStatus: http.StatusOK, listBody: "[]", createStatus: http.StatusCreated}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			*order = append(*order, "list")
			w.WriteHeader(s.listStatus)
			_, _ = w.Write([]byte(s.listBody))
		case http.MethodPost:
			*order = append(*order, "create")
			raw, _ := jsonDecodeBody(r)
			s.createdBodies = append(s.createdBodies, raw)
			w.WriteHeader(s.createStatus)
			_, _ = w.Write([]byte(s.createBody))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(s.server.Close)
	if s.createBody == "" {
		s.createBody = `{"number":1,"state":"open"}`
	}
	return s
}

func jsonDecodeBody(r *http.Request) (map[string]any, error) {
	var decoded map[string]any
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&decoded)
	return decoded, err
}

// install swaps NewGitHubClient with a closure returning a real go-github client pointed at s's
// server, restoring the original on test cleanup -- mirrors selfreportengine's installGitHubClient.
func (s *publishGitHubServer) install(t *testing.T) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(s.server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed

	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// installFailingGitHubClientFactory replaces NewGitHubClient with a closure that itself fails.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// readStuckFile reads the reason file Publish's stuck path writes for producer "Publish".
func readStuckFile(t *testing.T, scratchDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scratchDir, publishName+stuckFileSuffix))
	if err != nil {
		t.Fatalf("read stuck file: %v", err)
	}
	return string(data)
}

// --- NewPublish construction ---

func TestNewPublish_RejectsNilOpenFabric(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	if _, err := NewPublish(deps); err == nil {
		t.Fatal("NewPublish() error = nil; want an error naming Deps.OpenFabric")
	}
}

func TestNewPublish_RejectsNilPushBranch(t *testing.T) {
	deps := newTestDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	if _, err := NewPublish(deps); err == nil {
		t.Fatal("NewPublish() error = nil; want an error naming Deps.PushBranch")
	}
}

// TestNewPublish_RejectsNilShuttle asserts the constructor rejects a Deps whose told session-runner
// seam is nil, with a distinct error naming that field. deps.OpenFabric returns a typed-nil
// *fabricengine.Fabric: mergeresolve.New checks its Fabric field for a nil interface, which a
// typed-nil pointer does not satisfy, so this test reaches the Shuttle check without ever invoking a
// method on the fabric handle.
func TestNewPublish_RejectsNilShuttle(t *testing.T) {
	deps := newTestDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	deps.PushBranch = func() error { return nil }
	deps.Shuttle = nil

	_, err := NewPublish(deps)
	if err == nil {
		t.Fatal("NewPublish() error = nil; want an error naming Deps.Shuttle")
	}
}

// --- Call behaviour ---

func TestPublish_ParentBranchNotInBaseList_Done(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config.RequirePRToBase = []string{"other-base"}
	res := &recordingResolver{}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if res.called {
		t.Error("resolver.Resolve was called; want no merge-in when no pull request is required")
	}
}

func TestPublish_PushSkipped_NoPRRequired_Done(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config.RequirePRToBase = []string{"other-base"}
	deps.PushSkipped = true
	res := &recordingResolver{}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

func TestPublish_PushSkipped_PRRequired_StuckBeforeMergeInAndPush(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushSkipped = true
	pushed := false
	deps.PushBranch = func() error { pushed = true; return nil }
	res := &recordingResolver{}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if res.called {
		t.Error("resolver.Resolve was called; want push-skipped to refuse before merge-in")
	}
	if pushed {
		t.Error("PushBranch was called; want push-skipped to refuse before the push closure")
	}
	readStuckFile(t, deps.ScratchDir)
}

func TestPublish_MergeInStuck(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "merge-in could not be resolved"}}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if res.gotSource != deps.ParentBranch {
		t.Errorf("resolver.Resolve source = %q; want %q", res.gotSource, deps.ParentBranch)
	}
	got := readStuckFile(t, deps.ScratchDir)
	if got != "merge-in could not be resolved\n" {
		t.Errorf("stuck file = %q; want the resolver's own reason", got)
	}
}

func TestPublish_PushFails_NoGitHubCall(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return errors.New("boom") }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	readStuckFile(t, deps.ScratchDir)
}

func TestPublish_PushRejected_DistinctReason(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return gitrepo.ErrPushRejected }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}
	rejectedReason := runAndReadStuckFile(t, p)

	deps2 := newTestDeps(t)
	deps2.PushBranch = func() error { return errors.New("generic failure") }
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p2 := &Publish{deps: deps2, resolver: res2}
	genericReason := runAndReadStuckFile(t, p2)

	if rejectedReason == genericReason {
		t.Errorf("rejected-push reason %q equals generic-push-failure reason %q; want distinct", rejectedReason, genericReason)
	}
}

func runAndReadStuckFile(t *testing.T, p *Publish) string {
	t.Helper()
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	return readStuckFile(t, p.deps.ScratchDir)
}

func TestPublish_OriginURLUnusable_NoGitHubCall(t *testing.T) {
	tests := []string{"", "not-a-url", "https://gitlab.com/acme/proj.git"}
	for _, originURL := range tests {
		t.Run(originURL, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.OriginURL = originURL
			deps.PushBranch = func() error { return nil }
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			p := &Publish{deps: deps, resolver: res}

			outcome, _, err := p.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
			}
			readStuckFile(t, deps.ScratchDir)
		})
	}
}

func TestPublish_NoExistingPR_CreatesAndReportsStuck(t *testing.T) {
	deps := newTestDeps(t)
	writeSummary(t, deps.FinalSummaryPath, "My PR Title", "My PR body.")
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	wantOrder := []string{"push", "list", "create"}
	if len(order) != len(wantOrder) {
		t.Fatalf("call order = %v; want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Errorf("call order = %v; want %v", order, wantOrder)
		}
	}

	if len(srv.createdBodies) != 1 {
		t.Fatalf("created pull requests = %d; want 1", len(srv.createdBodies))
	}
	got := srv.createdBodies[0]
	if got["title"] != "My PR Title" {
		t.Errorf("created PR title = %v; want %q", got["title"], "My PR Title")
	}
	if got["body"] != "\nMy PR body.\n" {
		t.Errorf("created PR body = %v; want %q", got["body"], "\nMy PR body.\n")
	}
	readStuckFile(t, deps.ScratchDir)
}

func TestPublish_OpenPR_StuckNoCreate(t *testing.T) {
	deps := newTestDeps(t)
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.listBody = `[{"number":7,"state":"open","merged":false}]`
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(srv.createdBodies) != 0 {
		t.Errorf("created pull requests = %d; want 0 (an open PR already exists)", len(srv.createdBodies))
	}
}

func TestPublish_ClosedAndMergedPR_Done(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	var order []string
	srv := newPublishGitHubServer(t, &order)
	srv.listBody = `[{"number":7,"state":"closed","merged":true}]`
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

func TestPublish_ClosedAndUnmergedPR_StuckDistinctFromOpen(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	var order []string
	srv := newPublishGitHubServer(t, &order)
	srv.listBody = `[{"number":7,"state":"closed","merged":false}]`
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	closedUnmergedReason := readStuckFile(t, deps.ScratchDir)

	// Re-run against an open PR in a fresh scratch dir and compare reason-file contents.
	deps2 := newTestDeps(t)
	deps2.PushBranch = func() error { return nil }
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p2 := &Publish{deps: deps2, resolver: res2}
	var order2 []string
	srv2 := newPublishGitHubServer(t, &order2)
	srv2.listBody = `[{"number":8,"state":"open","merged":false}]`
	srv2.install(t)
	if _, _, err := p2.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	openReason := readStuckFile(t, deps2.ScratchDir)

	if closedUnmergedReason == openReason {
		t.Errorf("closed-unmerged reason %q equals open-PR reason %q; want distinct", closedUnmergedReason, openReason)
	}
}

func TestPublish_MissingSummary_FailsLoudlyNoCreate(t *testing.T) {
	deps := newTestDeps(t)
	// No summary.md written.
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want an error for a missing summary artifact")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a hard error", outcome)
	}
	if len(srv.createdBodies) != 0 {
		t.Errorf("created pull requests = %d; want 0 on a missing summary", len(srv.createdBodies))
	}
}

func TestPublish_ScratchDirAbsent_CreatedOnFirstStuckWrite(t *testing.T) {
	deps := newTestDeps(t)
	if _, err := os.Stat(deps.ScratchDir); !os.IsNotExist(err) {
		t.Fatalf("scratch dir already exists before the test: %v", err)
	}
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "some reason"}}
	p := &Publish{deps: deps, resolver: res}

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if _, err := os.Stat(deps.ScratchDir); err != nil {
		t.Errorf("scratch dir was not created by the first stuck write: %v", err)
	}
}

func TestPublish_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{}
	p := &Publish{deps: deps, resolver: res}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error on entry cancellation")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a cancelled entry", outcome)
	}
}

// --- publishGitHubErrorReason classification ---

func TestPublishGitHubErrorReason_ClassifiesDistinctly(t *testing.T) {
	tokenReason := publishGitHubErrorReason("query", githubclient.ErrTokenUnresolvable)
	apiReason := publishGitHubErrorReason("query", &github.ErrorResponse{Message: "nope"})
	networkReason := publishGitHubErrorReason("query", errors.New("connection refused"))

	if tokenReason == apiReason || tokenReason == networkReason || apiReason == networkReason {
		t.Errorf("classification reasons are not all distinct: token=%q api=%q network=%q", tokenReason, apiReason, networkReason)
	}
}
