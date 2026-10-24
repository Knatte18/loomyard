//go:build tier0

// hubissues_test.go covers Repo's calls against httptest servers, so no test reaches real token resolution or the network.

package hubissues_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/hubissues"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

const (
	repoOwner  = "owner"
	repoName   = "hub-repo"
	issuesPath = "/repos/owner/hub-repo/issues"
)

// Top-level constants so the untagged-test sleep guard can resolve them.
const (
	pageDelay = 250 * time.Millisecond
	pauseHold = 150 * time.Millisecond
)

// requestCapture is one request a test server received.
type requestCapture struct {
	method string
	path   string
	body   map[string]any
}

// captureLog collects the requests a test server receives, safe for the server's goroutines.
type captureLog struct {
	mu       sync.Mutex
	requests []requestCapture
}

func (c *captureLog) add(req requestCapture) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
}

func (c *captureLog) all() []requestCapture {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]requestCapture{}, c.requests...)
}

// newRepo returns the test repository reached through a fixed-token client pointed at server, so no token is resolved.
func newRepo(t *testing.T, server *httptest.Server) *hubissues.Repo {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%s): %v", server.URL, err)
	}
	client.BaseURL = parsed
	return hubissues.New(client, repoOwner, repoName)
}

// newIssueServer returns a server that answers every request with status and respBody, recording each request in log.
func newIssueServer(t *testing.T, status int, respBody string, log *captureLog) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		log.add(requestCapture{method: r.Method, path: r.URL.Path, body: decoded})
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = w.Write([]byte(respBody))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestOpen_RefusesNonGitHubURL(t *testing.T) {
	t.Parallel()
	const local = "/srv/git/hub-repo.git"

	repo, err := hubissues.Open(local)

	if err == nil || !strings.Contains(err.Error(), local) {
		t.Errorf("Open(%q) = %v, %v; want an error naming the URL", local, repo, err)
	}
}

// TestListOpenIssues_PaginatesAndExcludesPullRequests follows the Link header across two pages, drops the pull request, and asserts the repository's own path and the open filter.
func TestListOpenIssues_PaginatesAndExcludesPullRequests(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":3,"title":"three","state":"open","labels":[]}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, server.URL, issuesPath))
		_, _ = w.Write([]byte(`[
			{"number":1,"title":"one","body":"b","html_url":"https://x/1","state":"open","labels":[{"name":"bug"}],"created_at":"2026-01-02T03:04:05Z","comments":4},
			{"number":2,"title":"pr","state":"open","pull_request":{"url":"https://x/pr"}}
		]`))
	}))
	t.Cleanup(server.Close)

	got, err := newRepo(t, server).ListOpenIssues()
	if err != nil {
		t.Fatalf("ListOpenIssues() error = %v; want nil", err)
	}

	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 3 {
		t.Fatalf("ListOpenIssues() = %+v; want issues 1 and 3 with the pull request excluded", got)
	}
	if got[0].Title != "one" || got[0].Body != "b" || got[0].URL != "https://x/1" ||
		len(got[0].Labels) != 1 || got[0].Labels[0] != "bug" || got[0].CreatedAt.Year() != 2026 || got[0].Comments != 4 {
		t.Errorf("ListOpenIssues()[0] = %+v; want its fields carried over, comments 4 included", got[0])
	}
	if got[1].Comments != 0 {
		t.Errorf("ListOpenIssues()[1].Comments = %d; want 0 for an issue whose response omits the count", got[1].Comments)
	}
	if len(paths) != 2 {
		t.Fatalf("request count = %d; want 2 pages", len(paths))
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, issuesPath+"?") || !strings.Contains(p, "state=open") {
			t.Errorf("request = %q; want the %s path with state=open", p, issuesPath)
		}
	}
}

// TestListOpenIssues_PausedPageGetsItsOwnDeadline holds the second page under a hub pause and checks the held page still completes inside its own deadline.
// It sets environment variables and installs the process-wide pause gate, so it does not run in parallel.
func TestListOpenIssues_PausedPageGetsItsOwnDeadline(t *testing.T) {
	const perRequest = 400 * time.Millisecond
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")

	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	setLive := func(live bool) {
		if _, err := pauserecord.Update(recordPath, time.Now(), func(r *pauserecord.Record) error {
			r.Live = nil
			if live {
				r.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: time.Now()}}
			}
			return nil
		}); err != nil {
			t.Errorf("update the pause record: %v", err)
		}
	}
	var sleeps atomic.Int32
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
		Geometry: pauserecord.Geometry{RecordPath: recordPath},
		Notify:   func(string) {},
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleeps.Add(1)
			time.Sleep(pauseHold)
			setLive(false)
			return nil
		},
	}))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			time.Sleep(pageDelay)
			_, _ = w.Write([]byte(`[{"number":3,"title":"three","state":"open","labels":[]}]`))
			return
		}
		setLive(true)
		time.Sleep(pageDelay)
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, server.URL, issuesPath))
		_, _ = w.Write([]byte(`[{"number":1,"title":"one","state":"open","labels":[]}]`))
	}))
	t.Cleanup(server.Close)
	client, err := githubclient.NewWithBaseURL(server.URL, nil)
	if err != nil {
		t.Fatalf("NewWithBaseURL(%s): %v", server.URL, err)
	}
	repo := hubissues.New(client, repoOwner, repoName)
	repo.SetPerRequest(perRequest)

	got, err := repo.ListOpenIssues()
	if err != nil {
		t.Fatalf("ListOpenIssues() with a %v deadline error = %v; want nil", perRequest, err)
	}
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 3 {
		t.Fatalf("ListOpenIssues() = %+v; want issues 1 and 3", got)
	}
	if sleeps.Load() != 1 {
		t.Errorf("gate waits = %d; want the second page held once", sleeps.Load())
	}
}

// TestListIssueComments_PaginatesOldestFirst follows the Link header across two pages, asserts the comments path with the created-ascending sort, and carries bodies and creation times over in order;
// a rejected response surfaces its message.
func TestListIssueComments_PaginatesOldestFirst(t *testing.T) {
	t.Parallel()
	t.Run("TwoPagesInOrder", func(t *testing.T) {
		t.Parallel()
		commentsPath := issuesPath + "/5/comments"
		var mu sync.Mutex
		var queries []string
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != commentsPath {
				t.Errorf("request path = %q; want %q", r.URL.Path, commentsPath)
			}
			mu.Lock()
			queries = append(queries, r.URL.RawQuery)
			mu.Unlock()
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`[{"body":"third","created_at":"2026-03-01T00:00:00Z"}]`))
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, server.URL, commentsPath))
			_, _ = w.Write([]byte(`[
				{"body":"first","created_at":"2026-01-01T00:00:00Z","user":{"login":"a"}},
				{"body":"second","created_at":"2026-02-01T00:00:00Z","user":{"login":"b"}}
			]`))
		}))
		t.Cleanup(server.Close)

		got, err := newRepo(t, server).ListIssueComments(5)
		if err != nil {
			t.Fatalf("ListIssueComments() error = %v; want nil", err)
		}

		want := []hubissues.IssueComment{
			{Body: "first", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Body: "second", CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			{Body: "third", CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		}
		if len(got) != len(want) {
			t.Fatalf("ListIssueComments() = %+v; want %+v", got, want)
		}
		for i := range want {
			if got[i].Body != want[i].Body || !got[i].CreatedAt.Equal(want[i].CreatedAt) {
				t.Errorf("ListIssueComments()[%d] = %+v; want %+v", i, got[i], want[i])
			}
		}
		if len(queries) != 2 {
			t.Fatalf("request count = %d; want 2 pages", len(queries))
		}
		if !strings.Contains(queries[0], "sort=created") || !strings.Contains(queries[0], "direction=asc") {
			t.Errorf("first query = %q; want sort=created and direction=asc", queries[0])
		}
	})

	t.Run("RejectedResponse", func(t *testing.T) {
		t.Parallel()
		server := newIssueServer(t, http.StatusNotFound, `{"message":"Not Found thing"}`, &captureLog{})

		_, err := newRepo(t, server).ListIssueComments(5)

		if err == nil || !strings.Contains(err.Error(), "Not Found thing") || !strings.Contains(err.Error(), "owner/hub-repo") {
			t.Errorf("ListIssueComments() error = %v; want the response message and the repository", err)
		}
	})
}

// TestGetIssue_ReportsPullRequestAndClosedState checks that GetIssue returns a pull request and a closed issue without excluding either.
func TestGetIssue_ReportsPullRequestAndClosedState(t *testing.T) {
	t.Parallel()
	log := &captureLog{}
	server := newIssueServer(t, http.StatusOK,
		`{"number":7,"title":"t","state":"closed","state_reason":"not_planned","pull_request":{"url":"https://x/pr"}}`, log)

	got, err := newRepo(t, server).GetIssue(7)
	if err != nil {
		t.Fatalf("GetIssue() error = %v; want nil", err)
	}

	if got.Number != 7 || got.State != "closed" || got.StateReason != "not_planned" || !got.PullRequest {
		t.Errorf("GetIssue() = %+v; want issue 7, closed, not_planned, a pull request", got)
	}
	captured := log.all()
	if len(captured) != 1 || captured[0].method != http.MethodGet || captured[0].path != issuesPath+"/7" {
		t.Errorf("requests = %+v; want one GET %s/7", captured, issuesPath)
	}
}

// TestCommentAndClose_SendsCommentThenClose drives both state reasons and checks the order and payloads of the two requests.
func TestCommentAndClose_SendsCommentThenClose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		completed bool
		reason    string
	}{{true, "completed"}, {false, "not_planned"}}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			t.Parallel()
			log := &captureLog{}
			server := newIssueServer(t, http.StatusOK, `{"number":9}`, log)

			if err := newRepo(t, server).CommentAndClose(9, "folded into x", tt.completed); err != nil {
				t.Fatalf("CommentAndClose() error = %v; want nil", err)
			}

			captured := log.all()
			if len(captured) != 2 {
				t.Fatalf("request count = %d; want 2", len(captured))
			}
			if captured[0].method != http.MethodPost || captured[0].path != issuesPath+"/9/comments" {
				t.Errorf("first request = %s %s; want POST %s/9/comments", captured[0].method, captured[0].path, issuesPath)
			}
			if body, _ := captured[0].body["body"].(string); body != "folded into x" {
				t.Errorf("comment body = %q; want %q", body, "folded into x")
			}
			if captured[1].method != http.MethodPatch || captured[1].path != issuesPath+"/9" {
				t.Errorf("second request = %s %s; want PATCH %s/9", captured[1].method, captured[1].path, issuesPath)
			}
			if captured[1].body["state"] != "closed" || captured[1].body["state_reason"] != tt.reason {
				t.Errorf("close body = %v; want state closed with reason %s", captured[1].body, tt.reason)
			}
		})
	}
}

// TestCommentAndClose_FailedCommentSkipsClose checks that a rejected comment never reaches the close.
func TestCommentAndClose_FailedCommentSkipsClose(t *testing.T) {
	t.Parallel()
	log := &captureLog{}
	server := newIssueServer(t, http.StatusForbidden, `{"message":"Forbidden thing"}`, log)

	err := newRepo(t, server).CommentAndClose(9, "c", true)

	if err == nil || !strings.Contains(err.Error(), "Forbidden thing") {
		t.Fatalf("CommentAndClose() error = %v; want the response message", err)
	}
	if got := len(log.all()); got != 1 {
		t.Errorf("request count = %d; want 1 (no close after a failed comment)", got)
	}
}

// TestCommentAndClose_FailedCloseNamesPostedComment checks that a close failing after a posted comment says so.
func TestCommentAndClose_FailedCloseNamesPostedComment(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		count.Add(1)
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"cannot close"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	t.Cleanup(server.Close)

	err := newRepo(t, server).CommentAndClose(9, "c", false)

	if err == nil {
		t.Fatal("CommentAndClose() error = nil; want the close failure")
	}
	for _, want := range []string{"comment posted", "second comment", "cannot close"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CommentAndClose() error = %q; want it to contain %q", err.Error(), want)
		}
	}
	if got := count.Load(); got != 2 {
		t.Errorf("request count = %d; want comment then close", got)
	}
}

// failingTransport fails every request with err, standing in for githubclient's transport when no token resolves.
type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

// TestCalls_TokenNotResolvable checks that every call surfaces an unresolvable token as such.
func TestCalls_TokenNotResolvable(t *testing.T) {
	t.Parallel()
	client := github.NewClient(&http.Client{Transport: failingTransport{err: githubclient.ErrTokenUnresolvable}})
	repo := hubissues.New(client, repoOwner, repoName)

	_, listErr := repo.ListOpenIssues()
	_, getErr := repo.GetIssue(1)
	_, commentsErr := repo.ListIssueComments(1)
	closeErr := repo.CommentAndClose(1, "c", true)

	for name, err := range map[string]error{"ListOpenIssues": listErr, "GetIssue": getErr, "ListIssueComments": commentsErr, "CommentAndClose": closeErr} {
		if !errors.Is(err, githubclient.ErrTokenUnresolvable) || !strings.Contains(err.Error(), "github token not resolvable") {
			t.Errorf("%s() error = %v; want the token-not-resolvable error", name, err)
		}
	}
}
