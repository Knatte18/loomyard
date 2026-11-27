// server_test.go drives the board server's handler in process over a temp board.

package boardserve

import (
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
	testToken = "secret"
	testPort  = 4321
	testHost  = "127.0.0.1:4321"
)

// fixture is one seeded board with a server over it.
type fixture struct {
	t      *testing.T
	dir    string
	board  *boardengine.Board
	server *Server
}

// newFixture seeds a board with a released, a draft, a running and a hand-set task, and a note.
func newFixture(t *testing.T, pollBound time.Duration) fixture {
	t.Helper()
	dir := t.TempDir()
	board := boardengine.New(boardengine.Config{
		Path:         dir,
		Readme:       "README.md",
		DesignPrefix: "design-",
		Types:        []boardengine.Label{{Name: "bug"}},
		SkipGit:      true,
		Clock:        func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) },
	})
	for _, slug := range []string{"open", "drafted", "running", "waiting"} {
		fields := map[string]any{"slug": slug, "title": slug, "kind": boardengine.KindTask, "labels": []string{"bug"}}
		if slug == "open" {
			fields["body"] = "The design."
		}
		if _, err := board.UpsertTask(fields); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}
	if _, err := board.UpsertTask(map[string]any{"slug": "idea", "title": "idea", "kind": boardengine.KindNote, "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed idea: %v", err)
	}
	running, waiting := boardengine.RunStatus("running", "Webster"), "waiting"
	for slug, status := range map[string]*string{"open": nil, "running": &running, "waiting": &waiting} {
		if err := board.SetStatus(slug, status); err != nil {
			t.Fatalf("seed status of %s: %v", slug, err)
		}
	}
	server, err := NewServer(ServerConfig{Board: board, BoardPath: dir, Token: testToken, Port: testPort, PollBound: pollBound, Now: time.Now})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return fixture{t: t, dir: dir, board: board, server: server}
}

// request builds a request that passes every check.
func request(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = testHost
	r.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

type reply struct {
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error"`
	Revision  string                 `json:"revision"`
	Unchanged bool                   `json:"unchanged"`
	Entry     *boardengine.ViewEntry `json:"entry"`
	Board     boardengine.BoardView  `json:"board"`
}

func (f fixture) serve(r *http.Request) *httptest.ResponseRecorder {
	f.t.Helper()
	recorder := httptest.NewRecorder()
	f.server.ServeHTTP(recorder, r)
	header := recorder.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Access-Control-Allow-Origin") != "" {
		f.t.Errorf("%s %s headers = %v", r.Method, r.URL, header)
	}
	return recorder
}

func (f fixture) reply(r *http.Request) (int, reply) {
	f.t.Helper()
	recorder := f.serve(r)
	var got reply
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		f.t.Fatalf("decode reply %q: %v", recorder.Body, err)
	}
	return recorder.Code, got
}

func (f fixture) write(write writeRequest) (int, reply) {
	f.t.Helper()
	body, err := json.Marshal(write)
	if err != nil {
		f.t.Fatalf("encode write: %v", err)
	}
	return f.reply(request(http.MethodPost, routeWrite, string(body)))
}

func (f fixture) entry(slug string) boardengine.ViewEntry {
	f.t.Helper()
	_, entry, err := f.server.viewEntry(slug)
	if err != nil || entry == nil {
		f.t.Fatalf("view entry %s = %v, %v", slug, entry, err)
	}
	return *entry
}

func (f fixture) store() string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "board.json"))
	if err != nil {
		f.t.Fatalf("read board.json: %v", err)
	}
	return string(data)
}

func TestServer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TransportRefusalsChangeNothing", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		create := `{"op":"create","slug":"new","fields":{"kind":"task","title":"New","labels":["bug"]}}`
		post := func(change func(r *http.Request)) *http.Request {
			r := request(http.MethodPost, routeWrite, create)
			change(r)
			return r
		}
		get := func(target, authorization string) *http.Request {
			r := request(http.MethodGet, target, "")
			r.Header.Set("Authorization", authorization)
			return r
		}
		tests := []struct {
			name    string
			request *http.Request
			want    int
		}{
			{"WrongHost", post(func(r *http.Request) { r.Host = "localhost:4321" }), http.StatusMisdirectedRequest},
			{"NoToken", get(routeBoard, ""), http.StatusUnauthorized},
			{"WrongToken", get(routeBoard, "Bearer wrong"), http.StatusUnauthorized},
			{"FileWithoutToken", get(routeFile+"design-open.md", ""), http.StatusUnauthorized},
			{"FileWithWrongToken", get(routeFile+"design-open.md?t=wrong", ""), http.StatusUnauthorized},
			{"NotJSON", post(func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }), http.StatusUnsupportedMediaType},
			{"ForeignOrigin", post(func(r *http.Request) { r.Header.Set("Origin", "http://example.com") }), http.StatusForbidden},
			{"BodyAboveCap", request(http.MethodPost, routeWrite, create+strings.Repeat(" ", maxBodyBytes)), http.StatusRequestEntityTooLarge},
			{"UnknownKey", request(http.MethodPost, routeWrite, strings.Replace(create, "{", `{"extra":1,`, 1)), http.StatusBadRequest},
		}
		before := f.store()
		for _, tt := range tests {
			if code, got := f.reply(tt.request); code != tt.want || got.OK {
				t.Errorf("%s = %d %+v; want %d", tt.name, code, got, tt.want)
			}
		}
		if after := f.store(); after != before {
			t.Errorf("a refused request changed board.json")
		}
		// A browser puts the server's own origin on every write the page sends.
		own := post(func(r *http.Request) { r.Header.Set("Origin", "http://"+testHost) })
		if code, got := f.reply(own); code != http.StatusOK || !got.OK {
			t.Errorf("OwnOrigin = %d %q; want 200", code, got.Error)
		}
	})

	t.Run("PageCarriesAHashOnlyPolicyAndTheRoutes", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		r := request(http.MethodGet, routePage, "")
		r.Header.Del("Authorization")
		recorder := f.serve(r)
		page := string(boardengine.ViewerPage())
		if recorder.Code != http.StatusOK || recorder.Body.String() != page {
			t.Errorf("GET / = %d; want the viewer page", recorder.Code)
		}
		policy := recorder.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "'unsafe-inline'") {
			t.Errorf("policy = %q; want no unsafe-inline", policy)
		}
		for _, element := range []string{"script", "style"} {
			_, rest, _ := strings.Cut(page, "<"+element+">")
			content, _, _ := strings.Cut(rest, "</"+element+">")
			sum := sha256.Sum256([]byte(content))
			want := element + "-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"
			if !strings.Contains(policy, want) {
				t.Errorf("policy = %q; want %q", policy, want)
			}
		}
		for _, route := range []string{routeBoard, routeWrite, routeFile} {
			if !strings.Contains(page, `"`+route+`"`) {
				t.Errorf("the page does not hold the route %q", route)
			}
		}
	})

	t.Run("BoardAnswersTheView", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		code, got := f.reply(request(http.MethodGet, routeBoard, ""))
		want, err := f.board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		want.Rendered, got.Board.Rendered = "", ""
		gotJSON, _ := json.Marshal(got.Board)
		wantJSON, _ := json.Marshal(want)
		if code != http.StatusOK || string(gotJSON) != string(wantJSON) {
			t.Errorf("GET %s = %d %s; want %s", routeBoard, code, gotJSON, wantJSON)
		}
	})

	t.Run("EachOpWritesWhatItsVerbWrites", func(t *testing.T) {
		t.Parallel()
		draft := boardengine.StatusDraft
		upsert := func(fields map[string]any) func(b *boardengine.Board) error {
			return func(b *boardengine.Board) error { _, err := b.UpsertTaskReporting(fields); return err }
		}
		setStatus := func(slug string, status *string) func(b *boardengine.Board) error {
			return func(b *boardengine.Board) error { _, err := b.SetStatusReleasingProject(slug, status); return err }
		}
		tests := []struct {
			op, slug string
			fields   map[string]any
			verb     func(b *boardengine.Board) error
		}{
			{opCreate, "new", map[string]any{"kind": "task", "title": "New", "labels": []string{"bug"}, "depends_on": []string{"open"}},
				upsert(map[string]any{"slug": "new", "kind": "task", "title": "New", "labels": []string{"bug"}, "depends_on": []string{"open"}})},
			{opEdit, "open", map[string]any{"title": "Edited", "brief": "Brief."},
				upsert(map[string]any{"slug": "open", "title": "Edited", "brief": "Brief."})},
			{opRelease, "drafted", nil, setStatus("drafted", nil)},
			{opPromote, "idea", nil, func(b *boardengine.Board) error { _, err := b.PromoteTo("idea", "", nil); return err }},
			{opDraft, "open", nil, setStatus("open", &draft)},
			{opUndraft, "drafted", nil, setStatus("drafted", nil)},
		}
		for _, tt := range tests {
			t.Run(tt.op, func(t *testing.T) {
				t.Parallel()
				f, twin := newFixture(t, time.Minute), newFixture(t, time.Minute)
				revision := ""
				if tt.op != opCreate {
					revision = f.entry(tt.slug).Revision
				}
				code, got := f.write(writeRequest{Op: tt.op, Slug: tt.slug, Revision: revision, Fields: tt.fields})
				if err := tt.verb(twin.board); err != nil {
					t.Fatalf("verb: %v", err)
				}
				if code != http.StatusOK || got.Entry == nil || got.Entry.Revision != f.entry(tt.slug).Revision {
					t.Errorf("%s = %d %+v; want the fresh entry", tt.op, code, got)
				}
				if f.store() != twin.store() {
					t.Errorf("%s left board.json\n%s\nwant\n%s", tt.op, f.store(), twin.store())
				}
				if tt.op != opEdit {
					return
				}
				next := writeRequest{Op: opEdit, Slug: tt.slug, Revision: got.Entry.Revision, Fields: map[string]any{"title": "Again"}}
				if code, got := f.write(next); code != http.StatusOK {
					t.Errorf("an edit with the answered revision = %d %q", code, got.Error)
				}
			})
		}
	})

	t.Run("BoardRefusalsChangeNothing", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		_, lockErr := newFixture(t, time.Minute).board.UpsertTaskReporting(map[string]any{"slug": "running", "title": "Other"})
		if lockErr == nil {
			t.Fatalf("the board accepted an edit of a running task")
		}
		tests := []struct {
			name, op, slug string
			fields         map[string]any
			noRevision     bool
			want           string
		}{
			{"DraftOfAStatus", opDraft, "waiting", nil, false, "draft takes a task with no status"},
			{"UndraftOfNoStatus", opUndraft, "open", nil, false, "is released already"},
			{"UndraftOfARun", opUndraft, "running", nil, false, "clear it with lym board set-status"},
			{"UndraftOfAnotherStatus", opUndraft, "waiting", nil, false, "release it with the page's release"},
			{"DraftOfANote", opDraft, "idea", nil, false, "is a note"},
			{"UndraftOfANote", opUndraft, "idea", nil, false, "is a note"},
			{"ReleaseOfARun", opRelease, "running", nil, false, "release it with lym board set-status"},
			{"ReleaseOfANote", opRelease, "idea", nil, false, "is a note"},
			{"PromoteOfATask", opPromote, "open", nil, false, "is a task already"},
			{"EditWithoutRevision", opEdit, "absent", map[string]any{"title": "X"}, true, "carries no revision"},
			{"UnknownOp", "remove", "open", nil, false, "unknown op"},
			{"UnknownField", opEdit, "open", map[string]any{"status": "done"}, false, `not "status"`},
			{"CreateOfAProject", opCreate, "fresh", map[string]any{"kind": "project"}, true, "creates a task or a note"},
			{"EditOfARunningTask", opEdit, "running", map[string]any{"title": "Other"}, false, lockErr.Error()},
		}
		before := f.store()
		for _, tt := range tests {
			write := writeRequest{Op: tt.op, Slug: tt.slug, Fields: tt.fields}
			if !tt.noRevision {
				write.Revision = f.entry(tt.slug).Revision
			}
			if code, got := f.write(write); code != http.StatusUnprocessableEntity || !strings.Contains(got.Error, tt.want) {
				t.Errorf("%s = %d %q; want a refusal holding %q", tt.name, code, got.Error, tt.want)
			}
		}
		if after := f.store(); after != before {
			t.Errorf("a refused write changed board.json")
		}
	})

	t.Run("StaleWritesAnswerAConflict", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		old := f.entry("open").Revision
		if _, err := f.board.UpsertTask(map[string]any{"slug": "open", "title": "Moved on"}); err != nil {
			t.Fatalf("change: %v", err)
		}
		current := f.entry("open").Revision
		oldNote := f.entry("idea").Revision
		if _, err := f.board.UpsertTask(map[string]any{"slug": "idea", "title": "Moved on"}); err != nil {
			t.Fatalf("change the note: %v", err)
		}
		currentNote := f.entry("idea").Revision
		before := f.store()
		tests := []struct {
			name      string
			write     writeRequest
			wantFresh string
		}{
			{"CreateOfAnExistingSlug", writeRequest{Op: opCreate, Slug: "open", Fields: map[string]any{"kind": "task"}}, current},
			{"Edit", writeRequest{Op: opEdit, Slug: "open", Revision: old, Fields: map[string]any{"title": "X"}}, current},
			{"Release", writeRequest{Op: opRelease, Slug: "open", Revision: old}, current},
			{"Promote", writeRequest{Op: opPromote, Slug: "idea", Revision: oldNote}, currentNote},
			{"ReleaseOfARemovedEntry", writeRequest{Op: opRelease, Slug: "absent", Revision: old}, ""},
		}
		for _, tt := range tests {
			code, got := f.write(tt.write)
			fresh := ""
			if got.Entry != nil {
				fresh = got.Entry.Revision
			}
			if code != http.StatusConflict || fresh != tt.wantFresh {
				t.Errorf("%s = %d with entry revision %q; want 409 with %q", tt.name, code, fresh, tt.wantFresh)
			}
		}
		if after := f.store(); after != before {
			t.Errorf("a stale write changed board.json")
		}
	})

	t.Run("LongPoll", func(t *testing.T) {
		t.Parallel()
		poll := func(f fixture, revision string) (int, reply) {
			return f.reply(request(http.MethodGet, routeBoard+"?after="+revision, ""))
		}
		current := func(f fixture) string {
			_, got := f.reply(request(http.MethodGet, routeBoard, ""))
			return got.Revision
		}

		short := newFixture(t, time.Millisecond)
		if _, got := poll(short, current(short)); !got.Unchanged {
			t.Errorf("a poll past its bound = %+v; want unchanged", got)
		}

		changed := newFixture(t, time.Minute)
		revision := current(changed)
		go changed.server.BoardChanged("next")
		if _, got := poll(changed, revision); got.Unchanged || got.Revision != "next" || len(got.Board.Entries) == 0 {
			t.Errorf("a poll across a change = %+v; want the new view", got)
		}

		// A board with no entry store has the empty revision, which an empty after names.
		bare, err := NewServer(ServerConfig{Board: short.board, BoardPath: t.TempDir(), Token: testToken, Port: testPort, PollBound: time.Millisecond, Now: time.Now})
		if err != nil {
			t.Fatalf("NewServer over no store: %v", err)
		}
		if _, got := poll(fixture{t: t, server: bare}, ""); !got.Unchanged {
			t.Errorf("a poll of a board with no store = %+v; want unchanged", got)
		}

		draining := newFixture(t, time.Minute)
		revision = current(draining)
		go draining.server.beginDrain()
		if _, got := poll(draining, revision); !got.Unchanged {
			t.Errorf("a poll across a drain = %+v; want unchanged", got)
		}
	})

	t.Run("FileRouteServesOnlyListedFiles", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, time.Minute)
		get := func(name string) *httptest.ResponseRecorder {
			return f.serve(request(http.MethodGet, routeFile+name+"?t="+testToken, ""))
		}
		if got := get("design-open.md"); got.Code != http.StatusOK || !strings.HasSuffix(got.Body.String(), "The design.") || got.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("a listed file = %d %q", got.Code, got.Body)
		}
		for _, name := range []string{"README.md", "a/b.md"} {
			if got := get(name); got.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d; want 404", name, got.Code)
			}
		}

		if err := f.board.StoreDecisionRecord("open", []byte("The record.")); err != nil {
			t.Fatalf("store the record: %v", err)
		}
		project := map[string]any{"slug": "proj", "title": "Proj", "kind": boardengine.KindProject, "labels": []string{"bug"}, "body": "Project design."}
		if _, err := f.board.UpsertTask(project); err != nil {
			t.Fatalf("seed the project: %v", err)
		}
		decision := boardengine.DecisionFileName("open")
		if named := f.entry("open").DecisionFile; named != decision {
			t.Errorf("decision_file = %q; want %q", named, decision)
		}
		for name, want := range map[string]string{decision: "The record.", "design-proj.md": "Project design."} {
			if got := get(name); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), want) {
				t.Errorf("GET %s = %d %q; want 200 holding %q", name, got.Code, got.Body, want)
			}
		}
	})
}
