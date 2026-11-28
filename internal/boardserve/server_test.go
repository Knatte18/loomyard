// server_test.go drives the board server's handler in process over temp boards with git skipped.

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
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testToken = "secret-token"
	testPort  = 4321
	runStatus = "running · Build"
)

var testNow = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

// fixture is one board with a server over it.
type fixture struct {
	board  *boardengine.Board
	dir    string
	server *Server
}

// newFixture seeds a board holding an entry of every state the page's operations read.
// Two fixtures hold the same board bytes, since the board's clock is fixed.
func newFixture(t *testing.T, longPoll time.Duration) fixture {
	t.Helper()
	dir := t.TempDir()
	board := boardengine.New(boardengine.Config{
		Path: dir, Readme: "README.md", DesignPrefix: "design-", DefaultRecipe: "darn",
		Types:   []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
		Labels:  []boardengine.Label{{Name: "ui"}},
		SkipGit: true,
		Clock:   func() time.Time { return testNow },
	})
	for _, seed := range []struct{ slug, kind, status string }{
		{"a-note", "note", ""},
		{"released", "task", ""},
		{"drafted", "task", "draft"},
		{"busy", "task", runStatus},
		{"worded", "task", "waiting"},
	} {
		fields := map[string]any{"slug": seed.slug, "title": seed.slug, "kind": seed.kind, "labels": []string{"bug"}}
		if seed.slug == "released" {
			fields["body"] = "The design."
		}
		if _, err := board.UpsertTask(fields); err != nil {
			t.Fatalf("seed %s: %v", seed.slug, err)
		}
		if seed.kind != "task" || seed.status == "draft" {
			continue
		}
		var status *string
		if seed.status != "" {
			status = &seed.status
		}
		if err := board.SetStatus(seed.slug, status); err != nil {
			t.Fatalf("seed status of %s: %v", seed.slug, err)
		}
	}
	server, err := NewServer(ServerConfig{Board: board, BoardPath: dir, Token: testToken, Port: testPort, LongPoll: longPoll, Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return fixture{board: board, dir: dir, server: server}
}

// serve sends one request as the page does, after change has altered it, and checks the headers every response carries.
func (f fixture) serve(t *testing.T, method, target, body string, change func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:4321"+target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if change != nil {
		change(request)
	}
	response := httptest.NewRecorder()
	f.server.ServeHTTP(response, request)
	header := response.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("%s %s headers = %v; want no-store, nosniff and no CORS header", method, target, header)
	}
	return response
}

// reply is the JSON body of an API response.
type reply struct {
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error"`
	Unchanged bool                   `json:"unchanged"`
	Revision  string                 `json:"revision"`
	Board     *boardengine.BoardView `json:"board"`
	Entry     *boardengine.ViewEntry `json:"entry"`
}

func decodeReply(t *testing.T, response *httptest.ResponseRecorder) reply {
	t.Helper()
	var decoded reply
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", response.Body, err)
	}
	return decoded
}

// write posts one write and returns the status and decoded answer.
func (f fixture) write(t *testing.T, op, slug, revision string, fields map[string]any) (int, reply) {
	t.Helper()
	body, err := json.Marshal(writeRequest{Op: op, Slug: slug, Revision: revision, Fields: fields})
	if err != nil {
		t.Fatalf("encode write: %v", err)
	}
	response := f.serve(t, http.MethodPost, routeWrite, string(body), nil)
	return response.Code, decodeReply(t, response)
}

// entry is the projection's entry of slug, nil when there is none.
func (f fixture) entry(t *testing.T, slug string) *boardengine.ViewEntry {
	t.Helper()
	_, entry, err := f.server.viewEntry(slug)
	if err != nil {
		t.Fatalf("view %s: %v", slug, err)
	}
	return entry
}

// revisionOf is the revision of slug, "" when the board holds no such entry.
func (f fixture) revisionOf(t *testing.T, slug string) string {
	t.Helper()
	if entry := f.entry(t, slug); entry != nil {
		return entry.Revision
	}
	return ""
}

func (f fixture) boardJSON(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	return data
}

func encoded(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(data)
}

func TestServer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TransportRefusalsChangeNothing", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		before := f.boardJSON(t)
		edit := encoded(t, writeRequest{Op: opEdit, Slug: "released", Revision: f.revisionOf(t, "released"), Fields: map[string]any{"title": "Changed"}})
		large := `{"op":"` + strings.Repeat("x", maxBodyBytes) + `"}`
		tests := []struct {
			name, method, target, body string
			change                     func(*http.Request)
			want                       int
		}{
			{"WrongHost", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Host = "localhost:4321" }, http.StatusMisdirectedRequest},
			{"NoToken", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
			{"WrongToken", http.MethodGet, routeBoard, "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer other") }, http.StatusUnauthorized},
			{"FileWithoutToken", http.MethodGet, routeFile + "design-released.md", "", nil, http.StatusUnauthorized},
			{"FileWithWrongToken", http.MethodGet, routeFile + "design-released.md?t=other", "", nil, http.StatusUnauthorized},
			{"NotJSON", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
			{"ForeignOrigin", http.MethodPost, routeWrite, edit, func(r *http.Request) { r.Header.Set("Origin", "http://example.test") }, http.StatusForbidden},
			{"BodyAboveTheCap", http.MethodPost, routeWrite, large, nil, http.StatusRequestEntityTooLarge},
			{"UnknownRequestKey", http.MethodPost, routeWrite, `{"op":"edit","extra":1}`, nil, http.StatusUnprocessableEntity},
			{"UnknownPath", http.MethodGet, "/other", "", nil, http.StatusNotFound},
		}
		for _, tt := range tests {
			response := f.serve(t, tt.method, tt.target, tt.body, tt.change)
			if response.Code != tt.want || decodeReply(t, response).OK {
				t.Errorf("%s: status %d, body %s; want %d and a refusal", tt.name, response.Code, response.Body, tt.want)
			}
		}
		if after := f.boardJSON(t); !bytes.Equal(before, after) {
			t.Errorf("board.json changed by a refused request")
		}
	})

	t.Run("PageCarriesItsOwnPolicy", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		response := f.serve(t, http.MethodGet, routePage, "", func(r *http.Request) { r.Header.Del("Authorization") })
		page := string(boardengine.ViewerPage())
		if response.Code != http.StatusOK || response.Body.String() != page || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET / = %d %q; want the viewer page as text/html", response.Code, response.Header().Get("Content-Type"))
		}
		policy := response.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "'unsafe-inline'") {
			t.Errorf("policy = %q; want no 'unsafe-inline'", policy)
		}
		for element, directive := range map[string]string{"script": "script-src", "style": "style-src"} {
			_, rest, found := strings.Cut(page, "<"+element+">")
			content, _, closed := strings.Cut(rest, "</"+element+">")
			if !found || !closed {
				t.Fatalf("page holds no inline %s element", element)
			}
			sum := sha256.Sum256([]byte(content))
			want := directive + " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"
			if !strings.Contains(policy, want) {
				t.Errorf("policy = %q; want it to hold %q", policy, want)
			}
		}
		for _, route := range []string{routePage, routeBoard, routeWrite, routeFile} {
			if !strings.Contains(page, route) {
				t.Errorf("page does not hold the route %q", route)
			}
		}
	})

	t.Run("BoardIsTheProjection", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		later := testNow.Add(time.Hour)
		f.server.config.Now = func() time.Time { return later }
		response := f.serve(t, http.MethodGet, routeBoard, "", nil)
		got := decodeReply(t, response)
		want, err := f.board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		revision, err := boardengine.BoardRevision(f.dir)
		if err != nil {
			t.Fatalf("BoardRevision: %v", err)
		}
		if !got.OK || got.Board == nil || got.Revision != revision || encoded(t, *got.Board) != encoded(t, want) {
			t.Errorf("GET /api/board = %s; want the board's view at revision %s", response.Body, revision)
		}
		if last := f.server.lastAPIRequest(); !last.Equal(later) {
			t.Errorf("lastAPIRequest() = %v; want the request's time %v", last, later)
		}
	})

	t.Run("OperationsWriteAsTheCLIDoes", func(t *testing.T) {
		t.Parallel()
		draft := boardengine.StatusDraft
		upsert := func(fields map[string]any) func(*boardengine.Board) error {
			return func(b *boardengine.Board) error {
				_, err := b.UpsertTaskReporting(fields)
				return err
			}
		}
		setStatus := func(slug string, status *string) func(*boardengine.Board) error {
			return func(b *boardengine.Board) error {
				_, err := b.SetStatusReleasingProject(slug, status)
				return err
			}
		}
		tests := []struct {
			op, slug string
			fields   map[string]any
			cli      func(*boardengine.Board) error
		}{
			{opCreate, "fresh", map[string]any{"kind": "task", "title": "Fresh", "labels": []any{"bug"}, "depends_on": []any{"released"}},
				upsert(map[string]any{"slug": "fresh", "kind": "task", "title": "Fresh", "labels": []any{"bug"}, "depends_on": []any{"released"}})},
			{opEdit, "released", map[string]any{"title": "Edited"}, upsert(map[string]any{"slug": "released", "title": "Edited"})},
			{opRelease, "drafted", nil, setStatus("drafted", nil)},
			{opPromote, "a-note", nil, func(b *boardengine.Board) error {
				_, err := b.PromoteTo("a-note", "", nil)
				return err
			}},
			{opDraft, "released", nil, setStatus("released", &draft)},
			{opUndraft, "drafted", nil, setStatus("drafted", nil)},
		}
		for _, tt := range tests {
			f, twin := newFixture(t, time.Millisecond), newFixture(t, time.Millisecond)
			revision := ""
			if tt.op != opCreate {
				revision = f.revisionOf(t, tt.slug)
			}
			status, got := f.write(t, tt.op, tt.slug, revision, tt.fields)
			if err := tt.cli(twin.board); err != nil {
				t.Fatalf("%s: the twin write: %v", tt.op, err)
			}
			if status != http.StatusOK || !got.OK || got.Entry == nil {
				t.Errorf("%s = %d %+v; want 200 with the entry", tt.op, status, got)
				continue
			}
			if !bytes.Equal(f.boardJSON(t), twin.boardJSON(t)) {
				t.Errorf("%s: board.json = %s; want %s", tt.op, f.boardJSON(t), twin.boardJSON(t))
			}
			if encoded(t, got.Entry) != encoded(t, f.entry(t, tt.slug)) {
				t.Errorf("%s: answered entry = %+v; want the fresh one", tt.op, got.Entry)
			}
			next, saved := f.write(t, opEdit, tt.slug, got.Entry.Revision, map[string]any{"brief": "Again"})
			if tt.slug != "busy" && next != http.StatusOK {
				t.Errorf("%s: an edit with the answered revision = %d %q; want it saved", tt.op, next, saved.Error)
			}
		}
	})

	t.Run("RefusedWritesChangeNothing", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		twin := newFixture(t, time.Millisecond)
		_, runLocked := twin.board.UpsertTaskReporting(map[string]any{"slug": "busy", "title": "Other"})
		if runLocked == nil {
			t.Fatalf("the CLI's edit of a running task was not refused")
		}
		before := f.boardJSON(t)
		tests := []struct {
			op, slug string
			fields   map[string]any
			// message is what the 422 refusal holds.
			message string
		}{
			{opDraft, "drafted", nil, "draft takes a task with no status"},
			{opUndraft, "released", nil, "has no status, so it is released already"},
			{opUndraft, "busy", nil, "is held by a run"},
			{opUndraft, "worded", nil, "undraft takes a draft task"},
			{opDraft, "a-note", nil, "takes a task"},
			{opUndraft, "a-note", nil, "takes a task"},
			{opRelease, "a-note", nil, "takes a task"},
			{opRelease, "busy", nil, "lym board set-status"},
			{opPromote, "released", nil, "is a task already"},
			{opEdit, "absent", map[string]any{"title": "Other"}, "carries no revision"},
			{"remove", "released", nil, "create, edit, release, promote, draft, undraft"},
			{opEdit, "released", map[string]any{"kind": "note"}, `takes no field "kind"`},
			{opEdit, "busy", map[string]any{"title": "Other"}, runLocked.Error()},
		}
		for _, tt := range tests {
			status, got := f.write(t, tt.op, tt.slug, f.revisionOf(t, tt.slug), tt.fields)
			if status != http.StatusUnprocessableEntity || got.OK || !strings.Contains(got.Error, tt.message) {
				t.Errorf("%s %s = %d %q; want 422 holding %q", tt.op, tt.slug, status, got.Error, tt.message)
			}
		}
		if after := f.boardJSON(t); !bytes.Equal(before, after) {
			t.Errorf("board.json changed by a refused write")
		}
	})

	t.Run("StaleWritesAnswerTheFreshEntry", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		older := f.revisionOf(t, "released")
		if err := f.board.SetStatus("released", nil); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		if _, err := f.board.UpsertTask(map[string]any{"slug": "released", "title": "Moved on"}); err != nil {
			t.Fatalf("UpsertTask: %v", err)
		}
		removed := f.revisionOf(t, "worded")
		if err := f.board.RemoveTask("worded"); err != nil {
			t.Fatalf("RemoveTask: %v", err)
		}
		before := f.boardJSON(t)
		tests := []struct {
			name, op, slug, revision string
			fields                   map[string]any
			// fresh is the slug of the entry the answer carries, empty for a null entry.
			fresh string
		}{
			{"Edit", opEdit, "released", older, map[string]any{"title": "Mine"}, "released"},
			{"Release", opRelease, "released", older, nil, "released"},
			{"ReleaseOfARemovedEntry", opRelease, "worded", removed, nil, ""},
			{"CreateOfAnExistingSlug", opCreate, "a-note", "", map[string]any{"title": "Mine"}, "a-note"},
		}
		for _, tt := range tests {
			status, got := f.write(t, tt.op, tt.slug, tt.revision, tt.fields)
			if status != http.StatusConflict || got.OK || got.Error == "" {
				t.Errorf("%s = %d %+v; want 409 with a refusal", tt.name, status, got)
			}
			var want *boardengine.ViewEntry
			if tt.fresh != "" {
				want = f.entry(t, tt.fresh)
			}
			if encoded(t, got.Entry) != encoded(t, want) {
				t.Errorf("%s: entry = %+v; want %+v", tt.name, got.Entry, want)
			}
		}
		if after := f.boardJSON(t); !bytes.Equal(before, after) {
			t.Errorf("board.json changed by a stale write")
		}
	})

	t.Run("LongPoll", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			longPoll time.Duration
			// wake, when set, is run while the request waits.
			wake          func(*Server)
			wantUnchanged bool
		}{
			{"BoundPasses", time.Millisecond, nil, true},
			{"BoardChanges", time.Minute, func(s *Server) { s.BoardChanged("other") }, false},
			{"ServerDrains", time.Minute, (*Server).beginDrain, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newFixture(t, tt.longPoll)
				current := decodeReply(t, f.serve(t, http.MethodGet, routeBoard, "", nil)).Revision
				answered := make(chan *httptest.ResponseRecorder)
				go func() {
					answered <- f.serve(t, http.MethodGet, routeBoard+"?after="+current, "", nil)
				}()
				if tt.wake != nil {
					tt.wake(f.server)
				}
				got := decodeReply(t, <-answered)
				if !got.OK || got.Unchanged != tt.wantUnchanged || (got.Board == nil) != tt.wantUnchanged {
					t.Errorf("GET /api/board?after=current = %+v; want unchanged %v", got, tt.wantUnchanged)
				}
			})
		}
		t.Run("AbsentBoardWaits", func(t *testing.T) {
			t.Parallel()
			server, err := NewServer(ServerConfig{BoardPath: t.TempDir(), Token: testToken, Port: testPort, LongPoll: time.Millisecond, Now: time.Now})
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			got := decodeReply(t, fixture{server: server}.serve(t, http.MethodGet, routeBoard+"?after=", "", nil))
			if !got.Unchanged {
				t.Errorf("GET /api/board?after= on an absent board.json = %+v; want unchanged", got)
			}
		})
	})

	t.Run("FileRouteServesOnlyListedFiles", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Millisecond)
		want, err := os.ReadFile(filepath.Join(f.dir, "design-released.md"))
		if err != nil {
			t.Fatalf("read design file: %v", err)
		}
		tests := []struct {
			name string
			code int
		}{
			{"design-released.md", http.StatusOK},
			{"README.md", http.StatusNotFound},
			{"board.json", http.StatusNotFound},
			{"a/b.md", http.StatusNotFound},
		}
		for _, tt := range tests {
			response := f.serve(t, http.MethodGet, routeFile+tt.name+"?t="+testToken, "", nil)
			if response.Code != tt.code {
				t.Errorf("GET %s%s = %d; want %d", routeFile, tt.name, response.Code, tt.code)
			}
			if tt.code == http.StatusOK && (response.Body.String() != string(want) || response.Header().Get("Content-Type") != "text/plain; charset=utf-8") {
				t.Errorf("GET %s%s = %q as %q; want the design file as plain text", routeFile, tt.name, response.Body, response.Header().Get("Content-Type"))
			}
		}
	})
}
