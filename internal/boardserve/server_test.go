// server_test.go unit-tests the board server's handler in server.go and write.go, in process over a temp board.

package boardserve

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testPort  = 4321
	testToken = "0123456789abcdef0123456789abcdef"
)

// testClock is the fixed time every board write stamps its dates with.
var testClock = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

// newTestBoard returns a board over a fresh temp directory with git skipped, seeded with the entries every case reads:
// a draft task, a released task, a note, a task with a hand-set status and a task a run holds.
func newTestBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	boardPath := t.TempDir()
	board := boardengine.New(boardengine.Config{
		Path:         boardPath,
		Readme:       "Home.md",
		DesignPrefix: "design-",
		Types:        []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
		Labels:       []boardengine.Label{{Name: "area"}},
		SkipGit:      true,
		Clock:        func() time.Time { return testClock },
	})
	seed := func(fields map[string]any) {
		if _, err := board.UpsertTaskReporting(fields); err != nil {
			t.Fatalf("seed %v: %v", fields["slug"], err)
		}
	}
	seed(map[string]any{"slug": "draft-task", "kind": "task", "title": "Draft task", "labels": []any{"bug"}, "body": "A body."})
	seed(map[string]any{"slug": "ready-task", "kind": "task", "title": "Ready task", "labels": []any{"bug"}})
	seed(map[string]any{"slug": "status-task", "kind": "task", "title": "Status task", "labels": []any{"bug"}})
	seed(map[string]any{"slug": "run-task", "kind": "task", "title": "Run task", "labels": []any{"bug"}})
	seed(map[string]any{"slug": "a-note", "kind": "note", "title": "A note", "labels": []any{"bug"}})
	if _, err := board.SetStatusReleasingProject("ready-task", nil); err != nil {
		t.Fatalf("release ready-task: %v", err)
	}
	handStatus := "waiting"
	if _, err := board.SetStatusReleasingProject("status-task", &handStatus); err != nil {
		t.Fatalf("set status-task: %v", err)
	}
	if err := board.SetRunStatus("run-task", boardengine.RunStatus("running", "loom")); err != nil {
		t.Fatalf("set run-task: %v", err)
	}
	return board, boardPath
}

// newTestServer returns a server over a seeded board, answering a long poll within bound.
func newTestServer(t *testing.T, bound time.Duration) (*Server, *boardengine.Board, string) {
	t.Helper()
	board, boardPath := newTestBoard(t)
	server := NewServer(ServerConfig{Board: board, BoardPath: boardPath, Token: testToken, Port: testPort, LongPollBound: bound, Now: func() time.Time { return testClock }})
	return server, board, boardPath
}

// call sends one request to server with the loopback Host, and mutate may change it before it is served.
func call(server *Server, method, target string, body []byte, mutate func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = "127.0.0.1:4321"
	if mutate != nil {
		mutate(request)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

// authorized adds the bearer token to a request.
func authorized(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+testToken)
}

// authorizedJSON makes a request a valid write: the bearer token and the JSON content type.
func authorizedJSON(request *http.Request) {
	authorized(request)
	request.Header.Set("Content-Type", "application/json")
}

// write posts one write request and returns the response.
func write(t *testing.T, server *Server, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the write: %v", err)
	}
	return call(server, http.MethodPost, routeWrite, body, authorizedJSON)
}

// answer is the JSON a write or a board read answers.
type answer struct {
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error"`
	Entry     *boardengine.ViewEntry `json:"entry"`
	Revision  string                 `json:"revision"`
	Unchanged bool                   `json:"unchanged"`
	Board     *boardengine.BoardView `json:"board"`
}

// decode reads a response's JSON body.
func decode(t *testing.T, recorder *httptest.ResponseRecorder) answer {
	t.Helper()
	var decoded answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return decoded
}

// viewEntry returns the entry slug of the board's current view.
func viewEntry(t *testing.T, board *boardengine.Board, slug string) boardengine.ViewEntry {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	entry, found := entryOf(view, slug)
	if !found {
		t.Fatalf("the view holds no entry %q", slug)
	}
	return entry
}

// boardJSON returns the bytes of the board's store.
func boardJSON(t *testing.T, boardPath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	return data
}

// requireHardenedHeaders fails unless the response carries the no-store and nosniff headers and no CORS header.
func requireHardenedHeaders(t *testing.T, label string, recorder *httptest.ResponseRecorder) {
	t.Helper()
	header := recorder.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("%s: Cache-Control, X-Content-Type-Options = %q, %q; want no-store and nosniff", label, header.Get("Cache-Control"), header.Get("X-Content-Type-Options"))
	}
	if header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("%s: carries a CORS header", label)
	}
}

// TestServer drives the handler in process.
// It covers the transport checks, the page and its policy, and the board read and its long poll.
// It also covers the six writes and their refusals, the revision check and the file route.
func TestServer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TransportRefusalsChangeNothing", func(t *testing.T) {
		t.Parallel()
		board, boardPath := newTestBoard(t)
		clock := testClock
		server := NewServer(ServerConfig{Board: board, BoardPath: boardPath, Token: testToken, Port: testPort, LongPollBound: time.Minute, Now: func() time.Time { return clock }})
		// The clock moves on after construction, so a request that stamped API activity would move lastAPIRequest off testClock.
		clock = testClock.Add(time.Hour)
		before := boardJSON(t, boardPath)
		validWrite := []byte(`{"op":"edit","slug":"draft-task","revision":"x","fields":{"title":"T"}}`)
		oversize := []byte(`{"op":"edit","slug":"draft-task","revision":"x","fields":{"body":"` + strings.Repeat("a", maxBodyBytes) + `"}}`)
		tests := []struct {
			name   string
			method string
			target string
			body   []byte
			mutate func(*http.Request)
			want   int
		}{
			{"WrongHost", http.MethodGet, routeBoard, nil, func(r *http.Request) { authorized(r); r.Host = "localhost:4321" }, http.StatusMisdirectedRequest},
			{"NoToken", http.MethodGet, routeBoard, nil, nil, http.StatusUnauthorized},
			{"WrongToken", http.MethodGet, routeBoard, nil, func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
			{"WriteWithNoToken", http.MethodPost, routeWrite, validWrite, func(r *http.Request) { r.Header.Set("Content-Type", "application/json") }, http.StatusUnauthorized},
			{"FileWithNoToken", http.MethodGet, routeFile + "design-draft-task.md", nil, nil, http.StatusUnauthorized},
			{"FileWithWrongToken", http.MethodGet, routeFile + "design-draft-task.md?t=nope", nil, nil, http.StatusUnauthorized},
			{"WriteWithoutJSONContentType", http.MethodPost, routeWrite, validWrite, func(r *http.Request) { authorized(r); r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
			{"WriteFromAForeignOrigin", http.MethodPost, routeWrite, validWrite, func(r *http.Request) { authorizedJSON(r); r.Header.Set("Origin", "http://example.com") }, http.StatusForbidden},
			{"WriteAboveTheBodyCap", http.MethodPost, routeWrite, oversize, authorizedJSON, http.StatusRequestEntityTooLarge},
			{"WriteWithAnUnknownRequestKey", http.MethodPost, routeWrite, []byte(`{"op":"edit","slug":"draft-task","extra":1}`), authorizedJSON, http.StatusBadRequest},
			{"UnknownRoute", http.MethodGet, "/nope", nil, authorized, http.StatusNotFound},
		}
		for _, tt := range tests {
			recorder := call(server, tt.method, tt.target, tt.body, tt.mutate)
			if recorder.Code != tt.want {
				t.Errorf("%s: status = %d; want %d", tt.name, recorder.Code, tt.want)
			}
			requireHardenedHeaders(t, tt.name, recorder)
			// The rows that carry a valid token and fail a later check come after these, and they do count as activity.
			unauthenticated := tt.want == http.StatusUnauthorized || tt.want == http.StatusMisdirectedRequest
			if unauthenticated && !server.lastAPIRequest().Equal(testClock) {
				t.Errorf("%s: lastAPIRequest = %v; want it unchanged by a request with no valid token", tt.name, server.lastAPIRequest())
			}
		}
		if !bytes.Equal(boardJSON(t, boardPath), before) {
			t.Errorf("board.json changed through a refused request")
		}
	})

	t.Run("PageCarriesAPolicyThatHashesItsInlineElements", func(t *testing.T) {
		t.Parallel()
		server, _, _ := newTestServer(t, time.Minute)
		recorder := call(server, http.MethodGet, routePage, nil, nil)
		page := boardengine.ViewerPage()
		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), page) {
			t.Fatalf("GET / = %d with %d bytes; want 200 and the viewer page", recorder.Code, recorder.Body.Len())
		}
		requireHardenedHeaders(t, "page", recorder)
		policy := recorder.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "unsafe-inline") {
			t.Errorf("policy %q admits unsafe-inline", policy)
		}
		hashOf := func(tag string) string {
			pattern := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
			match := pattern.FindSubmatch(page)
			if match == nil {
				t.Fatalf("page holds no inline <%s> element", tag)
			}
			sum := sha256.Sum256(match[1])
			return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		}
		for directive, tag := range map[string]string{"script-src": "script", "style-src": "style"} {
			if want := directive + " " + hashOf(tag) + ";"; !strings.Contains(policy, want) {
				t.Errorf("policy %q; want it to hold %q", policy, want)
			}
		}
	})

	t.Run("PageHoldsEachRoute", func(t *testing.T) {
		t.Parallel()
		page := string(boardengine.ViewerPage())
		for _, route := range []string{routePage, routeBoard, routeWrite, routeFile} {
			if !strings.Contains(page, route) {
				t.Errorf("the page does not spell the route %q", route)
			}
		}
	})

	t.Run("BoardReadIsTheProjection", func(t *testing.T) {
		t.Parallel()
		server, board, _ := newTestServer(t, time.Minute)
		recorder := call(server, http.MethodGet, routeBoard, nil, authorized)
		requireHardenedHeaders(t, "board", recorder)
		got := decode(t, recorder)
		want, err := board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		if !got.OK || got.Board == nil {
			t.Fatalf("answer = %+v; want ok with a board", got)
		}
		got.Board.Rendered, want.Rendered = "", ""
		gotJSON, _ := json.Marshal(got.Board)
		wantJSON, _ := json.Marshal(want)
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Errorf("board = %s; want %s", gotJSON, wantJSON)
		}
		if !server.lastAPIRequest().Equal(testClock) {
			t.Errorf("lastAPIRequest = %v; want the clock's time", server.lastAPIRequest())
		}
	})

	t.Run("PageWritesLeaveTheBoardAsTheCLICallsDo", func(t *testing.T) {
		t.Parallel()
		draftStatus := boardengine.StatusDraft
		tests := []struct {
			name    string
			payload func(entryRevision string) map[string]any
			slug    string
			direct  func(*boardengine.Board) error
		}{
			{"Create", func(string) map[string]any {
				return map[string]any{"op": "create", "slug": "new-task", "fields": map[string]any{"kind": "task", "title": "New", "labels": []any{"bug"}}}
			}, "new-task", func(b *boardengine.Board) error {
				_, err := b.UpsertTaskReporting(map[string]any{"slug": "new-task", "kind": "task", "title": "New", "labels": []any{"bug"}})
				return err
			}},
			{"Edit", func(revision string) map[string]any {
				return map[string]any{"op": "edit", "slug": "draft-task", "revision": revision, "fields": map[string]any{"title": "Retitled"}}
			}, "draft-task", func(b *boardengine.Board) error {
				_, err := b.UpsertTaskReporting(map[string]any{"slug": "draft-task", "title": "Retitled"})
				return err
			}},
			{"Release", func(revision string) map[string]any {
				return map[string]any{"op": "release", "slug": "draft-task", "revision": revision}
			}, "draft-task", func(b *boardengine.Board) error {
				_, err := b.SetStatusReleasingProject("draft-task", nil)
				return err
			}},
			{"Promote", func(revision string) map[string]any {
				return map[string]any{"op": "promote", "slug": "a-note", "revision": revision}
			}, "a-note", func(b *boardengine.Board) error {
				_, err := b.PromoteTo("a-note", "", nil)
				return err
			}},
			{"Draft", func(revision string) map[string]any {
				return map[string]any{"op": "draft", "slug": "ready-task", "revision": revision}
			}, "ready-task", func(b *boardengine.Board) error {
				_, err := b.SetStatusReleasingProject("ready-task", &draftStatus)
				return err
			}},
			{"Undraft", func(revision string) map[string]any {
				return map[string]any{"op": "undraft", "slug": "draft-task", "revision": revision}
			}, "draft-task", func(b *boardengine.Board) error {
				_, err := b.SetStatusReleasingProject("draft-task", nil)
				return err
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				server, board, boardPath := newTestServer(t, time.Minute)
				twin, twinPath := newTestBoard(t)
				revision := ""
				if tt.name != "Create" {
					revision = viewEntry(t, board, tt.slug).Revision
				}
				recorder := write(t, server, tt.payload(revision))
				requireHardenedHeaders(t, tt.name, recorder)
				got := decode(t, recorder)
				if recorder.Code != http.StatusOK || !got.OK || got.Entry == nil {
					t.Fatalf("answer = %d %+v; want 200 and the entry", recorder.Code, got)
				}
				if err := tt.direct(twin); err != nil {
					t.Fatalf("direct write: %v", err)
				}
				if !bytes.Equal(boardJSON(t, boardPath), boardJSON(t, twinPath)) {
					t.Errorf("board.json differs from the twin board's after the same write")
				}
				if want := viewEntry(t, board, tt.slug); !reflect.DeepEqual(*got.Entry, want) {
					t.Errorf("entry = %+v; want the board's fresh %+v", *got.Entry, want)
				}
			})
		}
	})

	t.Run("EditWithTheRevisionAWriteAnsweredIsSaved", func(t *testing.T) {
		t.Parallel()
		server, board, _ := newTestServer(t, time.Minute)
		first := decode(t, write(t, server, map[string]any{"op": "edit", "slug": "draft-task", "revision": viewEntry(t, board, "draft-task").Revision, "fields": map[string]any{"title": "One"}}))
		if first.Entry == nil {
			t.Fatalf("first edit answered %+v", first)
		}
		second := write(t, server, map[string]any{"op": "edit", "slug": "draft-task", "revision": first.Entry.Revision, "fields": map[string]any{"title": "Two"}})
		if second.Code != http.StatusOK || viewEntry(t, board, "draft-task").Title != "Two" {
			t.Errorf("second edit = %d %s; want it saved", second.Code, second.Body.String())
		}
	})

	t.Run("RefusedWritesChangeNothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			payload func(board *boardengine.Board) map[string]any
			want    string
		}{
			{"DraftOfATaskWithAStatus", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "draft", "slug": "status-task", "revision": viewEntry(t, b, "status-task").Revision}
			}, `draft takes a task with no status; entry "status-task" has status "waiting"`},
			{"UndraftOfATaskWithNoStatus", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "undraft", "slug": "ready-task", "revision": viewEntry(t, b, "ready-task").Revision}
			}, `entry "ready-task" has no status, so it is released already`},
			{"UndraftOfARunTask", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "undraft", "slug": "run-task", "revision": viewEntry(t, b, "run-task").Revision}
			}, `entry "run-task" is held by a run`},
			{"UndraftOfAnotherStatus", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "undraft", "slug": "status-task", "revision": viewEntry(t, b, "status-task").Revision}
			}, `undraft takes a draft task; entry "status-task" has status "waiting"`},
			{"DraftOfANote", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "draft", "slug": "a-note", "revision": viewEntry(t, b, "a-note").Revision}
			}, `entry "a-note" is a note, and the page's draft takes a task`},
			{"UndraftOfANote", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "undraft", "slug": "a-note", "revision": viewEntry(t, b, "a-note").Revision}
			}, `entry "a-note" is a note, and the page's undraft takes a task`},
			{"ReleaseOfARunTask", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "release", "slug": "run-task", "revision": viewEntry(t, b, "run-task").Revision}
			}, "release it with lym board set-status"},
			{"ReleaseOfANote", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "release", "slug": "a-note", "revision": viewEntry(t, b, "a-note").Revision}
			}, `entry "a-note" is a note, and the page's release takes a task`},
			{"PromoteOfATask", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "promote", "slug": "ready-task", "revision": viewEntry(t, b, "ready-task").Revision}
			}, `entry "ready-task" is a task already`},
			{"CreateWithARevision", func(b *boardengine.Board) map[string]any {
				return map[string]any{"op": "create", "slug": "ready-task", "revision": viewEntry(t, b, "ready-task").Revision, "fields": map[string]any{"kind": "task", "title": "Again", "labels": []any{"bug"}}}
			}, `create of entry "ready-task" carries a revision`},
			{"EditWithNoRevision", func(*boardengine.Board) map[string]any {
				return map[string]any{"op": "edit", "slug": "ghost", "fields": map[string]any{"title": "T"}}
			}, `edit of entry "ghost" carries no revision`},
			{"UnknownOp", func(*boardengine.Board) map[string]any {
				return map[string]any{"op": "remove", "slug": "draft-task", "revision": "x"}
			}, "create, edit, release, promote, draft, undraft"},
			{"UnknownField", func(*boardengine.Board) map[string]any {
				return map[string]any{"op": "edit", "slug": "draft-task", "revision": "x", "fields": map[string]any{"status": "done"}}
			}, `field "status" is not allowed for edit`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				server, board, boardPath := newTestServer(t, time.Minute)
				before := boardJSON(t, boardPath)
				recorder := write(t, server, tt.payload(board))
				got := decode(t, recorder)
				if recorder.Code != http.StatusUnprocessableEntity || got.OK || !strings.Contains(got.Error, tt.want) {
					t.Errorf("answer = %d %+v; want 422 with an error holding %q", recorder.Code, got, tt.want)
				}
				if !bytes.Equal(boardJSON(t, boardPath), before) {
					t.Errorf("board.json changed through a refused write")
				}
			})
		}
	})

	t.Run("EditOfARunTaskIsRefusedWithTheRunLocksMessage", func(t *testing.T) {
		t.Parallel()
		server, board, _ := newTestServer(t, time.Minute)
		twin, _ := newTestBoard(t)
		_, wantErr := twin.UpsertTaskReporting(map[string]any{"slug": "run-task", "title": "Other"})
		if wantErr == nil {
			t.Fatalf("the direct edit of a run task was accepted")
		}
		recorder := write(t, server, map[string]any{"op": "edit", "slug": "run-task", "revision": viewEntry(t, board, "run-task").Revision, "fields": map[string]any{"title": "Other"}})
		if got := decode(t, recorder); recorder.Code != http.StatusUnprocessableEntity || got.Error != wantErr.Error() {
			t.Errorf("answer = %d %q; want 422 with %q", recorder.Code, got.Error, wantErr.Error())
		}
	})

	t.Run("StaleWritesAnswerTheFreshEntry", func(t *testing.T) {
		t.Parallel()
		server, board, boardPath := newTestServer(t, time.Minute)
		old := viewEntry(t, board, "draft-task").Revision
		if _, err := board.SetStatusReleasingProject("draft-task", nil); err != nil {
			t.Fatalf("release draft-task: %v", err)
		}
		before := boardJSON(t, boardPath)

		existing := decode(t, write(t, server, map[string]any{"op": "create", "slug": "ready-task", "fields": map[string]any{"kind": "task", "title": "Again", "labels": []any{"bug"}}}))
		if want := viewEntry(t, board, "ready-task"); existing.Entry == nil || !reflect.DeepEqual(*existing.Entry, want) {
			t.Errorf("create of an existing slug answered %+v; want 409 with %+v", existing, want)
		}
		for _, op := range []string{"edit", "release"} {
			payload := map[string]any{"op": op, "slug": "draft-task", "revision": old}
			if op == "edit" {
				payload["fields"] = map[string]any{"title": "Late"}
			}
			recorder := write(t, server, payload)
			got := decode(t, recorder)
			if want := viewEntry(t, board, "draft-task"); recorder.Code != http.StatusConflict || got.Entry == nil || !reflect.DeepEqual(*got.Entry, want) {
				t.Errorf("%s with an old revision = %d %+v; want 409 with %+v", op, recorder.Code, got, want)
			}
		}
		if !bytes.Equal(boardJSON(t, boardPath), before) {
			t.Errorf("board.json changed through a stale write")
		}

		removedRevision := viewEntry(t, board, "ready-task").Revision
		if err := board.RemoveTask("ready-task"); err != nil {
			t.Fatalf("remove ready-task: %v", err)
		}
		recorder := write(t, server, map[string]any{"op": "release", "slug": "ready-task", "revision": removedRevision})
		if got := decode(t, recorder); recorder.Code != http.StatusConflict || got.Entry != nil || !strings.Contains(got.Error, "was removed") {
			t.Errorf("release of a removed entry = %d %+v; want 409 with no entry", recorder.Code, got)
		}
		if !strings.Contains(recorder.Body.String(), `"entry":null`) {
			t.Errorf("body = %s; want an explicit null entry", recorder.Body.String())
		}
	})

	t.Run("LongPollAnswersUnchangedOnTheBound", func(t *testing.T) {
		t.Parallel()
		server, _, _ := newTestServer(t, 20*time.Millisecond)
		current := decode(t, call(server, http.MethodGet, routeBoard, nil, authorized)).Revision
		got := decode(t, call(server, http.MethodGet, routeBoard+"?after="+current, nil, authorized))
		if !got.OK || !got.Unchanged || got.Revision != current || got.Board != nil {
			t.Errorf("answer = %+v; want unchanged at %q", got, current)
		}
	})

	t.Run("LongPollOnABoardWithNoBoardJSONWaitsInsteadOfAnsweringAtOnce", func(t *testing.T) {
		t.Parallel()
		boardPath := t.TempDir()
		board := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "design-", SkipGit: true, Clock: func() time.Time { return testClock }})
		server := NewServer(ServerConfig{Board: board, BoardPath: boardPath, Token: testToken, Port: testPort, LongPollBound: 20 * time.Millisecond, Now: func() time.Time { return testClock }})
		first := decode(t, call(server, http.MethodGet, routeBoard, nil, authorized))
		if !first.OK || first.Unchanged || first.Revision != "" || first.Board == nil {
			t.Fatalf("first read = %+v; want the board at once, at the empty revision", first)
		}
		got := decode(t, call(server, http.MethodGet, routeBoard+"?after=", nil, authorized))
		if !got.OK || !got.Unchanged || got.Revision != "" || got.Board != nil {
			t.Errorf("poll with an empty after = %+v; want unchanged at the empty revision after the bound", got)
		}
	})

	t.Run("LongPollReturnsTheNewProjectionOnAChange", func(t *testing.T) {
		t.Parallel()
		server, _, _ := newTestServer(t, time.Minute)
		current := decode(t, call(server, http.MethodGet, routeBoard, nil, authorized)).Revision
		recorders := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			recorders <- call(server, http.MethodGet, routeBoard+"?after="+current, nil, authorized)
		}()
		server.BoardChanged("changed-revision")
		got := decode(t, <-recorders)
		if !got.OK || got.Unchanged || got.Revision != "changed-revision" || got.Board == nil {
			t.Errorf("answer = %+v; want the new projection at changed-revision", got)
		}
	})

	t.Run("DrainAnswersAWaiterUnchangedAtOnce", func(t *testing.T) {
		t.Parallel()
		server, _, _ := newTestServer(t, time.Minute)
		current := decode(t, call(server, http.MethodGet, routeBoard, nil, authorized)).Revision
		recorders := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			recorders <- call(server, http.MethodGet, routeBoard+"?after="+current, nil, authorized)
		}()
		server.beginDrain()
		if got := decode(t, <-recorders); !got.OK || !got.Unchanged {
			t.Errorf("answer = %+v; want unchanged", got)
		}
		if got := decode(t, call(server, http.MethodGet, routeBoard+"?after="+current, nil, authorized)); !got.Unchanged {
			t.Errorf("a long poll after the drain = %+v; want unchanged at once", got)
		}
	})

	t.Run("FileRouteServesOnlyTheFilesTheProjectionNames", func(t *testing.T) {
		t.Parallel()
		server, _, boardPath := newTestServer(t, time.Minute)
		listed := call(server, http.MethodGet, routeFile+"design-draft-task.md?t="+testToken, nil, nil)
		want, err := os.ReadFile(filepath.Join(boardPath, "design-draft-task.md"))
		if err != nil {
			t.Fatalf("read the design file: %v", err)
		}
		if listed.Code != http.StatusOK || listed.Body.String() != string(want) || listed.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("listed file = %d %q %q; want 200, its text and text/plain", listed.Code, listed.Body.String(), listed.Header().Get("Content-Type"))
		}
		for _, name := range []string{"board.json", "a/b.md", "design-nope.md"} {
			recorder := call(server, http.MethodGet, routeFile+name+"?t="+testToken, nil, nil)
			if recorder.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d; want 404", name, recorder.Code)
			}
			requireHardenedHeaders(t, name, recorder)
		}
	})
}
