// server.go is the board server's HTTP handler: the request checks, the routes and the long poll.

package boardserve

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The server's routes, which the viewer page spells too.
const (
	routePage  = "/"
	routeBoard = "/api/board"
	routeWrite = "/api/write"
	routeFile  = "/file/"
)

// apiPrefix opens the path of every route that needs the bearer token.
const apiPrefix = "/api/"

// loopbackHost is the address every URL of the server names.
const loopbackHost = "127.0.0.1"

// maxBodyBytes caps a write request's body.
const maxBodyBytes = 1 << 20

// ServerConfig is what a Server is built from.
type ServerConfig struct {
	Board *boardengine.Board
	// BoardPath is the board directory the file route and the board revision read.
	BoardPath string
	// Token is the secret every /api and file request carries.
	Token string
	// Port is the loopback port the server is reached on.
	Port int
	// PollBound is how long a long poll waits for a change before it answers "unchanged".
	PollBound time.Duration
	// Now is the clock that times /api requests.
	Now func() time.Time
}

// Server serves one board over HTTP.
// It is safe for concurrent use.
type Server struct {
	config ServerConfig
	host   string
	csp    string

	mu       sync.Mutex
	revision string
	// changed is closed and replaced at every BoardChanged.
	changed chan struct{}
	// drained is closed once the server stops holding long polls.
	drained     chan struct{}
	lastRequest time.Time
}

// NewServer returns a server over config's board, at the board's current revision.
// Its idle time counts from now.
func NewServer(config ServerConfig) (*Server, error) {
	revision, err := boardengine.BoardRevision(config.BoardPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		config:      config,
		host:        loopbackHost + ":" + strconv.Itoa(config.Port),
		csp:         contentSecurityPolicy(boardengine.ViewerPage()),
		revision:    revision,
		changed:     make(chan struct{}),
		drained:     make(chan struct{}),
		lastRequest: config.Now(),
	}, nil
}

// BoardChanged records revision as the board's and wakes every waiting long poll.
func (s *Server) BoardChanged(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	close(s.changed)
	s.changed = make(chan struct{})
}

// beginDrain answers every waiting long poll "unchanged", and every later one at once.
func (s *Server) beginDrain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.drained:
	default:
		close(s.drained)
	}
}

// lastAPIRequest is when the latest /api request arrived, or when the server was built if none has.
func (s *Server) lastAPIRequest() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRequest
}

// currentRevision returns the recorded board revision and the channel that closes when it is recorded again.
func (s *Server) currentRevision() (string, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision, s.changed
}

// ServeHTTP implements http.Handler: it checks the request and answers its route.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	path := r.URL.Path
	api := strings.HasPrefix(path, apiPrefix)
	if api {
		s.mu.Lock()
		s.lastRequest = s.config.Now()
		s.mu.Unlock()
	}

	if r.Host != s.host {
		refuse(w, http.StatusMisdirectedRequest, "the request names another host")
		return
	}
	if api {
		token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !bearer || !s.tokenMatches(token) {
			refuse(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
	}

	switch {
	case path == routePage && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", s.csp)
		_, _ = w.Write(boardengine.ViewerPage())
	case path == routeBoard && r.Method == http.MethodGet:
		s.serveBoard(w, r)
	case path == routeWrite && r.Method == http.MethodPost:
		s.serveWrite(w, r)
	case strings.HasPrefix(path, routeFile) && r.Method == http.MethodGet:
		s.serveFile(w, r)
	default:
		refuse(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) tokenMatches(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

// serveBoard answers the view projection, first waiting out an `after` that names the current revision.
func (s *Server) serveBoard(w http.ResponseWriter, r *http.Request) {
	// A board with no entry store has the empty revision, so the parameter's presence asks for the wait, not its value.
	if query := r.URL.Query(); query.Has("after") && !s.awaitChange(w, r, query.Get("after")) {
		return
	}
	// The revision is read before the view, so a change in between is asked for again rather than missed.
	revision, _ := s.currentRevision()
	view, err := s.config.Board.View()
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "board": view})
}

// awaitChange waits until the board's revision differs from after, and reports whether it does.
// When it reports false the request is answered "unchanged", or has ended.
func (s *Server) awaitChange(w http.ResponseWriter, r *http.Request, after string) bool {
	bound := time.NewTimer(s.config.PollBound)
	defer bound.Stop()
	for {
		revision, changed := s.currentRevision()
		if revision != after {
			return true
		}
		select {
		case <-changed:
			continue
		case <-r.Context().Done():
			return false
		case <-s.drained:
		case <-bound.C:
		}
		answer(w, http.StatusOK, map[string]any{"ok": true, "unchanged": true, "revision": revision})
		return false
	}
}

// serveFile serves a file of the board directory that the current view lists as a design or decision file.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	if !s.tokenMatches(r.URL.Query().Get("t")) {
		refuse(w, http.StatusUnauthorized, "missing or wrong token")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, routeFile)
	view, err := s.config.Board.View()
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if name == "" || strings.ContainsAny(name, `/\`) || !listsFile(view, name) {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	data, err := os.ReadFile(filepath.Join(s.config.BoardPath, name))
	if err != nil {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// listsFile reports whether view names name as a design or decision file.
func listsFile(view boardengine.BoardView, name string) bool {
	return slices.ContainsFunc(view.Projects, func(p boardengine.ViewProject) bool { return p.DesignFile == name }) ||
		slices.ContainsFunc(view.Entries, func(e boardengine.ViewEntry) bool { return e.DesignFile == name || e.DecisionFile == name })
}

// serveWrite checks a write request's headers and body, and runs it.
func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		refuse(w, http.StatusUnsupportedMediaType, "a write needs Content-Type application/json")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.host {
		refuse(w, http.StatusForbidden, "the write comes from another origin")
		return
	}
	// The body is read whole before it is decoded, so one over the cap is refused whatever it opens with.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		refuse(w, http.StatusRequestEntityTooLarge, "the body is too large")
		return
	}
	var request writeRequest
	if err == nil {
		request, err = decodeWriteRequest(body)
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, "the body is not a write request: "+err.Error())
		return
	}

	entry, err := s.applyWrite(request)
	var stale *boardengine.StaleEntryError
	switch {
	case errors.As(err, &stale):
		answer(w, http.StatusConflict, map[string]any{"ok": false, "error": stale.Error(), "entry": stale.Fresh})
	case err != nil:
		refuse(w, http.StatusUnprocessableEntity, err.Error())
	default:
		answer(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
	}
}

// inlineElement matches an inline style or script element, which has no attribute, and captures its content.
var inlineElement = regexp.MustCompile(`(?s)<(script|style)>(.*?)</(?:script|style)>`)

// contentSecurityPolicy is the policy page is served under.
// It lets page run its own inline script and style, by their hashes, and connect to its own origin only.
func contentSecurityPolicy(page []byte) string {
	hashes := map[string]string{}
	for _, element := range inlineElement.FindAllSubmatch(page, -1) {
		sum := sha256.Sum256(element[2])
		hashes[string(element[1])] += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return "default-src 'none'; script-src" + hashes["script"] + "; style-src" + hashes["style"] +
		"; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}

// refuse answers a request that changed nothing with message.
func refuse(w http.ResponseWriter, status int, message string) {
	answer(w, status, map[string]any{"ok": false, "error": message})
}

func answer(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A failed write means the client is gone, and there is no one left to tell.
	_ = json.NewEncoder(w).Encode(body)
}
