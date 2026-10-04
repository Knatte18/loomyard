// inbox_test.go covers the intake calls in inbox.go through the NewGitHubClient seam, against an
// httptest server, so no test reaches real token resolution or the network -- untagged Tier 1.

package selfreportengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/githubclient"
)

const issuesPath = "/repos/Knatte18/loomyard/issues"

// TestListOpenIssues_PaginatesAndExcludesPullRequests follows the Link header across two pages,
// drops the pull request, and asserts the exact owner/repo path and the open filter.
func TestListOpenIssues_PaginatesAndExcludesPullRequests(t *testing.T) {
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":3,"title":"three","state":"open","labels":[]}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, server.URL, issuesPath))
		_, _ = w.Write([]byte(`[
			{"number":1,"title":"one","body":"b","html_url":"https://x/1","state":"open","labels":[{"name":"bug"}],"created_at":"2026-01-02T03:04:05Z"},
			{"number":2,"title":"pr","state":"open","pull_request":{"url":"https://x/pr"}}
		]`))
	}))
	t.Cleanup(server.Close)
	installGitHubClient(t, server.URL)

	got, err := ListOpenIssues()
	if err != nil {
		t.Fatalf("ListOpenIssues() error = %v; want nil", err)
	}

	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 3 {
		t.Fatalf("ListOpenIssues() = %+v; want issues 1 and 3 with the pull request excluded", got)
	}
	if got[0].Title != "one" || got[0].Body != "b" || got[0].URL != "https://x/1" ||
		len(got[0].Labels) != 1 || got[0].Labels[0] != "bug" || got[0].CreatedAt.Year() != 2026 {
		t.Errorf("ListOpenIssues()[0] = %+v; want its fields carried over", got[0])
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

// TestGetIssue_ReportsPullRequestAndClosedState checks that GetIssue returns a pull request and
// a closed issue without excluding either.
func TestGetIssue_ReportsPullRequestAndClosedState(t *testing.T) {
	var captured []requestCapture
	server := newIssueServer(t, http.StatusOK,
		`{"number":7,"title":"t","state":"closed","state_reason":"not_planned","pull_request":{"url":"https://x/pr"}}`, &captured)
	installGitHubClient(t, server.URL)

	got, err := GetIssue(7)
	if err != nil {
		t.Fatalf("GetIssue() error = %v; want nil", err)
	}

	if got.Number != 7 || got.State != "closed" || got.StateReason != "not_planned" || !got.PullRequest {
		t.Errorf("GetIssue() = %+v; want issue 7, closed, not_planned, a pull request", got)
	}
	if len(captured) != 1 || captured[0].method != http.MethodGet || captured[0].path != issuesPath+"/7" {
		t.Errorf("requests = %+v; want one GET %s/7", captured, issuesPath)
	}
}

// TestCommentAndClose_SendsCommentThenClose drives both state reasons and checks the order and
// payloads of the two requests.
func TestCommentAndClose_SendsCommentThenClose(t *testing.T) {
	for _, tc := range []struct {
		completed bool
		reason    string
	}{{true, "completed"}, {false, "not_planned"}} {
		t.Run(tc.reason, func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusOK, `{"number":9}`, &captured)
			installGitHubClient(t, server.URL)

			if err := CommentAndClose(9, "folded into x", tc.completed); err != nil {
				t.Fatalf("CommentAndClose() error = %v; want nil", err)
			}

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
			if captured[1].body["state"] != "closed" || captured[1].body["state_reason"] != tc.reason {
				t.Errorf("close body = %v; want state closed with reason %s", captured[1].body, tc.reason)
			}
		})
	}
}

// TestCommentAndClose_FailedCommentSkipsClose checks that a rejected comment never reaches the
// close.
func TestCommentAndClose_FailedCommentSkipsClose(t *testing.T) {
	var captured []requestCapture
	server := newIssueServer(t, http.StatusForbidden, `{"message":"Forbidden thing"}`, &captured)
	installGitHubClient(t, server.URL)

	err := CommentAndClose(9, "c", true)

	if err == nil || !strings.Contains(err.Error(), "Forbidden thing") {
		t.Fatalf("CommentAndClose() error = %v; want the response message", err)
	}
	if len(captured) != 1 {
		t.Errorf("request count = %d; want 1 (no close after a failed comment)", len(captured))
	}
}

// TestCommentAndClose_FailedCloseNamesPostedComment checks that a close failing after a posted
// comment says so.
func TestCommentAndClose_FailedCloseNamesPostedComment(t *testing.T) {
	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		count++
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"cannot close"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	t.Cleanup(server.Close)
	installGitHubClient(t, server.URL)

	err := CommentAndClose(9, "c", false)

	if err == nil {
		t.Fatal("CommentAndClose() error = nil; want the close failure")
	}
	for _, want := range []string{"comment posted", "second comment", "cannot close"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CommentAndClose() error = %q; want it to contain %q", err.Error(), want)
		}
	}
	if count != 2 {
		t.Errorf("request count = %d; want comment then close", count)
	}
}

// TestInboxCalls_TokenNotResolvable checks that all three calls surface an unresolvable token.
func TestInboxCalls_TokenNotResolvable(t *testing.T) {
	installFailingGitHubClientFactory(t, githubclient.ErrTokenUnresolvable)

	_, listErr := ListOpenIssues()
	_, getErr := GetIssue(1)
	closeErr := CommentAndClose(1, "c", true)

	for name, err := range map[string]error{"ListOpenIssues": listErr, "GetIssue": getErr, "CommentAndClose": closeErr} {
		if !errors.Is(err, githubclient.ErrTokenUnresolvable) {
			t.Errorf("%s() error = %v; want errors.Is(err, githubclient.ErrTokenUnresolvable)", name, err)
		}
	}
}
