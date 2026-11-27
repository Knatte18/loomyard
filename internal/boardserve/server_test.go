// server_test.go drives the board server's handler in process, over a temporary board.

package boardserve

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
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
	testToken = "0123456789abcdef"
	testHost  = "127.0.0.1:4711"
)

// testNow is the one clock of the boards and servers under test, so twin boards stamp the same dates.
var testNow = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

// seededBoard returns a board in a temporary directory and the directory.
// It holds a draft, a released, a parked and a run-held task, and a note.
func seededBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	dir := t.TempDir()
	board := boardengine.New(boardengine.Config{
		Path:         dir,
		Readme:       "README.md",
		DesignPrefix: "design-",
		Types:        []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
		SkipGit:      true,
		Clock:        func() time.Time { return testNow },
	})
	upsert := func(slug, kind, body string) {
		fields := map[string]any{"slug": slug, "title": slug, "kind": kind, "labels": []string{"bug"}, "body": body}
		if _, err := board.UpsertTask(fields); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}
	upsert("drafted", boardengine.KindTask, "Drafted design.")
	upsert("released", boardengine.KindTask, "")
	upsert("parked", boardengine.KindTask, "")
	upsert("running", boardengine.KindTask, "")
	upsert("idea", boardengine.KindNote, "")
	parked := "parked"
	for slug, status := range map[string]*string{"released": nil, "parked": &parked} {
		if err := board.SetStatus(slug, status); err != nil {
			t.Fatalf("seed status of %s: %v", slug, err)
		}
	}
	if err := board.SetRunStatus("running", boardengine.RunStatus("running", "Webster")); err != nil {
		t.Fatalf("seed the run status: %v", err)
	}
	return board, dir
}

// newTestServer returns a server over board with the long-poll bound longPoll.
func newTestServer(t *testing.T, board *boardengine.Board, dir string, longPoll time.Duration) *Server {
	t.Helper()
	server, err := NewServer(ServerConfig{
		Board:     board,
		BoardPath: dir,
		Token:     testToken,
		Port:      4711,
		LongPoll:  longPoll,
		Now:       func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server
}

// serve sends one request carrying the token, after change has had its say.
func serve(server *Server, method, path, body string, change func(*http.Request)) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, "http://"+testHost+path, reader)
	request.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if change != nil {
		change(request)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

// answer is the union of the server's JSON answers.
type answer struct {
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error"`
	Revision  string                 `json:"revision"`
	Unchanged bool                   `json:"unchanged"`
	Board     boardengine.BoardView  `json:"board"`
	Entry     *boardengine.ViewEntry `json:"entry"`
	Warning   string                 `json:"warning"`
}

func decodeAnswer(t *testing.T, recorder *httptest.ResponseRecorder) answer {
	t.Helper()
	var decoded answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode answer %q: %v", recorder.Body.String(), err)
	}
	return decoded
}

func storeBytes(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	return string(data)
}

func revisionOf(t *testing.T, board *boardengine.Board, slug string) string {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	entry := entryOf(view, slug)
	if entry == nil {
		t.Fatalf("the board holds no entry %q", slug)
	}
	return entry.Revision
}

func writeBody(t *testing.T, request writeRequest) string {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestServer pins the handler's checks, routes, writes and long poll.
func TestServer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TransportRefusalsChangeNothing", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		server := newTestServer(t, board, dir, time.Minute)
		edit := writeBody(t, writeRequest{Op: opEdit, Slug: "drafted", Revision: revisionOf(t, board, "drafted"), Fields: map[string]any{"title": "Changed"}})
		tests := []struct {
			name, method, path, body string
			change                   func(*http.Request)
			want                     int
		}{
			{"WrongHost", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Host = "localhost:4711" }, http.StatusMisdirectedRequest},
			{"NoToken", http.MethodGet, routeBoard, "", func(r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
			{"WrongToken", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
			{"FileWithoutToken", http.MethodGet, routeFile + "design-drafted.md", "", nil, http.StatusUnauthorized},
			{"FileWithWrongToken", http.MethodGet, routeFile + "design-drafted.md?t=nope", "", nil, http.StatusUnauthorized},
			{"NotJSON", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
			{"ForeignOrigin", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Origin", "http://example.test") }, http.StatusForbidden},
			{"BodyAboveTheCap", http.MethodPost, routeWrite, strings.Repeat(" ", maxBodyBytes) + edit, nil, http.StatusRequestEntityTooLarge},
			{"UnknownKey", http.MethodPost, routeWrite, `{"op":"edit","slug":"drafted","force":true}`, nil, http.StatusBadRequest},
			{"WrongMethod", http.MethodGet, routeWrite, "", nil, http.StatusMethodNotAllowed},
		}
		before := storeBytes(t, dir)
		for _, tt := range tests {
			recorder := serve(server, tt.method, tt.path, tt.body, tt.change)
			if recorder.Code != tt.want || decodeAnswer(t, recorder).OK {
				t.Errorf("%s: status %d body %s; want a refusal with %d", tt.name, recorder.Code, recorder.Body, tt.want)
			}
			assertHeaders(t, tt.name, recorder)
			if after := storeBytes(t, dir); after != before {
				t.Errorf("%s changed board.json", tt.name)
			}
		}
	})

	t.Run("PageAndPolicy", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		server := newTestServer(t, board, dir, time.Minute)
		recorder := serve(server, http.MethodGet, routePage, "", func(r *http.Request) { r.Header.Del("Authorization") })
		page := string(boardengine.ViewerPage())
		if recorder.Code != http.StatusOK || recorder.Body.String() != page {
			t.Fatalf("GET / = %d; want 200 with ViewerPage()", recorder.Code)
		}
		assertHeaders(t, "page", recorder)
		assertHeaders(t, "not found", serve(server, http.MethodGet, "/nothing", "", nil))

		policy := recorder.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "'unsafe-inline'") {
			t.Errorf("policy %q admits unsafe-inline", policy)
		}
		for element, directive := range map[string]string{"script": "script-src", "style": "style-src"} {
			content := regexp.MustCompile(`(?s)<` + element + `>(.*?)</` + element + `>`).FindStringSubmatch(page)
			if content == nil {
				t.Fatalf("the page holds no inline %s", element)
			}
			sum := sha256.Sum256([]byte(content[1]))
			want := directive + " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"
			if !strings.Contains(policy, want) {
				t.Errorf("policy %q lacks %q", policy, want)
			}
		}
		for _, route := range []string{routePage, routeBoard, routeWrite, routeFile} {
			if !strings.Contains(page, route) {
				t.Errorf("the page does not spell the route %q", route)
			}
		}
	})

	t.Run("Board", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		recorder := serve(newTestServer(t, board, dir, time.Minute), http.MethodGet, routeBoard, "", nil)
		assertHeaders(t, "board", recorder)
		want, err := board.View()
		if err != nil {
			t.Fatal(err)
		}
		got := decodeAnswer(t, recorder)
		got.Board.Rendered = want.Rendered
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got.Board)
		if !got.OK || string(gotJSON) != string(wantJSON) {
			t.Errorf("GET %s = %s; want %s", routeBoard, gotJSON, wantJSON)
		}
	})

	t.Run("OpsWriteWhatTheCommandLineWrites", func(t *testing.T) {
		t.Parallel()
		draft := boardengine.StatusDraft
		tests := []struct {
			name    string
			request writeRequest
			// direct makes the board call the matching command line verb makes.
			direct func(*boardengine.Board) error
		}{
			{
				"Create",
				writeRequest{Op: opCreate, Slug: "fresh", Fields: map[string]any{"kind": "task", "title": "Fresh", "labels": []any{"bug"}}},
				func(b *boardengine.Board) error {
					_, err := b.UpsertTaskReporting(map[string]any{"slug": "fresh", "kind": "task", "title": "Fresh", "labels": []any{"bug"}})
					return err
				},
			},
			{
				"Edit",
				writeRequest{Op: opEdit, Slug: "drafted", Fields: map[string]any{"title": "Changed"}},
				func(b *boardengine.Board) error {
					_, err := b.UpsertTaskReporting(map[string]any{"slug": "drafted", "title": "Changed"})
					return err
				},
			},
			{
				"Release",
				writeRequest{Op: opRelease, Slug: "parked"},
				func(b *boardengine.Board) error {
					_, err := b.SetStatusReleasingProject("parked", nil)
					return err
				},
			},
			{
				"Promote",
				writeRequest{Op: opPromote, Slug: "idea"},
				func(b *boardengine.Board) error {
					_, err := b.PromoteTo("idea", "", nil)
					return err
				},
			},
			{
				"Draft",
				writeRequest{Op: opDraft, Slug: "released"},
				func(b *boardengine.Board) error {
					_, err := b.SetStatusReleasingProject("released", &draft)
					return err
				},
			},
			{
				"Undraft",
				writeRequest{Op: opUndraft, Slug: "drafted"},
				func(b *boardengine.Board) error {
					_, err := b.SetStatusReleasingProject("drafted", nil)
					return err
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				board, dir := seededBoard(t)
				twin, twinDir := seededBoard(t)
				server := newTestServer(t, board, dir, time.Minute)
				request := tt.request
				if request.Op != opCreate {
					request.Revision = revisionOf(t, board, request.Slug)
				}

				recorder := serve(server, http.MethodPost, routeWrite, writeBody(t, request), nil)
				if err := tt.direct(twin); err != nil {
					t.Fatalf("direct write: %v", err)
				}

				got := decodeAnswer(t, recorder)
				if recorder.Code != http.StatusOK || !got.OK || got.Entry == nil {
					t.Fatalf("write = %d %s; want the fresh entry", recorder.Code, recorder.Body)
				}
				if got.Entry.Revision != revisionOf(t, board, request.Slug) {
					t.Errorf("answered revision %s is not the entry's current one", got.Entry.Revision)
				}
				if server, direct := storeBytes(t, dir), storeBytes(t, twinDir); server != direct {
					t.Errorf("board.json after the server's write:\n%s\nafter the direct call:\n%s", server, direct)
				}

				// The answered revision carries the next edit.
				if tt.name == "Edit" {
					next := writeRequest{Op: opEdit, Slug: request.Slug, Revision: got.Entry.Revision, Fields: map[string]any{"title": "Again"}}
					if again := serve(server, http.MethodPost, routeWrite, writeBody(t, next), nil); again.Code != http.StatusOK {
						t.Errorf("second edit = %d %s; want it saved", again.Code, again.Body)
					}
				}
			})
		}
	})

	t.Run("RefusedWritesChangeNothing", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		server := newTestServer(t, board, dir, time.Minute)
		_, runLock := board.UpsertTaskReporting(map[string]any{"slug": "running", "title": "Changed"})
		if runLock == nil {
			t.Fatal("an edit of a running task was accepted")
		}
		for _, slug := range []string{"ship", "shipped"} {
			if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": boardengine.KindProject, "labels": []string{"bug"}}); err != nil {
				t.Fatalf("seed project %s: %v", slug, err)
			}
		}
		done := "done"
		if err := board.SetStatus("shipped", &done); err != nil {
			t.Fatalf("seed the done project: %v", err)
		}
		const current = "current"
		tests := []struct {
			name     string
			op, slug string
			// revision is current for the entry's own.
			revision  string
			fields    map[string]any
			status    int
			wantError string
		}{
			{"DraftWithAStatus", opDraft, "parked", current, nil, 422, `draft takes a task with no status; entry "parked" has status "parked"`},
			{"UndraftWithoutAStatus", opUndraft, "released", current, nil, 422, "released already"},
			{"UndraftOfARunningTask", opUndraft, "running", current, nil, 422, "way forward: clear it with lym board set-status"},
			{"UndraftOfAnotherStatus", opUndraft, "parked", current, nil, 422, "way forward: release it with the page's release"},
			{"DraftOfANote", opDraft, "idea", current, nil, 422, "the page's draft takes a task"},
			{"UndraftOfANote", opUndraft, "idea", current, nil, 422, "the page's undraft takes a task"},
			{"ReleaseOfARunningTask", opRelease, "running", current, nil, 422, "way forward: release it with lym board set-status"},
			{"ReleaseOfANote", opRelease, "idea", current, nil, 422, "the page's release takes a task"},
			{"PromoteOfATask", opPromote, "drafted", current, nil, 422, "is a task already"},
			{"EditWithoutARevision", opEdit, "absent", "", nil, 422, "carries no revision"},
			{"UnknownOp", "remove", "drafted", current, nil, 422, "create, edit, release, promote, draft, undraft"},
			{"UnknownField", opEdit, "drafted", current, map[string]any{"status": "done"}, 422, "title, labels, brief, body, project, depends_on"},
			{"CreateOfAnExistingSlug", opCreate, "drafted", "", nil, 409, "already exists"},
			{"EditOfARunningTask", opEdit, "running", current, map[string]any{"title": "Changed"}, 422, runLock.Error()},
			{"StaleEdit", opEdit, "drafted", "older", map[string]any{"title": "Changed"}, 409, "changed since the page read it"},
			{"StaleRelease", opRelease, "parked", "older", nil, 409, "changed since the page read it"},
			{"ReleaseOfARemovedEntry", opRelease, "absent", "older", nil, 409, "was removed"},
			{"EmptySlug", opEdit, "", "older", nil, 422, "slug must be a non-empty string"},
			{"ReleaseOfAProject", opRelease, "ship", "older", nil, 422, "the page's release takes a task"},
			{"DraftOfAProject", opDraft, "ship", "older", nil, 422, "the page's draft takes a task"},
			{"UndraftOfAProject", opUndraft, "ship", "older", nil, 422, "the page's undraft takes a task"},
			{"EditOfAProject", opEdit, "ship", "older", map[string]any{"title": "Changed"}, 422, "the page edits a task or a note"},
			{"EditOfADoneProject", opEdit, "shipped", "older", map[string]any{"title": "Changed"}, 422, "the page edits a task or a note"},
			{"CreateOfAProject", opCreate, "epic", "", map[string]any{"kind": "project", "title": "Epic"}, 422, "the page creates a task or a note"},
			{"CreateOfAnUnknownKind", opCreate, "epic", "", map[string]any{"kind": "chore", "title": "Epic"}, 422, "the page creates a task or a note"},
		}
		before := storeBytes(t, dir)
		for _, tt := range tests {
			revision := tt.revision
			if revision == current {
				revision = revisionOf(t, board, tt.slug)
			}
			recorder := serve(server, http.MethodPost, routeWrite, writeBody(t, writeRequest{Op: tt.op, Slug: tt.slug, Revision: revision, Fields: tt.fields}), nil)
			got := decodeAnswer(t, recorder)
			if recorder.Code != tt.status || got.OK || !strings.Contains(got.Error, tt.wantError) {
				t.Errorf("%s = %d %q; want %d naming %q", tt.name, recorder.Code, got.Error, tt.status, tt.wantError)
			}
			if tt.status == http.StatusConflict {
				removed := tt.slug == "absent"
				if removed != (got.Entry == nil) || (!removed && got.Entry.Revision != revisionOf(t, board, tt.slug)) {
					t.Errorf("%s answered entry %+v; want the fresh entry, or null for a removed one", tt.name, got.Entry)
				}
			}
			if after := storeBytes(t, dir); after != before {
				t.Errorf("%s changed board.json", tt.name)
			}
		}
	})

	t.Run("PromoteAnswersTheRecordWarning", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		board := boardengine.New(boardengine.Config{
			Path:         dir,
			Readme:       "README.md",
			DesignPrefix: "design-",
			Types:        []boardengine.Label{{Name: boardengine.SketchType}},
			SkipGit:      true,
		})
		note := map[string]any{"slug": "plan", "title": "Plan", "kind": boardengine.KindNote, "labels": []string{boardengine.SketchType}, "body": "First."}
		if _, err := board.UpsertTask(note); err != nil {
			t.Fatalf("seed the note: %v", err)
		}
		if err := board.StoreDecisionRecord("plan", []byte("The record.")); err != nil {
			t.Fatalf("store the record: %v", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": "plan", "body": "Outgrown."}); err != nil {
			t.Fatalf("outgrow the record: %v", err)
		}

		request := writeBody(t, writeRequest{Op: opPromote, Slug: "plan", Revision: revisionOf(t, board, "plan")})
		recorder := serve(newTestServer(t, board, dir, time.Minute), http.MethodPost, routeWrite, request, nil)

		got := decodeAnswer(t, recorder)
		if !got.OK || !strings.Contains(got.Warning, "changed after its decision record was stored") {
			t.Errorf("promote = %d %s; want success with the record warning", recorder.Code, recorder.Body)
		}
	})

	t.Run("LongPoll", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		revision, err := boardengine.BoardRevision(dir)
		if err != nil {
			t.Fatal(err)
		}
		waitPath := routeBoard + "?after=" + revision

		bounded := decodeAnswer(t, serve(newTestServer(t, board, dir, time.Millisecond), http.MethodGet, waitPath, "", nil))
		if want := (answer{OK: true, Unchanged: true, Revision: revision}); !reflect.DeepEqual(bounded, want) {
			t.Errorf("poll past its bound = %+v; want %+v", bounded, want)
		}

		server := newTestServer(t, board, dir, time.Minute)
		poll := func() <-chan *httptest.ResponseRecorder {
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- serve(server, http.MethodGet, waitPath, "", nil) }()
			return done
		}

		waiting := poll()
		if _, err := board.UpsertTask(map[string]any{"slug": "drafted", "title": "Moved"}); err != nil {
			t.Fatal(err)
		}
		server.BoardChanged("next")
		changed := decodeAnswer(t, <-waiting)
		if changed.Unchanged || changed.Revision != "next" || len(changed.Board.Entries) == 0 {
			t.Errorf("poll across a change = %+v; want the new projection", changed)
		}

		waitPath = routeBoard + "?after=next"
		waiting = poll()
		server.beginDrain()
		drained := decodeAnswer(t, <-waiting)
		if !drained.Unchanged || drained.Revision != "next" {
			t.Errorf("drained poll = %+v; want unchanged", drained)
		}
		if got := server.lastAPIRequest(); !got.Equal(testNow) {
			t.Errorf("lastAPIRequest() = %v; want %v", got, testNow)
		}
	})

	t.Run("Files", func(t *testing.T) {
		t.Parallel()
		board, dir := seededBoard(t)
		if err := board.StoreDecisionRecord("drafted", []byte("Drafted record.")); err != nil {
			t.Fatalf("store the record: %v", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": "ship", "title": "Ship", "kind": boardengine.KindProject, "labels": []string{"bug"}, "body": "Ship design."}); err != nil {
			t.Fatalf("seed the project: %v", err)
		}
		server := newTestServer(t, board, dir, time.Minute)
		tests := []struct {
			name string
			code int
			// ending closes a served file's content.
			ending string
		}{
			{"design-drafted.md", 200, "Drafted design."},
			{"decision-drafted.md", 200, "Drafted record."},
			{"design-ship.md", 200, "Ship design."},
			{"board.json", 404, ""},
			{"a/b.md", 404, ""},
		}
		for _, tt := range tests {
			recorder := serve(server, http.MethodGet, routeFile+tt.name+"?t="+testToken, "", nil)
			if recorder.Code != tt.code {
				t.Errorf("GET %s%s = %d; want %d", routeFile, tt.name, recorder.Code, tt.code)
			}
			assertHeaders(t, tt.name, recorder)
			if tt.code == 200 && !strings.HasSuffix(recorder.Body.String(), tt.ending) {
				t.Errorf("GET %s%s = %q; want content ending %q", routeFile, tt.name, recorder.Body, tt.ending)
			}
		}
	})
}

// assertHeaders checks the headers every response carries and the one none does.
func assertHeaders(t *testing.T, name string, recorder *httptest.ResponseRecorder) {
	t.Helper()
	header := recorder.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("%s headers = %v; want no-store, nosniff and no CORS header", name, header)
	}
}
