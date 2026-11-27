// server.go is the board server's HTTP handler: its routes, the checks every request passes and the long poll.

package boardserve

import (
	"bytes"
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

// The paths the server answers; the viewer page spells the same ones.
const (
	routePage  = "/"
	routeBoard = "/api/board"
	routeWrite = "/api/write"
	// routeFile is followed by the file's name.
	routeFile = "/file/"
)

// routeAPI is the prefix of the routes that take the bearer token.
const routeAPI = "/api/"

// maxBodyBytes caps the body of a write.
const maxBodyBytes = 1 << 20

// inlineElement matches an inline script or style element of the page, capturing its name and content.
var inlineElement = regexp.MustCompile(`(?s)<(script|style)>(.*?)</(?:script|style)>`)

// ServerConfig is what a Server is built from.
type ServerConfig struct {
	Board *boardengine.Board
	// BoardPath is the board's data directory, which the file route serves from.
	BoardPath string
	// Token is the secret a request must carry to read or write the board.
	Token string
	// Port is the loopback port the server is bound to.
	Port int
	// LongPoll is how long a long poll waits for a new revision before it answers "unchanged".
	LongPoll time.Duration
	// Now is the clock the last API request is timed by.
	Now func() time.Time
}

// Server is the HTTP handler of one board.
// It is safe for concurrent use.
type Server struct {
	config ServerConfig
	page   []byte
	policy string

	mu       sync.Mutex
	revision string
	// changed is closed and replaced each time the revision is set.
	changed     chan struct{}
	lastRequest time.Time

	draining  chan struct{}
	drainOnce sync.Once
}

// NewServer returns the handler of config's board, at the revision the board holds now.
func NewServer(config ServerConfig) (*Server, error) {
	revision, err := boardengine.BoardRevision(config.BoardPath)
	if err != nil {
		return nil, err
	}
	page := boardengine.ViewerPage()
	return &Server{
		config:   config,
		page:     page,
		policy:   contentSecurityPolicy(page),
		revision: revision,
		changed:  make(chan struct{}),
		draining: make(chan struct{}),
	}, nil
}

// contentSecurityPolicy is the policy of page, admitting each of its inline scripts and styles by hash.
func contentSecurityPolicy(page []byte) string {
	// A browser hashes an element's content after normalizing its line endings.
	normalized := bytes.ReplaceAll(page, []byte("\r\n"), []byte("\n"))
	sources := map[string][]string{}
	for _, match := range inlineElement.FindAllSubmatch(normalized, -1) {
		sum := sha256.Sum256(match[2])
		name := string(match[1])
		sources[name] = append(sources[name], "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return "default-src 'none'; script-src " + strings.Join(sources["script"], " ") +
		"; style-src " + strings.Join(sources["style"], " ") +
		"; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}

// BoardChanged sets the board's current revision and wakes every waiting long poll.
func (s *Server) BoardChanged(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	close(s.changed)
	s.changed = make(chan struct{})
}

// beginDrain answers every waiting long poll "unchanged" at once, and every later one too.
func (s *Server) beginDrain() {
	s.drainOnce.Do(func() { close(s.draining) })
}

// lastAPIRequest returns when the last API request arrived, the zero time when none has.
func (s *Server) lastAPIRequest() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRequest
}

// current returns the revision and the channel closed at its next change.
func (s *Server) current() (string, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision, s.changed
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != s.host() {
		refuse(w, http.StatusMisdirectedRequest, "the request does not name this server's host")
		return
	}

	path := r.URL.Path
	switch {
	case path == routePage:
		if allowMethod(w, r, http.MethodGet) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", s.policy)
			_, _ = w.Write(s.page)
		}
	case strings.HasPrefix(path, routeAPI):
		s.serveAPI(w, r)
	case strings.HasPrefix(path, routeFile):
		if !s.tokenMatches(r.URL.Query().Get("t")) {
			refuse(w, http.StatusUnauthorized, "the request carries no valid token")
			return
		}
		if allowMethod(w, r, http.MethodGet) {
			s.serveFile(w, strings.TrimPrefix(path, routeFile))
		}
	default:
		refuse(w, http.StatusNotFound, "no such page")
	}
}

// host is the one value of a request's Host the server answers.
func (s *Server) host() string {
	return "127.0.0.1:" + strconv.Itoa(s.config.Port)
}

// tokenMatches reports whether token is the server's, comparing in constant time.
func (s *Server) tokenMatches(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	now := s.config.Now()
	s.mu.Lock()
	s.lastRequest = now
	s.mu.Unlock()

	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || !s.tokenMatches(token) {
		refuse(w, http.StatusUnauthorized, "the request carries no valid token")
		return
	}
	switch r.URL.Path {
	case routeBoard:
		if allowMethod(w, r, http.MethodGet) {
			s.serveBoard(w, r)
		}
	case routeWrite:
		if allowMethod(w, r, http.MethodPost) {
			s.serveWrite(w, r)
		}
	default:
		refuse(w, http.StatusNotFound, "no such page")
	}
}

// serveBoard answers the projection, after a wait when the caller already holds the current revision.
func (s *Server) serveBoard(w http.ResponseWriter, r *http.Request) {
	after, waits := r.URL.Query().Get("after"), r.URL.Query().Has("after")
	bound := time.NewTimer(s.config.LongPoll)
	defer bound.Stop()
	for {
		// The revision is read before the view, so a change between the two is answered by the next poll.
		revision, changed := s.current()
		if !waits || after != revision {
			view, err := s.config.Board.View()
			if err != nil {
				refuse(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			reply(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "board": view})
			return
		}
		select {
		case <-changed:
			continue
		case <-bound.C:
		case <-r.Context().Done():
		case <-s.draining:
		}
		reply(w, http.StatusOK, map[string]any{"ok": true, "unchanged": true, "revision": revision})
		return
	}
}

// serveFile serves name from the board directory when the current projection names it as a design or decision file.
func (s *Server) serveFile(w http.ResponseWriter, name string) {
	view, err := s.config.Board.View()
	if err != nil {
		refuse(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	listed := name != "" &&
		(slices.ContainsFunc(view.Entries, func(entry boardengine.ViewEntry) bool {
			return entry.DesignFile == name || entry.DecisionFile == name
		}) || slices.ContainsFunc(view.Projects, func(project boardengine.ViewProject) bool {
			return project.DesignFile == name
		}))
	if !listed {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	data, err := os.ReadFile(filepath.Join(s.config.BoardPath, name))
	if err != nil {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// serveWrite checks a write's transport, then applies it and answers the entry it left.
func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		refuse(w, http.StatusUnsupportedMediaType, "a write is application/json")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.host() {
		refuse(w, http.StatusForbidden, "the write comes from another origin")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		refuse(w, http.StatusRequestEntityTooLarge, "the write is too large")
		return
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, "read the write: "+err.Error())
		return
	}
	request, err := decodeWriteRequest(body)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}

	entry, err := s.applyWrite(request)
	var stale *boardengine.StaleEntryError
	switch {
	case errors.As(err, &stale):
		reply(w, http.StatusConflict, map[string]any{"ok": false, "error": stale.Error(), "entry": stale.Fresh})
	case err != nil:
		refuse(w, http.StatusUnprocessableEntity, err.Error())
	default:
		reply(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
	}
}

// allowMethod reports whether r uses method, and refuses it otherwise.
func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	refuse(w, http.StatusMethodNotAllowed, "the route takes "+method)
	return false
}

// refuse answers status with message as the refusal.
func refuse(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]any{"ok": false, "error": message})
}

// reply answers status with body as JSON.
func reply(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A failed encode means the client is gone, which leaves nobody to tell.
	_ = json.NewEncoder(w).Encode(body)
}
