//go:build tier0

// client_test.go covers Client's calls against one scripted httptest server, so no test reaches real token resolution or the network.

package pullrequest_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/pullrequest"
)

const pullsPath = "/repos/owner/repo/pulls"

// capturedRequest is one request the scripted server received.
type capturedRequest struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

// scriptedServer answers the pull request calls of one repository and records each request, safe for the server's goroutines.
type scriptedServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

// newScriptedServer starts the server and shuts it down at cleanup.
func newScriptedServer(t *testing.T) *scriptedServer {
	t.Helper()
	s := &scriptedServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// handle records the request and answers it from the script.
func (s *scriptedServer) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	s.mu.Lock()
	s.requests = append(s.requests, capturedRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query(), body: body})
	s.mu.Unlock()

	reply := func(status int, payload string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}
	key := r.Method + " " + r.URL.Path
	switch key {
	case "GET " + pullsPath:
		if r.URL.Query().Get("head") == "owner:task" {
			reply(200, `[{"number": 4, "html_url": "https://github.com/owner/repo/pull/4"}]`)
			return
		}
		reply(200, `[]`)
	case "POST " + pullsPath:
		reply(201, `{"number": 9, "html_url": "https://github.com/owner/repo/pull/9"}`)
	case "PATCH " + pullsPath + "/9", "PATCH " + pullsPath + "/5":
		reply(200, `{}`)
	case "POST /repos/owner/repo/issues/5/comments":
		reply(201, `{}`)
	case "GET " + pullsPath + "/1":
		reply(200, `{"state": "open", "head": {"sha": "h1"}}`)
	case "GET " + pullsPath + "/2":
		reply(200, `{"state": "closed", "merged": true, "head": {"sha": "h2"}}`)
	case "GET " + pullsPath + "/3":
		reply(200, `{"state": "closed", "head": {"sha": "h3"}}`)
	case "GET " + pullsPath + "/1/reviews":
		if r.URL.Query().Get("page") == "2" {
			reply(200, `[{"user": {"login": "b"}, "state": "CHANGES_REQUESTED", "commit_id": "h1"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s%s/1/reviews?page=2>; rel="next"`, s.URL, pullsPath))
		reply(200, `[{"user": {"login": "a"}, "state": "APPROVED", "commit_id": "h1"}]`)
	case "GET " + pullsPath + "/2/reviews", "GET " + pullsPath + "/3/reviews":
		reply(200, `[]`)
	case "PUT " + pullsPath + "/10/merge":
		reply(405, `{"message": "Squash merges are not allowed on this repository."}`)
	case "PUT " + pullsPath + "/11/merge":
		reply(405, `{"message": "Base branch was modified. Review and try the merge again."}`)
	case "PUT " + pullsPath + "/12/merge":
		reply(409, `{"message": "Head branch was modified. Review and try the merge again."}`)
	case "PUT " + pullsPath + "/13/merge":
		reply(409, `{"message": "At least 1 approving review is required by reviewers with write access."}`)
	case "PUT " + pullsPath + "/14/merge":
		reply(200, `{"merged": true, "sha": "m"}`)
	default:
		reply(404, `{"message": "Not Found"}`)
	}
}

// all returns a copy of the requests received so far.
func (s *scriptedServer) all() []capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]capturedRequest{}, s.requests...)
}

// last returns the most recent request received.
func (s *scriptedServer) last(t *testing.T) capturedRequest {
	t.Helper()
	requests := s.all()
	if len(requests) == 0 {
		t.Fatal("the server received no request")
	}
	return requests[len(requests)-1]
}

func TestClient_Calls(t *testing.T) {
	t.Parallel()
	server := newScriptedServer(t)
	gh := github.NewClient(nil).WithAuthToken("test-token")
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%s): %v", server.URL, err)
	}
	gh.BaseURL = baseURL
	client := pullrequest.NewWithClient(gh, "owner", "repo")
	ctx := t.Context()

	t.Run("FindOpen", func(t *testing.T) {
		ref, found, err := client.FindOpen(ctx, "task", "main")
		if err != nil || !found || ref != (pullrequest.Ref{Number: 4, URL: "https://github.com/owner/repo/pull/4"}) {
			t.Errorf("FindOpen(task, main) = %+v, %v, %v; want pull request 4, true, nil", ref, found, err)
		}
		if query := server.last(t).query; query.Get("head") != "owner:task" || query.Get("base") != "main" || query.Get("state") != "open" {
			t.Errorf("FindOpen sent query %v; want head owner:task, base main, state open", query)
		}
		if _, found, err := client.FindOpen(ctx, "other", "main"); err != nil || found {
			t.Errorf("FindOpen(other, main) = found %v, err %v; want none", found, err)
		}
	})

	t.Run("CreateAndEdit", func(t *testing.T) {
		ref, err := client.Create(ctx, "task", "main", "title", "body")
		if err != nil || ref.Number != 9 {
			t.Fatalf("Create() = %+v, %v; want pull request 9", ref, err)
		}
		sent := server.last(t)
		want := map[string]any{"title": "title", "head": "task", "base": "main", "body": "body"}
		if sent.method != "POST" || sent.path != pullsPath || !reflect.DeepEqual(sent.body, want) {
			t.Errorf("Create sent %s %s %v; want POST %s %v", sent.method, sent.path, sent.body, pullsPath, want)
		}
		if err := client.Edit(ctx, 9, "new title", "new body"); err != nil {
			t.Fatalf("Edit() error: %v", err)
		}
		sent = server.last(t)
		want = map[string]any{"title": "new title", "body": "new body"}
		if sent.method != "PATCH" || sent.path != pullsPath+"/9" || !reflect.DeepEqual(sent.body, want) {
			t.Errorf("Edit sent %s %s %v; want PATCH %s/9 %v", sent.method, sent.path, sent.body, pullsPath, want)
		}
	})

	t.Run("CloseCommentsBeforeClosing", func(t *testing.T) {
		before := len(server.all())
		if err := client.Close(ctx, 5, "closing"); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
		sent := server.all()[before:]
		if len(sent) != 2 {
			t.Fatalf("Close sent %d requests; want a comment then a close", len(sent))
		}
		if sent[0].method != "POST" || sent[0].path != "/repos/owner/repo/issues/5/comments" || sent[0].body["body"] != "closing" {
			t.Errorf("Close's first request = %s %s %v; want the comment POST", sent[0].method, sent[0].path, sent[0].body)
		}
		if sent[1].method != "PATCH" || sent[1].path != pullsPath+"/5" || sent[1].body["state"] != "closed" {
			t.Errorf("Close's second request = %s %s %v; want the closing PATCH", sent[1].method, sent[1].path, sent[1].body)
		}
	})

	t.Run("Status", func(t *testing.T) {
		tests := []struct {
			number int
			want   pullrequest.Status
		}{
			{1, pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: "h1", Approved: true, ChangesRequested: true}},
			{2, pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: "h2"}},
			{3, pullrequest.Status{State: pullrequest.StateClosed, HeadSHA: "h3"}},
		}
		for _, tt := range tests {
			got, err := client.Status(ctx, tt.number)
			if err != nil || got != tt.want {
				t.Errorf("Status(%d) = %+v, %v; want %+v", tt.number, got, err, tt.want)
			}
		}
	})

	t.Run("Merge", func(t *testing.T) {
		tests := []struct {
			number   int
			wantKind pullrequest.RefusalKind
		}{
			{10, pullrequest.RefusalMethod},
			{11, pullrequest.RefusalPending},
			{12, pullrequest.RefusalOther},
			{13, pullrequest.RefusalOther},
		}
		for _, tt := range tests {
			err := client.Merge(ctx, tt.number, "h", true, "t", "m")
			var refusal *pullrequest.MergeRefusal
			if !errors.As(err, &refusal) || refusal.Kind != tt.wantKind || refusal.Message == "" {
				t.Errorf("Merge(%d) error = %v; want a %q refusal carrying GitHub's message", tt.number, err, tt.wantKind)
			}
		}
		if err := client.Merge(ctx, 14, "headsha", false, "the title", "the message"); err != nil {
			t.Fatalf("Merge(14) error: %v", err)
		}
		sent := server.last(t)
		want := map[string]any{"sha": "headsha", "merge_method": "merge", "commit_title": "the title", "commit_message": "the message"}
		if sent.method != "PUT" || sent.path != pullsPath+"/14/merge" || !reflect.DeepEqual(sent.body, want) {
			t.Errorf("Merge sent %s %s %v; want PUT %s/14/merge %v", sent.method, sent.path, sent.body, pullsPath, want)
		}
		if err := client.Merge(ctx, 14, "headsha", true, "", ""); err != nil {
			t.Fatalf("Merge(14, squash) error: %v", err)
		}
		if got := server.last(t).body["merge_method"]; got != "squash" {
			t.Errorf("squash Merge sent merge_method %v; want squash", got)
		}
		err := client.Merge(ctx, 99, "h", false, "t", "m")
		var refusal *pullrequest.MergeRefusal
		if err == nil || errors.As(err, &refusal) {
			t.Errorf("Merge(99) error = %v; want a plain wrapped failure, not a refusal", err)
		}
	})

	t.Run("OpenRefusesNonGitHubOrigin", func(t *testing.T) {
		if _, err := pullrequest.Open("https://gitlab.com/owner/repo.git"); err == nil || !strings.Contains(err.Error(), "gitlab.com") {
			t.Errorf("Open(gitlab origin) error = %v; want a refusal naming the origin", err)
		}
	})
}
