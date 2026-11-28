// server_test.go drives the board server's handler in process over a temp board.

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
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testToken = "0123456789abcdef"
	testPort  = 4567
	testHost  = "127.0.0.1:4567"
)

// fixture is a server over its own temp board.
type fixture struct {
	board  *boardengine.Board
	dir    string
	server *Server
}

// newFixture seeds a board with a draft, a released, a running and a hand-set task and a note with a body.
func newFixture(t *testing.T, pollBound time.Duration) fixture {
	t.Helper()
	dir := t.TempDir()
	now := func() time.Time { return time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC) }
	board := boardengine.New(boardengine.Config{
		Path: dir, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true, Clock: now,
		Types:  []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
		Labels: []boardengine.Label{{Name: "undecided"}},
	})
	for _, slug := range []string{"drafted", "released", "busy", "active"} {
		must(t, upsert(board, map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []any{"bug"}}))
	}
	must(t, upsert(board, map[string]any{"slug": "idea", "title": "idea", "kind": "note", "labels": []any{"bug"}, "body": "why"}))
	must(t, board.SetStatus("released", nil))
	running, active := boardengine.RunStatus("running", "Webster"), "active"
	must(t, board.SetStatus("busy", &running))
	must(t, board.SetStatus("active", &active))

	server, err := NewServer(ServerConfig{Board: board, BoardPath: dir, Token: testToken, Port: testPort, PollBound: pollBound, Now: now})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return fixture{board: board, dir: dir, server: server}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func upsert(board *boardengine.Board, fields map[string]any) error {
	_, err := board.UpsertTaskReporting(fields)
	return err
}

func (f fixture) boardJSON(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "board.json"))
	must(t, err)
	return string(data)
}

// entry returns the current projection of the entry slug.
func (f fixture) entry(t *testing.T, slug string) *boardengine.ViewEntry {
	t.Helper()
	view, err := f.board.View()
	must(t, err)
	return viewedEntry(view, slug)
}

// newRequest builds a request that passes every check, which a test then spoils.
func newRequest(method, target, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Host = testHost
	request.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://"+testHost)
	}
	return request
}

// serve runs request through the server and checks the headers every response carries.
func (f fixture) serve(t *testing.T, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.server.ServeHTTP(recorder, request)
	header := recorder.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("%s %s headers = %v; want no-store, nosniff and no CORS header", request.Method, request.URL, header)
	}
	return recorder
}

// answer is the JSON body of an API response.
type answer struct {
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error"`
	Unchanged bool                   `json:"unchanged"`
	Revision  string                 `json:"revision"`
	Entry     *boardengine.ViewEntry `json:"entry"`
	Board     *boardengine.BoardView `json:"board"`
}

func decodeAnswer(t *testing.T, recorder *httptest.ResponseRecorder) answer {
	t.Helper()
	var got answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body, err)
	}
	return got
}

// write posts one write and returns its status and answer.
func (f fixture) write(t *testing.T, op, slug, revision string, fields map[string]any) (int, answer) {
	t.Helper()
	body, err := json.Marshal(writeRequest{Op: op, Slug: slug, Revision: revision, Fields: fields})
	must(t, err)
	recorder := f.serve(t, newRequest(http.MethodPost, routeWrite, string(body)))
	return recorder.Code, decodeAnswer(t, recorder)
}

func TestServer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TransportRefusals", testTransportRefusals)
	t.Run("PageAndPolicy", testPageAndPolicy)
	t.Run("ReadRoutes", testReadRoutes)
	t.Run("Ops", testOps)
	t.Run("RefusedWrites", testRefusedWrites)
	t.Run("LongPoll", testLongPoll)
}

func testTransportRefusals(t *testing.T) {
	t.Parallel()
	const edit = `{"op":"edit","slug":"drafted","revision":"x","fields":{"title":"T"}}`
	tests := []struct {
		name   string
		method string
		target string
		body   string
		spoil  func(r *http.Request)
		want   int
	}{
		{"WrongHost", http.MethodGet, routePage, "", func(r *http.Request) { r.Host = "evil.example:4567" }, http.StatusMisdirectedRequest},
		{"NoToken", http.MethodGet, routeBoard, "", func(r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
		{"WrongToken", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
		{"FileWithoutToken", http.MethodGet, routeFile + "proposal-idea.md", "", nil, http.StatusUnauthorized},
		{"FileWrongToken", http.MethodGet, routeFile + "proposal-idea.md?t=nope", "", nil, http.StatusUnauthorized},
		{"NotJSON", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"ForeignOrigin", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, http.StatusForbidden},
		{"BodyAboveCap", http.MethodPost, routeWrite, `{"op":"` + strings.Repeat("x", maxBodyBytes) + `"}`, nil, http.StatusRequestEntityTooLarge},
		{"UnknownKey", http.MethodPost, routeWrite, `{"op":"edit","extra":1}`, nil, http.StatusBadRequest},
		{"UnknownRoute", http.MethodGet, "/nothing", "", nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, time.Minute)
			before := f.boardJSON(t)
			request := newRequest(tt.method, tt.target, tt.body)
			if tt.spoil != nil {
				tt.spoil(request)
			}
			recorder := f.serve(t, request)
			if recorder.Code != tt.want || decodeAnswer(t, recorder).OK {
				t.Errorf("status = %d, body %s; want a %d refusal", recorder.Code, recorder.Body, tt.want)
			}
			if f.boardJSON(t) != before {
				t.Errorf("board.json changed")
			}
		})
	}
}

// inlineHash returns the policy source of the content between the first open and close tags of page.
func inlineHash(t *testing.T, page, open, closing string) string {
	t.Helper()
	_, rest, found := strings.Cut(page, open)
	content, _, ended := strings.Cut(rest, closing)
	if !found || !ended {
		t.Fatalf("page holds no %s element", open)
	}
	sum := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func testPageAndPolicy(t *testing.T) {
	t.Parallel()
	f := newFixture(t, time.Minute)
	request := newRequest(http.MethodGet, routePage, "")
	request.Header.Del("Authorization")
	recorder := f.serve(t, request)
	page := string(boardengine.ViewerPage())
	if recorder.Code != http.StatusOK || recorder.Body.String() != page {
		t.Errorf("GET / = %d, %d bytes; want the viewer page", recorder.Code, recorder.Body.Len())
	}
	want := "default-src 'none'; script-src " + inlineHash(t, page, "<script>", "</script>") +
		"; style-src " + inlineHash(t, page, "<style>", "</style>") +
		"; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	if got := recorder.Header().Get("Content-Security-Policy"); got != want {
		t.Errorf("Content-Security-Policy = %q; want %q", got, want)
	}
	for _, route := range []string{routePage, routeBoard, routeWrite, routeFile} {
		if !strings.Contains(page, route) {
			t.Errorf("the page does not hold the route %q", route)
		}
	}
}

func testReadRoutes(t *testing.T) {
	t.Parallel()
	f := newFixture(t, time.Minute)

	got := decodeAnswer(t, f.serve(t, newRequest(http.MethodGet, routeBoard, "")))
	want, err := f.board.View()
	must(t, err)
	revision, err := boardengine.BoardRevision(f.dir)
	must(t, err)
	// Compared as JSON, which is the form the page reads.
	gotJSON, err := json.Marshal(got.Board)
	must(t, err)
	wantJSON, err := json.Marshal(want)
	must(t, err)
	if !got.OK || got.Revision != revision || !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Errorf("GET /api/board = %s at %q; want %s at %q", gotJSON, got.Revision, wantJSON, revision)
	}

	file := f.serve(t, newRequest(http.MethodGet, routeFile+"proposal-idea.md?t="+testToken, ""))
	if file.Code != http.StatusOK || !strings.HasSuffix(file.Body.String(), "why") || file.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("design file = %d %q %q", file.Code, file.Header().Get("Content-Type"), file.Body)
	}
	for _, name := range []string{"board.json", "a/b.md"} {
		if got := f.serve(t, newRequest(http.MethodGet, routeFile+name+"?t="+testToken, "")); got.Code != http.StatusNotFound {
			t.Errorf("GET /file/%s = %d; want 404", name, got.Code)
		}
	}
}

func testOps(t *testing.T) {
	t.Parallel()
	draft := boardengine.StatusDraft
	tests := []struct {
		op     string
		slug   string
		fields map[string]any
		// direct makes the board write the matching CLI verb makes.
		direct func(board *boardengine.Board) error
	}{
		{opCreate, "fresh", map[string]any{"kind": "task", "title": "Fresh", "labels": []any{"bug"}}, func(b *boardengine.Board) error {
			return upsert(b, map[string]any{"slug": "fresh", "kind": "task", "title": "Fresh", "labels": []any{"bug"}})
		}},
		{opEdit, "released", map[string]any{"title": "Retitled"}, func(b *boardengine.Board) error {
			return upsert(b, map[string]any{"slug": "released", "title": "Retitled"})
		}},
		{opRelease, "drafted", nil, func(b *boardengine.Board) error {
			_, err := b.SetStatusReleasingProject("drafted", nil)
			return err
		}},
		{opPromote, "idea", nil, func(b *boardengine.Board) error {
			_, err := b.PromoteTo("idea", "", nil)
			return err
		}},
		{opDraft, "released", nil, func(b *boardengine.Board) error {
			_, err := b.SetStatusReleasingProject("released", &draft)
			return err
		}},
		{opUndraft, "drafted", nil, func(b *boardengine.Board) error {
			_, err := b.SetStatusReleasingProject("drafted", nil)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			t.Parallel()
			f, twin := newFixture(t, time.Minute), newFixture(t, time.Minute)
			revision := ""
			if tt.op != opCreate {
				revision = f.entry(t, tt.slug).Revision
			}
			status, got := f.write(t, tt.op, tt.slug, revision, tt.fields)
			must(t, tt.direct(twin.board))
			if status != http.StatusOK || !got.OK {
				t.Fatalf("%s = %d %q; want 200", tt.op, status, got.Error)
			}
			if f.boardJSON(t) != twin.boardJSON(t) {
				t.Errorf("board.json = %s; want %s", f.boardJSON(t), twin.boardJSON(t))
			}
			if !reflect.DeepEqual(got.Entry, f.entry(t, tt.slug)) {
				t.Errorf("entry = %+v; want the board's %+v", got.Entry, f.entry(t, tt.slug))
			}
			// The answered revision carries the next edit through.
			if status, next := f.write(t, opEdit, tt.slug, got.Entry.Revision, map[string]any{"brief": "next"}); status != http.StatusOK {
				t.Errorf("edit at the answered revision = %d %q; want 200", status, next.Error)
			}
		})
	}
}

func testRefusedWrites(t *testing.T) {
	t.Parallel()
	// The run lock's own message is what any writer of a held task gets.
	reference := newFixture(t, time.Minute)
	runLocked := upsert(reference.board, map[string]any{"slug": "busy", "title": "Retitled"})
	if runLocked == nil {
		t.Fatal("an edit of a running task was not refused")
	}
	const noteRefusal = `entry "idea" is a note, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status`

	tests := []struct {
		name     string
		op, slug string
		// revision is the revision to send; "current" stands for the entry's own.
		revision   string
		fields     map[string]any
		wantStatus int
		wantError  string
	}{
		{"DraftOfADraft", opDraft, "drafted", "current", nil, 422, `draft takes a task with no status; entry "drafted" has status "draft": way forward: release it first, or leave it`},
		{"UndraftOfReleased", opUndraft, "released", "current", nil, 422, `entry "released" has no status, so it is released already and nothing was written`},
		{"UndraftOfRunning", opUndraft, "busy", "current", nil, 422, `entry "busy" is held by a run (status "running · Webster"): way forward: clear it with lym board set-status`},
		{"UndraftOfOtherStatus", opUndraft, "active", "current", nil, 422, `undraft takes a draft task; entry "active" has status "active": way forward: release it with the page's release`},
		{"DraftOfNote", opDraft, "idea", "current", nil, 422, strings.Replace(noteRefusal, "%s", "draft", 1)},
		{"UndraftOfNote", opUndraft, "idea", "current", nil, 422, strings.Replace(noteRefusal, "%s", "undraft", 1)},
		{"ReleaseOfNote", opRelease, "idea", "current", nil, 422, strings.Replace(noteRefusal, "%s", "release", 1)},
		{"ReleaseOfRunning", opRelease, "busy", "current", nil, 422, `entry "busy" is held by a run (status "running · Webster"), and a release from the page would let its parent's dobby start it while the run holds its pair: way forward: release it with lym board set-status`},
		{"PromoteOfTask", opPromote, "released", "current", nil, 422, `entry "released" is a task already, and only a note is promoted: way forward: edit it, or promote a note`},
		{"EditWithoutRevision", opEdit, "absent", "", map[string]any{"title": "T"}, 422, `edit of entry "absent" carries no revision, so nothing was written: way forward: reload the board and act on what it holds now`},
		{"UnknownOp", "remove", "drafted", "current", nil, 422, `unknown op "remove": way forward: send one of create, edit, release, promote, draft, undraft`},
		{"UnknownField", opEdit, "drafted", "current", map[string]any{"status": "done"}, 422, `edit takes no field "status": way forward: send only title, labels, brief, body, project, depends_on`},
		{"EditOfRunning", opEdit, "busy", "current", map[string]any{"title": "Retitled"}, 422, runLocked.Error()},
		{"CreateOfExisting", opCreate, "drafted", "", map[string]any{"title": "T"}, 409, `entry "drafted" already exists, so nothing was written: way forward: pick another slug, or edit the existing entry`},
		{"StaleEdit", opEdit, "drafted", "older", map[string]any{"title": "T"}, 409, `entry "drafted" changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again`},
		{"StaleRelease", opRelease, "drafted", "older", nil, 409, `entry "drafted" changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again`},
		{"ReleaseOfRemoved", opRelease, "gone", "older", nil, 409, `entry "gone" was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, time.Minute)
			before := f.boardJSON(t)
			revision := tt.revision
			if revision == "current" {
				revision = f.entry(t, tt.slug).Revision
			}
			status, got := f.write(t, tt.op, tt.slug, revision, tt.fields)
			if status != tt.wantStatus || got.OK || got.Error != tt.wantError {
				t.Errorf("answer = %d %q; want %d %q", status, got.Error, tt.wantStatus, tt.wantError)
			}
			if tt.wantStatus == http.StatusConflict && !reflect.DeepEqual(got.Entry, f.entry(t, tt.slug)) {
				t.Errorf("entry = %+v; want the board's %+v", got.Entry, f.entry(t, tt.slug))
			}
			if f.boardJSON(t) != before {
				t.Errorf("board.json changed")
			}
		})
	}
}

func testLongPoll(t *testing.T) {
	t.Parallel()
	// poll starts a long poll at the server's own revision and returns the channel its answer arrives on.
	poll := func(t *testing.T, f fixture) <-chan *httptest.ResponseRecorder {
		t.Helper()
		revision, err := boardengine.BoardRevision(f.dir)
		must(t, err)
		answered := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			recorder := httptest.NewRecorder()
			f.server.ServeHTTP(recorder, newRequest(http.MethodGet, routeBoard+"?after="+revision, ""))
			answered <- recorder
		}()
		return answered
	}

	t.Run("BoundPasses", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		if got := decodeAnswer(t, <-poll(t, f)); !got.Unchanged || got.Board != nil {
			t.Errorf("answer = %+v; want unchanged", got)
		}
	})

	t.Run("ChangeAnswersTheBoard", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		answered := poll(t, f)
		f.server.BoardChanged("next")
		if got := decodeAnswer(t, <-answered); got.Unchanged || got.Revision != "next" || got.Board == nil {
			t.Errorf("answer = %+v; want the board at revision next", got)
		}
	})

	t.Run("DrainAnswersUnchanged", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Hour)
		answered := poll(t, f)
		f.server.beginDrain()
		if got := decodeAnswer(t, <-answered); !got.Unchanged {
			t.Errorf("answer = %+v; want unchanged", got)
		}
		if got := f.server.lastAPIRequest(); got.IsZero() {
			t.Errorf("lastAPIRequest() is zero after an /api request")
		}
	})
}
