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

// documentedAnswers holds, by request path, the example answer Slack's API reference shows for each method lym calls.
// The answers keep Slack's shapes.
// chat.postMessage sends `channel` as an id string, while conversations.create sends it as an object.
var documentedAnswers = map[string]string{
	"/auth.test": `{
		"ok": true,
		"url": "https://subarachnoid.slack.com/",
		"team": "Subarachnoid Workspace",
		"user": "grace",
		"team_id": "T12345678",
		"user_id": "W12345678",
		"bot_id": "B12345678"
	}`,
	"/bots.info":            `{"ok":true,"bot":{"id":"B12345678","app_id":"A161CLERW","name":"beforebot","deleted":false}}`,
	"/conversations.create": `{"ok":true,"channel":{"id":"C0EAQDV4Z","name":"endeavor","is_archived":false,"creator":"U012A3CDE","is_member":true}}`,
	"/conversations.list": `{"ok":true,"channels":[{"id":"C012AB3CD","name":"general","is_archived":false,"creator":"U012A3CDE","is_member":true}],` +
		`"response_metadata":{"next_cursor":""}}`,
	"/conversations.join":    `{"ok":true,"channel":{"id":"C012AB3CD","name":"general"}}`,
	"/conversations.invite":  `{"ok":true,"channel":{"id":"C012AB3CD","name":"general"}}`,
	"/conversations.rename":  `{"ok":true,"channel":{"id":"C012AB3CD","name":"general"}}`,
	"/conversations.archive": `{"ok":true}`,
	"/conversations.replies": `{"ok":true,"messages":[` +
		`{"type":"message","user":"U061F7AUR","text":"island","thread_ts":"1482960137.003543","reply_count":1,` +
		`"latest_reply":"1483037603.017503","ts":"1482960137.003543"},` +
		`{"type":"message","user":"U061F7AUR","text":"one island","thread_ts":"1482960137.003543",` +
		`"parent_user_id":"U061F7AUR","ts":"1483037603.017503"}],"has_more":false}`,
	"/apps.connections.open": `{"ok":true,"url":"wss://wss-primary.slack.com/link/?ticket=1234-5678"}`,
	"/conversations.history": `{
		"ok": true,
		"messages": [
			{"type": "message", "user": "U123ABC456", "text": "I find you punny and would like to smell your nose letter", "ts": "1512085950.000216"},
			{"type": "message", "user": "U222BBB222", "text": "What, you want to smell my shoes better?", "ts": "1512104434.000490"}
		],
		"has_more": true,
		"pin_count": 0,
		"response_metadata": {"next_cursor": "bmV4dF90czoxNTEyMDg1ODYxMDAwNTQz"}
	}`,
	"/users.info": `{
		"ok": true,
		"user": {
			"id": "W012A3CDE",
			"team_id": "T012AB3C4",
			"name": "spengler",
			"deleted": false,
			"color": "9f69e7",
			"real_name": "Egon Spengler",
			"tz": "America/Los_Angeles",
			"tz_label": "Pacific Daylight Time",
			"tz_offset": -25200,
			"profile": {
				"avatar_hash": "ge3b51ca72de",
				"status_text": "Print is dead",
				"status_emoji": ":books:",
				"real_name": "Egon Spengler",
				"display_name": "spengler",
				"real_name_normalized": "Egon Spengler",
				"display_name_normalized": "spengler",
				"email": "spengler@ghostbusters.example.com",
				"team": "T012AB3C4"
			},
			"is_admin": true,
			"is_owner": false,
			"is_primary_owner": false,
			"is_restricted": false,
			"is_ultra_restricted": false,
			"is_bot": false,
			"updated": 1502138686,
			"is_app_user": false,
			"has_2fa": false
		}
	}`,
	"/chat.postMessage": `{
		"ok": true,
		"channel": "C123ABC456",
		"ts": "1503435956.000247",
		"message": {
			"text": "Here's a message for you",
			"username": "ecto1",
			"bot_id": "B123ABC456",
			"attachments": [{"text": "This is an attachment", "id": 1, "fallback": "This is an attachment's fallback"}],
			"type": "message",
			"subtype": "bot_message",
			"ts": "1503435956.000247"
		}
	}`,
}

func TestAPI(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	ctx := context.Background()

	t.Run("EachMethodSendsItsRequestAndDecodesSlacksDocumentedAnswer", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(request recordedRequest, w http.ResponseWriter) {
			if request.form.Get("cursor") != "" {
				answerJSON(w, `{"ok":true,"messages":[],"has_more":false}`)
				return
			}
			answerJSON(w, documentedAnswers[request.path])
		})
		api := fake.api()
		identity, err := api.AuthTest(ctx)
		if err != nil {
			t.Fatalf("AuthTest = %v", err)
		}
		if want := (slackengine.Identity{TeamID: "T12345678", UserID: "W12345678", BotID: "B12345678"}); identity != want {
			t.Errorf("AuthTest = %+v; want %+v", identity, want)
		}
		if err := api.UserInfo(ctx, "U1"); err != nil {
			t.Fatalf("UserInfo = %v", err)
		}
		if err := api.PostAs(ctx, "C1", "hello", slackengine.Sender{}); err != nil {
			t.Fatalf("PostAs = %v", err)
		}
		messages, err := api.History(ctx, "C1", "4.0")
		if err != nil {
			t.Fatalf("History = %v", err)
		}
		wantMessages := []slackengine.Message{
			{TS: "1512085950.000216", User: "U123ABC456", Text: "I find you punny and would like to smell your nose letter"},
			{TS: "1512104434.000490", User: "U222BBB222", Text: "What, you want to smell my shoes better?"},
		}
		if diff := cmp.Diff(wantMessages, messages); diff != "" {
			t.Errorf("History() mismatch (-want +got):\n%s", diff)
		}
		appID, err := api.BotAppID(ctx, "B12345678")
		if err != nil || appID != "A161CLERW" {
			t.Errorf("BotAppID = %q, %v; want A161CLERW", appID, err)
		}
		created, err := api.CreateChannel(ctx, "endeavor")
		wantCreated := slackengine.ChannelInfo{ID: "C0EAQDV4Z", Name: "endeavor", IsMember: true, Creator: "U012A3CDE"}
		if err != nil || created != wantCreated {
			t.Errorf("CreateChannel = %+v, %v; want %+v", created, err, wantCreated)
		}
		found, ok, err := api.FindChannel(ctx, "general")
		wantFound := slackengine.ChannelInfo{ID: "C012AB3CD", Name: "general", IsMember: true, Creator: "U012A3CDE"}
		if err != nil || !ok || found != wantFound {
			t.Errorf("FindChannel = %+v, %v, %v; want %+v", found, ok, err, wantFound)
		}
		for _, step := range []struct {
			name string
			call func() error
		}{
			{"Join", func() error { return api.Join(ctx, "C1") }},
			{"Invite", func() error { return api.Invite(ctx, "C1", "U1") }},
			{"Rename", func() error { return api.Rename(ctx, "C1", "new-name") }},
			{"Archive", func() error { return api.Archive(ctx, "C1") }},
		} {
			if err := step.call(); err != nil {
				t.Errorf("%s = %v", step.name, err)
			}
		}
		replies, err := api.Replies(ctx, "C1", "1482960137.003543", "0")
		if err != nil {
			t.Fatalf("Replies = %v", err)
		}
		wantReplies := []slackengine.Message{
			{TS: "1482960137.003543", ThreadTS: "1482960137.003543", User: "U061F7AUR", Text: "island", LatestReply: "1483037603.017503"},
			{TS: "1483037603.017503", ThreadTS: "1482960137.003543", User: "U061F7AUR", Text: "one island"},
		}
		if diff := cmp.Diff(wantReplies, replies); diff != "" {
			t.Errorf("Replies() mismatch (-want +got):\n%s", diff)
		}
		if err := api.PostAs(ctx, "C1", "hi", slackengine.Sender{Username: "ly-orch", IconEmoji: ":robot_face:"}); err != nil {
			t.Errorf("PostAs = %v", err)
		}
		connectionURL, err := api.OpenConnection(ctx)
		if wantURL := "wss://wss-primary.slack.com/link/?ticket=1234-5678"; err != nil || connectionURL != wantURL {
			t.Errorf("OpenConnection = %q, %v; want %q", connectionURL, err, wantURL)
		}
		historyForm := url.Values{"channel": {"C1"}, "oldest": {"4.0"}, "limit": {"200"}}
		want := []recordedRequest{
			{path: "/auth.test", form: url.Values{}},
			{path: "/users.info", form: url.Values{"user": {"U1"}}},
			{path: "/chat.postMessage", form: url.Values{"channel": {"C1"}, "text": {"hello"}}},
			{path: "/conversations.history", form: historyForm},
			{path: "/conversations.history", form: url.Values{
				"channel": {"C1"}, "oldest": {"4.0"}, "limit": {"200"}, "cursor": {"bmV4dF90czoxNTEyMDg1ODYxMDAwNTQz"},
			}},
			{path: "/bots.info", form: url.Values{"bot": {"B12345678"}}},
			{path: "/conversations.create", form: url.Values{"name": {"endeavor"}, "is_private": {"false"}}},
			{path: "/conversations.list", form: url.Values{
				"types": {"public_channel"}, "exclude_archived": {"false"}, "limit": {"200"},
			}},
			{path: "/conversations.join", form: url.Values{"channel": {"C1"}}},
			{path: "/conversations.invite", form: url.Values{"channel": {"C1"}, "users": {"U1"}}},
			{path: "/conversations.rename", form: url.Values{"channel": {"C1"}, "name": {"new-name"}}},
			{path: "/conversations.archive", form: url.Values{"channel": {"C1"}}},
			{path: "/conversations.replies", form: url.Values{"channel": {"C1"}, "ts": {"1482960137.003543"}, "oldest": {"0"}}},
			{path: "/chat.postMessage", form: url.Values{
				"channel": {"C1"}, "text": {"hi"}, "username": {"ly-orch"}, "icon_emoji": {":robot_face:"},
			}},
			{path: "/apps.connections.open", form: url.Values{}},
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

	t.Run("FindChannelFollowsThePagesAndReportsAMissingName", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(request recordedRequest, w http.ResponseWriter) {
			if request.form.Get("cursor") == "page2" {
				answerJSON(w, `{"ok":true,"channels":[{"id":"C2","name":"wanted","is_archived":true}],"response_metadata":{"next_cursor":""}}`)
				return
			}
			answerJSON(w, `{"ok":true,"channels":[{"id":"C1","name":"other"}],"response_metadata":{"next_cursor":"page2"}}`)
		})
		api := fake.api()
		found, ok, err := api.FindChannel(ctx, "wanted")
		if want := (slackengine.ChannelInfo{ID: "C2", Name: "wanted", IsArchived: true}); err != nil || !ok || found != want {
			t.Errorf("FindChannel(wanted) = %+v, %v, %v; want %+v", found, ok, err, want)
		}
		if requests := fake.recorded(); len(requests) != 2 || requests[1].form.Get("cursor") != "page2" {
			t.Errorf("FindChannel sent %+v; want a second request carrying the cursor page2", requests)
		}
		if _, ok, err := api.FindChannel(ctx, "absent"); err != nil || ok {
			t.Errorf("FindChannel(absent) = found %v, %v; want not found", ok, err)
		}
	})

	t.Run("PostAsWithAnEmptySenderSendsNoUsernameOrIcon", func(t *testing.T) {
		t.Parallel()
		fake := newFakeSlackAPI(t, func(_ recordedRequest, w http.ResponseWriter) { answerJSON(w, `{"ok":true}`) })
		if err := fake.api().PostAs(ctx, "C1", "hi", slackengine.Sender{}); err != nil {
			t.Fatalf("PostAs = %v", err)
		}
		if got, want := fake.recorded()[0].form, (url.Values{"channel": {"C1"}, "text": {"hi"}}); !cmp.Equal(got, want) {
			t.Errorf("PostAs sent %v; want %v", got, want)
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
				call:     func(api *slackengine.API) error { return api.PostAs(ctx, "C1", "x", slackengine.Sender{}) },
				wantRate: 7 * time.Second,
			},
			{
				name:     "MissingRetryAfterReadsAsOneSecond",
				reply:    func(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) },
				call:     func(api *slackengine.API) error { return api.PostAs(ctx, "C1", "x", slackengine.Sender{}) },
				wantRate: time.Second,
			},
			{
				name:     "RatelimitedCodeIsARateLimit",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"ratelimited"}`) },
				call:     func(api *slackengine.API) error { return api.PostAs(ctx, "C1", "x", slackengine.Sender{}) },
				wantRate: time.Second,
			},
			{
				name:     "MsgTooLongIsAnAPIError",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"msg_too_long"}`) },
				call:     func(api *slackengine.API) error { return api.PostAs(ctx, "C1", "x", slackengine.Sender{}) },
				wantCode: "msg_too_long",
			},
			{
				name:  "InvalidAuthOnAuthTestIsAnAPIError",
				reply: func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"invalid_auth"}`) },
				call: func(api *slackengine.API) error {
					_, err := api.AuthTest(ctx)
					return err
				},
				wantCode: "invalid_auth",
			},
			{
				name:  "InviteOfAMemberIsNil",
				reply: func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"already_in_channel"}`) },
				call:  func(api *slackengine.API) error { return api.Invite(ctx, "C1", "U1") },
			},
			{
				name:  "ArchiveOfAnArchivedChannelIsNil",
				reply: func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"already_archived"}`) },
				call:  func(api *slackengine.API) error { return api.Archive(ctx, "C1") },
			},
			{
				name:     "InviteRefusedForAnotherCodeIsAnAPIError",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"user_not_found"}`) },
				call:     func(api *slackengine.API) error { return api.Invite(ctx, "C1", "U1") },
				wantCode: "user_not_found",
			},
			{
				name:     "ArchiveRefusedForAnotherCodeIsAnAPIError",
				reply:    func(w http.ResponseWriter) { answerJSON(w, `{"ok":false,"error":"cant_archive_general"}`) },
				call:     func(api *slackengine.API) error { return api.Archive(ctx, "C1") },
				wantCode: "cant_archive_general",
			},
			{
				name:      "ServerErrorIsPlain",
				reply:     func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
				call:      func(api *slackengine.API) error { return api.PostAs(ctx, "C1", "x", slackengine.Sender{}) },
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
				default:
					if err != nil {
						t.Errorf("error = %v; want nil", err)
					}
				}
				if err != nil && strings.Contains(err.Error(), "xoxb-secret") {
					t.Errorf("error %q carries the token", err)
				}
			})
		}
	})
}
