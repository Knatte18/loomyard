// api_test.go covers the Slack Web API client against an httptest server that records each request.

package slackengine_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordedRequest is what the fake Slack saw of one request.
type recordedRequest struct {
	path          string
	authorization string
	form          url.Values
}

// fakeSlackAPI is an httptest server whose handler answers each request from reply and records it.
type fakeSlackAPI struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

// newFakeSlackAPI starts a server that answers every request with reply and closes it at cleanup.
func newFakeSlackAPI(t *testing.T, reply func(request recordedRequest, w http.ResponseWriter)) *fakeSlackAPI {
	t.Helper()
	fake := &fakeSlackAPI{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm = %v", err)
		}
		request := recordedRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization"), form: r.PostForm}
		fake.mu.Lock()
		fake.requests = append(fake.requests, request)
		fake.mu.Unlock()
		reply(request, w)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// api returns a client for the fake with the token xoxb-secret.
func (fake *fakeSlackAPI) api() *slackengine.API {
	return slackengine.NewAPI(fake.server.URL+"/", "xoxb-secret", fake.server.Client())
}

// recorded returns a copy of the requests the fake has seen.
func (fake *fakeSlackAPI) recorded() []recordedRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]recordedRequest(nil), fake.requests...)
}

// answerJSON writes body as a Slack answer.
func answerJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestAPI(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	ctx := context.Background()

	t.Run("EachMethodSendsItsPathBearerAndFormFields", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(request recordedRequest, w http.ResponseWriter) {
			if request.path == "/conversations.info" {
				answerJSON(w, `{"ok":true,"channel":{"id":"C1","name":"ops","is_member":true,"is_private":true}}`)
				return
			}
			answerJSON(w, `{"ok":true,"messages":[{"ts":"5.000001"}]}`)
		})
		api := fake.api()
		if err := api.AuthTest(ctx); err != nil {
			t.Fatalf("AuthTest = %v", err)
		}
		info, err := api.ConversationInfo(ctx, "C1")
		if err != nil {
			t.Fatalf("ConversationInfo = %v", err)
		}
		if want := (slackengine.ChannelInfo{ID: "C1", Name: "ops", IsMember: true, IsPrivate: true}); info != want {
			t.Errorf("ConversationInfo = %+v; want %+v", info, want)
		}
		if err := api.UserInfo(ctx, "U1"); err != nil {
			t.Fatalf("UserInfo = %v", err)
		}
		if err := api.PostMessage(ctx, "C1", "hello"); err != nil {
			t.Fatalf("PostMessage = %v", err)
		}
		if _, err := api.History(ctx, "C1", "4.0"); err != nil {
			t.Fatalf("History = %v", err)
		}
		want := []recordedRequest{
			{path: "/auth.test", form: url.Values{}},
			{path: "/conversations.info", form: url.Values{"channel": {"C1"}}},
			{path: "/users.info", form: url.Values{"user": {"U1"}}},
			{path: "/chat.postMessage", form: url.Values{"channel": {"C1"}, "text": {"hello"}}},
			{path: "/conversations.history", form: url.Values{"channel": {"C1"}, "oldest": {"4.0"}, "limit": {"200"}}},
		}
		got := fake.recorded()
		if len(got) != len(want) {
			t.Fatalf("recorded %d requests; want %d", len(got), len(want))
		}
		for i := range want {
			if got[i].authorization != "Bearer xoxb-secret" {
				t.Errorf("request %d authorization = %q; want the bearer token", i, got[i].authorization)
			}
			if got[i].path != want[i].path || !cmp.Equal(got[i].form, want[i].form) {
				t.Errorf("request %d = %s %v; want %s %v", i, got[i].path, got[i].form, want[i].path, want[i].form)
			}
		}
	})

	t.Run("HistoryFollowsThePagesAndReturnsOldestFirst", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(request recordedRequest, w http.ResponseWriter) {
			if request.form.Get("cursor") == "page2" {
				answerJSON(w, `{"ok":true,"has_more":false,"messages":[{"ts":"2.000000","text":"b"},{"ts":"1.000000","text":"a"}]}`)
				return
			}
			answerJSON(w, `{"ok":true,"has_more":true,"response_metadata":{"next_cursor":"page2"},"messages":[{"ts":"4.000000","text":"d"},{"ts":"3.000000","text":"c"}]}`)
		})
		messages, err := fake.api().History(ctx, "C1", "0")
		if err != nil {
			t.Fatalf("History = %v", err)
		}
		var texts []string
		for _, message := range messages {
			texts = append(texts, message.Text)
		}
		if want := []string{"a", "b", "c", "d"}; !cmp.Equal(texts, want) {
			t.Errorf("History texts = %v; want %v", texts, want)
		}
		requests := fake.recorded()
		if len(requests) != 2 || requests[1].form.Get("cursor") != "page2" {
			t.Errorf("History sent %+v; want a second request carrying the cursor page2", requests)
		}
	})

	t.Run("LatestOnAnEmptyChannelFindsNothing", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(_ recordedRequest, w http.ResponseWriter) { answerJSON(w, `{"ok":true,"messages":[]}`) })
		ts, found, err := fake.api().Latest(ctx, "C1")
		if err != nil || found || ts != "" {
			t.Errorf("Latest(empty) = %q, %v, %v; want not found", ts, found, err)
		}
		if got := fake.recorded()[0].form.Get("limit"); got != "1" {
			t.Errorf("Latest sent limit %q; want 1", got)
		}
	})

	t.Run("FailureKinds", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			reply     func(w http.ResponseWriter)
			call      func(api *slackengine.API) error
			wantRate  time.Duration
			wantCode  string
			wantPlain bool
		}{
			{
				name: "RetryAfterHeaderOn429",
				reply: func(w http.ResponseWriter) {
					w.Header().Set("Retry-After", "7")
					w.WriteHeader(http.StatusTooManyRequests)
				},
				call:     func(api *slackengine.API) error { return api.PostMessage(ctx, "C1", "x") },
				wantRate: 7 * time.Second,
			},
			{
				name:     "MissingRetryAfterReadsAsOneSecond",
				reply:    func(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) },
				call:     func(api *slackengine.API) error { return api.PostMessage(ctx, "C1", "x") },
				wantRate: time.Second,
			},
			{
				name:     "RatelimitedCodeIsARateLimit",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"ratelimited"}`) },
				call:     func(api *slackengine.API) error { return api.PostMessage(ctx, "C1", "x") },
				wantRate: time.Second,
			},
			{
				name:     "MsgTooLongIsAnAPIError",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"msg_too_long"}`) },
				call:     func(api *slackengine.API) error { return api.PostMessage(ctx, "C1", "x") },
				wantCode: "msg_too_long",
			},
			{
				name:     "InvalidAuthOnAuthTestIsAnAPIError",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"invalid_auth"}`) },
				call:     func(api *slackengine.API) error { return api.AuthTest(ctx) },
				wantCode: "invalid_auth",
			},
			{
				name:      "ServerErrorIsPlain",
				reply:     func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
				call:      func(api *slackengine.API) error { return api.PostMessage(ctx, "C1", "x") },
				wantPlain: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fake := newFakeSlackAPI(t, func(_ recordedRequest, w http.ResponseWriter) { tt.reply(w) })
				err := tt.call(fake.api())
				var rateLimit *slackengine.RateLimitError
				var apiErr *slackengine.APIError
				switch {
				case tt.wantRate != 0:
					if !errors.As(err, &rateLimit) || rateLimit.RetryAfter != tt.wantRate {
						t.Errorf("error = %v; want a *RateLimitError retrying after %s", err, tt.wantRate)
					}
				case tt.wantCode != "":
					if !errors.As(err, &apiErr) || apiErr.Code != tt.wantCode {
						t.Errorf("error = %v; want an *APIError with code %s", err, tt.wantCode)
					}
				case tt.wantPlain:
					if err == nil || errors.As(err, &rateLimit) || errors.As(err, &apiErr) {
						t.Errorf("error = %v; want a plain error", err)
					}
				}
				if err != nil && strings.Contains(err.Error(), "xoxb-secret") {
					t.Errorf("error %q carries the token", err)
				}
			})
		}
	})
}
