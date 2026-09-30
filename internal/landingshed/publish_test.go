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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// captureLogOutput redirects logger output into a buffer for the duration of
// one test, restoring os.Stderr via t.Cleanup -- the test-log-capture-pattern
// shared decision's inline shape, modeled on
// internal/loomshed/gatefindings_test.go.
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

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
// requiring a pull request, a told DescriptionPath pointing at a directory with no summary.md yet,
// and a scratch dir under t.TempDir().
func newTestDeps(t *testing.T) Deps {
	t.Helper()
	return Deps{
		WorktreeRoot:    t.TempDir(),
		TaskBranch:      "task-branch",
		ParentBranch:    "main",
		DescriptionPath: summaryparser.Path(t.TempDir()),
		StencilsDir:     t.TempDir(),
		ScratchDir:      filepath.Join(t.TempDir(), "scratch"),
		OriginURL:       "https://github.com/acme/proj.git",
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

// requireReason returns the stuck reason carried on ptr, failing the test when it is empty.
func requireReason(t *testing.T, ptr shedengine.OutputPointer) string {
	t.Helper()
	if ptr.Reason == "" {
		t.Fatal("OutputPointer.Reason is empty; want the producer's stuck reason")
	}
	return ptr.Reason
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

func TestNewPublish_RejectsEmptyDescriptionPath(t *testing.T) {
	deps := newTestDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	deps.PushBranch = func() error { return nil }
	deps.DescriptionPath = ""
	if _, err := NewPublish(deps); err == nil {
		t.Fatal("NewPublish() error = nil; want an error naming Deps.DescriptionPath")
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

	outcome, ptr, err := p.Call(context.Background())
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
	requireReason(t, ptr)
}

func TestPublish_MergeInStuck(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "merge-in could not be resolved"}}
	p := &Publish{deps: deps, resolver: res}

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if res.gotSource != deps.ParentBranch {
		t.Errorf("resolver.Resolve source = %q; want %q", res.gotSource, deps.ParentBranch)
	}
	got := requireReason(t, ptr)
	if got != "merge-in could not be resolved" {
		t.Errorf("reason = %q; want the resolver's own reason", got)
	}
}

func TestPublish_PushFails_NoGitHubCall(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return errors.New("boom") }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	requireReason(t, ptr)
}

func TestPublish_PushRejected_DistinctReason(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return gitrepo.ErrPushRejected }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}
	rejectedReason := runAndGetReason(t, p)

	deps2 := newTestDeps(t)
	deps2.PushBranch = func() error { return errors.New("generic failure") }
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p2 := &Publish{deps: deps2, resolver: res2}
	genericReason := runAndGetReason(t, p2)

	if rejectedReason == genericReason {
		t.Errorf("rejected-push reason %q equals generic-push-failure reason %q; want distinct", rejectedReason, genericReason)
	}
}

func runAndGetReason(t *testing.T, p *Publish) string {
	t.Helper()
	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	return requireReason(t, ptr)
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

			outcome, ptr, err := p.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
			}
			requireReason(t, ptr)
		})
	}
}

// TestPublish_GitHubClientUnavailable_WarnsWithActionAndCause covers the NewGitHubClient factory
// failure site: Call must still reach its usual stuck verdict, and the site's own logger.Warn line
// must carry the action and cause field keys.
func TestPublish_GitHubClientUnavailable_WarnsWithActionAndCause(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	installFailingGitHubClientFactory(t, errors.New("boom"))
	buf := captureLogOutput(t)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	requireReason(t, ptr)

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output = %q; want a WARN line", logged)
	}
	for _, field := range []string{"action=", "cause="} {
		if !strings.Contains(logged, field) {
			t.Errorf("log output = %q; want a %s field", logged, field)
		}
	}
}

// TestPublish_QueryExistingPRFails_WarnsWithActionOwnerRepoAndCause covers the
// client.PullRequests.List failure site: Call must still reach its usual stuck verdict, driven by
// publishGitHubErrorReason, and the site's own logger.Warn line must carry action, owner, repo, and
// cause -- the reason-classification behaviour itself stays covered by
// TestPublishGitHubErrorReason_ClassifiesDistinctly, unchanged here.
func TestPublish_QueryExistingPRFails_WarnsWithActionOwnerRepoAndCause(t *testing.T) {
	deps := newTestDeps(t)
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.listStatus = http.StatusInternalServerError
	srv.listBody = `{"message":"server exploded"}`
	srv.install(t)
	buf := captureLogOutput(t)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	requireReason(t, ptr)

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output = %q; want a WARN line", logged)
	}
	for _, field := range []string{"action=", "owner=", "repo=", "cause="} {
		if !strings.Contains(logged, field) {
			t.Errorf("log output = %q; want a %s field", logged, field)
		}
	}
}

// TestPublish_CreatePRFails_WarnsWithActionOwnerRepoAndCause covers the
// client.PullRequests.Create failure site: Call must still reach its usual stuck verdict, and the
// site's own logger.Warn line must carry action, owner, repo, and cause.
func TestPublish_CreatePRFails_WarnsWithActionOwnerRepoAndCause(t *testing.T) {
	deps := newTestDeps(t)
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.createStatus = http.StatusInternalServerError
	srv.createBody = `{"message":"server exploded"}`
	srv.install(t)
	buf := captureLogOutput(t)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	requireReason(t, ptr)

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output = %q; want a WARN line", logged)
	}
	for _, field := range []string{"action=", "owner=", "repo=", "cause="} {
		if !strings.Contains(logged, field) {
			t.Errorf("log output = %q; want a %s field", logged, field)
		}
	}
}

func TestPublish_NoExistingPR_CreatesAndReportsStuck(t *testing.T) {
	deps := newTestDeps(t)
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	outcome, ptr, err := p.Call(context.Background())
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
	requireReason(t, ptr)
}

func TestPublish_OpenPR_StuckNoCreate(t *testing.T) {
	deps := newTestDeps(t)
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	// No "merged"/"merged_at" key at all: the real List Pull Requests endpoint never sets "merged"
	// on an open PR and reports "merged_at": null (crucible round 3, F-R3-1) -- an absent key decodes
	// to the same nil field either way.
	srv.listBody = `[{"number":7,"state":"open"}]`
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

// TestPublish_ClosedAndMergedPR_Done pins the fix for crucible round 3's F-R3-1: GitHub's List Pull
// Requests endpoint never populates the "merged" boolean on any item it returns (confirmed live
// against a real merged PR -- `gh api ".../pulls?state=all&..."` reports "merged":null even for a
// genuinely merged PR, while only the single-PR Get endpoint reports "merged":true), so Publish must
// read "merged_at" instead. This mock's shape is deliberately the REAL List response shape -- no
// "merged" key at all, only "merged_at" -- specifically so this test cannot again pass against a
// mock shape the real API would never produce.
func TestPublish_ClosedAndMergedPR_Done(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	var order []string
	srv := newPublishGitHubServer(t, &order)
	srv.listBody = `[{"number":7,"state":"closed","merged_at":"2026-09-13T16:49:32Z"}]`
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
	// No "merged_at": a genuinely closed-and-not-merged PR carries neither "merged" nor "merged_at"
	// on the real List endpoint.
	srv.listBody = `[{"number":7,"state":"closed"}]`
	srv.install(t)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	closedUnmergedReason := requireReason(t, ptr)

	// Re-run against an open PR and compare the returned reasons.
	deps2 := newTestDeps(t)
	deps2.PushBranch = func() error { return nil }
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p2 := &Publish{deps: deps2, resolver: res2}
	var order2 []string
	srv2 := newPublishGitHubServer(t, &order2)
	srv2.listBody = `[{"number":8,"state":"open"}]`
	srv2.install(t)
	_, ptr2, err := p2.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	openReason := requireReason(t, ptr2)

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
		t.Fatal("Call() error = nil; want an error for a missing change description")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a hard error", outcome)
	}
	if len(srv.createdBodies) != 0 {
		t.Errorf("created pull requests = %d; want 0 on a missing summary", len(srv.createdBodies))
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

// TestPublish_CreatedPRLogsInfo asserts the one GitHub write lands in the durable trace exactly
// once, carrying the created pull request's number. It never runs in parallel: the sink override
// is package-level state.
func TestPublish_CreatedPRLogsInfo(t *testing.T) {
	logger.SetDurableSinkDir(t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	deps := newTestDeps(t)
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	data, err := os.ReadFile(logger.TraceFile())
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	var created []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, `msg="landingshed: pull request created"`) {
			created = append(created, line)
		}
	}
	if len(created) != 1 {
		t.Fatalf("pull request created records = %d; want 1\n%s", len(created), data)
	}
	if !strings.Contains(created[0], "number=1") {
		t.Errorf("record = %q; want number=1", created[0])
	}
}

// publishReasonFor runs one Publish against a fake GitHub server with the given list and create
// bodies and returns the stuck reason it returned.
func publishReasonFor(t *testing.T, listBody, createBody string) string {
	t.Helper()
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return nil }
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	var order []string
	srv := newPublishGitHubServer(t, &order)
	srv.listBody = listBody
	if createBody != "" {
		srv.createBody = createBody
	}
	srv.install(t)
	return runAndGetReason(t, p)
}

func TestPublish_PRStateReasons_EndWithURL(t *testing.T) {
	const url = "https://github.com/owner/repo/pull/7"
	tests := []struct {
		name       string
		listBody   string
		createBody string
		want       string
	}{
		{"created", "[]", `{"number":7,"state":"open","html_url":"` + url + `"}`, "pull request created; awaiting review, then run `lyx loom approve` and `lyx loom start`: " + url},
		{"already open", `[{"number":7,"state":"open","html_url":"` + url + `"}]`, "", "an open pull request already exists against parent branch \"main\"; run `lyx loom approve` and `lyx loom start` once it is reviewed: " + url},
		{"closed unmerged", `[{"number":7,"state":"closed","html_url":"` + url + `"}]`, "", "the pull request was closed without being merged: " + url},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publishReasonFor(t, tt.listBody, tt.createBody)
			if got != tt.want {
				t.Errorf("reason = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestPublish_PRStateReason_NoURL_IsBareText(t *testing.T) {
	got := publishReasonFor(t, "[]", `{"number":7,"state":"open"}`)
	if got != "pull request created; awaiting review, then run `lyx loom approve` and `lyx loom start`" {
		t.Errorf("reason = %q; want the bare text with no suffix", got)
	}
}

func TestPublish_StuckWritesNoReasonFile(t *testing.T) {
	deps := newTestDeps(t)
	deps.PushBranch = func() error { return errors.New("boom") }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}
	runAndGetReason(t, p)

	matches, err := filepath.Glob(filepath.Join(deps.ScratchDir, "*-stuck.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("stuck-reason files = %v; want none", matches)
	}
}

func TestFindPullRequest_QueryParametersAndEmptyList(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed

	pr, err := FindPullRequest(context.Background(), client, "o", "r", "task", "parent")
	if err != nil {
		t.Fatalf("FindPullRequest: %v", err)
	}
	if pr != nil {
		t.Errorf("pr = %v, want nil for an empty list", pr)
	}
	want := map[string]string{"state": "all", "head": "o:task", "base": "parent", "sort": "created", "direction": "desc"}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), v)
		}
	}
}

// approvalFixture is a Publish over a written approval record, a fake GitHub server answering the
// list query with listBody, and a resolver and push that record whether they were called.
type approvalFixture struct {
	p        *Publish
	res      *recordingResolver
	order    []string
	taskHead string
}

func newApprovalFixture(t *testing.T, listBody string, approval *Approval, taskHeadErr error) *approvalFixture {
	t.Helper()
	fx := &approvalFixture{taskHead: "aaa"}
	deps := newTestDeps(t)
	deps.ApprovalPath = filepath.Join(t.TempDir(), "approval.json")
	if approval != nil {
		if err := WriteApproval(deps.ApprovalPath, *approval); err != nil {
			t.Fatalf("WriteApproval: %v", err)
		}
	}
	deps.TaskHead = func() (string, error) { return fx.taskHead, taskHeadErr }
	deps.PushBranch = func() error { fx.order = append(fx.order, "push"); return nil }
	fx.res = &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fx.p = &Publish{deps: deps, resolver: fx.res}
	srv := newPublishGitHubServer(t, &fx.order)
	srv.listBody = listBody
	srv.install(t)
	return fx
}

func TestPublish_MatchingApproval_DoneWithoutSync(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"open","head":{"sha":"aaa"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	outcome, _, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
	if fx.res.called {
		t.Error("resolver was called; want no merge-in")
	}
	for _, o := range fx.order {
		if o == "push" {
			t.Error("push was called; want none")
		}
	}
}

// TestPublish_NilTaskHead_ApprovalIgnored pins the nil-is-absent seam: without TaskHead the record
// is not consulted, so a matching approval takes the ordinary open-PR path instead.
func TestPublish_NilTaskHead_ApprovalIgnored(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"open","head":{"sha":"aaa"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	fx.p.deps.TaskHead = nil
	outcome, ptr, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	if !strings.Contains(ptr.Reason, "already exists") {
		t.Errorf("reason %q; want the ordinary already-open reason", ptr.Reason)
	}
	if !fx.res.called {
		t.Error("resolver was not called; want the ordinary merge-in")
	}
}

func TestPublish_MismatchedApproval_StuckNamesSHAs(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"open","head":{"sha":"bbb"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	outcome, ptr, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	for _, want := range []string{"aaa", "bbb", "lyx loom approve"} {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("reason %q lacks %q", ptr.Reason, want)
		}
	}
	if fx.res.called {
		t.Error("resolver was called; want no merge-in")
	}
}

func TestPublish_ApprovalOverMergedPR_Done(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"sha":"zzz"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	outcome, _, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
}

func TestPublish_ApprovalOverClosedPR_ClosedStuck(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"closed","head":{"sha":"aaa"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, nil)
	outcome, ptr, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	if !strings.Contains(ptr.Reason, "closed without being merged") {
		t.Errorf("reason = %q; want the closed reason", ptr.Reason)
	}
}

func TestPublish_NoApproval_MergedPRStillDone(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"closed","merged_at":"2026-01-01T00:00:00Z"}]`, nil, nil)
	outcome, _, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
	}
}

func TestPublish_MalformedApproval_StuckNamesFile(t *testing.T) {
	fx := newApprovalFixture(t, "[]", nil, nil)
	if err := os.WriteFile(fx.p.deps.ApprovalPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome, ptr, err := fx.p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
	}
	if !strings.Contains(ptr.Reason, fx.p.deps.ApprovalPath) || !strings.Contains(ptr.Reason, "lyx loom approve") {
		t.Errorf("reason = %q; want the file and the verb named", ptr.Reason)
	}
}

func TestPublish_TaskHeadError_ReturnedError(t *testing.T) {
	fx := newApprovalFixture(t, `[{"number":7,"state":"open","head":{"sha":"aaa"}}]`, &Approval{PRNumber: 7, HeadSHA: "aaa", ApprovedAt: "2026-01-01T00:00:00Z"}, errors.New("git broke"))
	if _, _, err := fx.p.Call(context.Background()); err == nil {
		t.Fatal("Call() error = nil; want the TaskHead error")
	}
}
