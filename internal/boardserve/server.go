// server.go is the board server's HTTP handler: the request checks, the page, the long-polled board and the file route.
// The six page writes live in write.go.

package boardserve

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The server's routes, which the static page spells too.
const (
	routePage  = "/"
	routeBoard = "/api/board"
	routeWrite = "/api/write"
	routeFile  = "/file/"
)

// maxBodyBytes caps a write's body.
const maxBodyBytes = 1 << 20

// fileTokenParameter is the query parameter that carries the token on the file route, where a header cannot be sent.
const fileTokenParameter = "t"

// loopbackHost is the address the server binds and the only Host it answers.
const loopbackHost = "127.0.0.1"

// ServerConfig holds what a Server is built from.
type ServerConfig struct {
	// Board is the board every read and write goes through.
	Board *boardengine.Board
	// BoardPath is the board directory, which the file route reads from and the revision is taken over.
	BoardPath string
	// Token is the bearer token every /api request and the file route need.
	Token string
	// Port is the bound loopback port, which the Host and Origin checks name.
	Port int
	// LongPollBound is how long a long poll waits for a change before it answers "unchanged".
	LongPollBound time.Duration
	// Now is the clock the server stamps API activity with; nil is the wall clock.
	Now func() time.Time
}

// Server serves the board over HTTP and is its own http.Handler.
// It is safe for concurrent use.
type Server struct {
	cfg ServerConfig
	// csp is the page's Content-Security-Policy, computed once from the embedded page.
	csp string

	mu       sync.Mutex
	revision string
	// changed is closed and replaced whenever the revision changes or the server starts draining.
	changed  chan struct{}
	draining bool
	lastAPI  time.Time
}

// NewServer returns a Server over cfg.
// Its current revision is the board.json hash at the call; BoardChanged moves it on.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	// An unreadable board.json reads as no revision, and the next BoardChanged sets one.
	revision, _ := boardengine.BoardRevision(cfg.BoardPath)
	return &Server{
		cfg:      cfg,
		csp:      contentSecurityPolicy(boardengine.ViewerPage()),
		revision: revision,
		changed:  make(chan struct{}),
		lastAPI:  cfg.Now(),
	}
}

// BoardChanged records revision as the board's current one and wakes every waiting long poll.
func (s *Server) BoardChanged(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	s.wakeWaiters()
}

// beginDrain wakes every waiting long poll with "unchanged" and makes later long polls answer at once.
func (s *Server) beginDrain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draining = true
	s.wakeWaiters()
}

// wakeWaiters releases every goroutine waiting on the changed channel; the caller holds s.mu.
func (s *Server) wakeWaiters() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// lastAPIRequest returns when the server last received an /api request that carried its token, or when it was built.
func (s *Server) lastAPIRequest() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAPI
}

// stampAPIRequest records now as the time of the last authorized /api request, which the idle exit measures from.
func (s *Server) stampAPIRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAPI = s.cfg.Now()
}

// ServeHTTP checks the request, then routes it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")

	if r.Host != s.host() {
		refuse(w, http.StatusMisdirectedRequest, "the request names another host than the server's own")
		return
	}
	path := r.URL.Path
	switch {
	case path == routePage:
		s.servePage(w, r)
	case path == routeBoard:
		if s.bearerAuthorized(r) {
			s.stampAPIRequest()
			s.serveBoard(w, r)
		} else {
			refuse(w, http.StatusUnauthorized, "the request carries no valid token")
		}
	case path == routeWrite:
		if s.bearerAuthorized(r) {
			s.stampAPIRequest()
			s.serveWrite(w, r)
		} else {
			refuse(w, http.StatusUnauthorized, "the request carries no valid token")
		}
	case strings.HasPrefix(path, routeFile):
		if s.tokenEqual(r.URL.Query().Get(fileTokenParameter)) {
			s.serveFile(w, r, strings.TrimPrefix(path, routeFile))
		} else {
			refuse(w, http.StatusUnauthorized, "the request carries no valid token")
		}
	default:
		refuse(w, http.StatusNotFound, "no such route")
	}
}

// host is the address:port value the server answers in a request's Host field.
func (s *Server) host() string {
	return fmt.Sprintf("%s:%d", loopbackHost, s.cfg.Port)
}

// origin is the Origin header value of the server's own page.
func (s *Server) origin() string {
	return "http://" + s.host()
}

// tokenEqual reports whether presented is the server's token, compared in constant time.
func (s *Server) tokenEqual(presented string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.cfg.Token)) == 1
}

// bearerAuthorized reports whether r carries the server's token as a bearer credential.
func (s *Server) bearerAuthorized(r *http.Request) bool {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return found && s.tokenEqual(token)
}

// writeJSON answers status with payload encoded as JSON.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A failed write means the client went away, which the server cannot act on.
	_ = json.NewEncoder(w).Encode(payload)
}

// refuse answers status with the JSON refusal {"ok":false,"error":message}.
func refuse(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message})
}

// servePage answers GET / with the embedded viewer page under its Content-Security-Policy.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		refuse(w, http.StatusMethodNotAllowed, "the page takes GET")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", s.csp)
	_, _ = w.Write(boardengine.ViewerPage())
}

// serveBoard answers GET /api/board with the view projection, or with "unchanged" after a long poll that saw no change.
func (s *Server) serveBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		refuse(w, http.StatusMethodNotAllowed, "the board takes GET")
		return
	}
	query := r.URL.Query()
	revision, unchanged := s.waitForChange(r, query.Get("after"), query.Has("after"))
	if unchanged {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unchanged": true, "revision": revision})
		return
	}
	view, err := s.cfg.Board.View()
	if err != nil {
		refuse(w, http.StatusInternalServerError, fmt.Sprintf("read the board: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "board": view})
}

// waitForChange returns the revision to answer with and whether the poll saw no change.
// A request with no after key, or one whose after is not the current revision, answers at once.
// An after that is the current revision waits for a change, the bound, the end of the request or the drain.
// A board with no board.json has the empty revision.
func (s *Server) waitForChange(r *http.Request, after string, hasAfter bool) (revision string, unchanged bool) {
	timer := time.NewTimer(s.cfg.LongPollBound)
	defer timer.Stop()
	for {
		s.mu.Lock()
		revision, draining, changed := s.revision, s.draining, s.changed
		s.mu.Unlock()
		if !hasAfter || after != revision {
			return revision, false
		}
		if draining {
			return revision, true
		}
		select {
		case <-changed:
		case <-timer.C:
			return revision, true
		case <-r.Context().Done():
			return revision, true
		}
	}
}

// serveFile answers GET /file/<name> with a design file or decision record the current projection names.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		refuse(w, http.StatusMethodNotAllowed, "the file route takes GET")
		return
	}
	view, err := s.cfg.Board.View()
	if err != nil {
		refuse(w, http.StatusInternalServerError, fmt.Sprintf("read the board: %v", err))
		return
	}
	if name == "" || !namesFile(view, name) {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	content, err := os.ReadFile(filepath.Join(s.cfg.BoardPath, name))
	if err != nil {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(content)
}

// namesFile reports whether name is the design file of a project or entry of view, or the decision file of an entry.
func namesFile(view boardengine.BoardView, name string) bool {
	for _, project := range view.Projects {
		if project.DesignFile == name {
			return true
		}
	}
	for _, entry := range view.Entries {
		if entry.DesignFile == name || entry.DecisionFile == name {
			return true
		}
	}
	return false
}

// serveWrite answers POST /api/write: it checks the transport, decodes the request and applies it.
func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		refuse(w, http.StatusMethodNotAllowed, "the write route takes POST")
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		refuse(w, http.StatusUnsupportedMediaType, "a write carries Content-Type: application/json")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.origin() {
		refuse(w, http.StatusForbidden, "the write comes from another origin than the server's own")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var request writeRequest
	if err == nil {
		err = decodeWriteRequest(body, &request)
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			refuse(w, http.StatusRequestEntityTooLarge, "the write body is over the size cap")
			return
		}
		refuse(w, http.StatusBadRequest, fmt.Sprintf("the write body is not a valid write request: %v", err))
		return
	}

	entry, err := s.applyWrite(request)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
		return
	}
	if fresh, stale := staleEntry(err); stale {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error(), "entry": fresh})
		return
	}
	refuse(w, http.StatusUnprocessableEntity, err.Error())
}

// inlineElementPattern matches one script or style element of the page, capturing its tag, attributes and content.
var inlineElementPattern = regexp.MustCompile(`(?s)<(script|style)([^>]*)>(.*?)</(?:script|style)>`)

// contentSecurityPolicy returns the policy the page is served under.
// It admits only the sha256 of each inline script and style element's content, and never unsafe-inline,
// so the page's board-data.js script tag, which loads a file, stays blocked over http.
func contentSecurityPolicy(page []byte) string {
	hashes := map[string][]string{}
	for _, match := range inlineElementPattern.FindAllSubmatch(page, -1) {
		tag, attributes, content := string(match[1]), string(match[2]), match[3]
		if strings.Contains(attributes, "src=") {
			continue
		}
		sum := sha256.Sum256(content)
		hashes[tag] = append(hashes[tag], "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	source := func(tag string) string {
		if len(hashes[tag]) == 0 {
			return "'none'"
		}
		return strings.Join(hashes[tag], " ")
	}
	return "default-src 'none'; script-src " + source("script") + "; style-src " + source("style") +
		"; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}
