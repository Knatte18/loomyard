// server.go holds the HTTP handler: the request checks, the read routes, the long poll and the page's security policy.

package boardserve

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// The routes, which the viewer page spells too.
const (
	routePage  = "/"
	routeBoard = "/api/board"
	routeWrite = "/api/write"
	routeFile  = "/file/"
	// routeAPI is the prefix of the routes that take the token as a bearer credential.
	routeAPI = "/api/"
)

// maxBodyBytes caps the body of a write request.
const maxBodyBytes = 1 << 20

// loopbackHost is the one address the server is reached at.
const loopbackHost = "127.0.0.1"

// ServerConfig is what a Server serves and how.
type ServerConfig struct {
	Board *boardengine.Board
	// BoardPath is the board directory, which the file route reads from.
	BoardPath string
	// Token is the secret an API or file request must carry.
	Token string
	// Port is the bound port, which a request's Host and Origin must name.
	Port int
	// LongPoll bounds how long a board request waits for a change.
	LongPoll time.Duration
	// Now is the clock API requests are timed by.
	Now func() time.Time
}

// Server is the board's HTTP handler, safe for concurrent use.
type Server struct {
	config ServerConfig
	policy string
	drain  chan struct{}
	// draining closes drain once.
	draining sync.Once

	mu       sync.Mutex
	revision string
	// changed is closed and replaced each time the revision is set.
	changed chan struct{}
	// apiRequestAt is the time of the latest API request, or of the server's making before the first.
	apiRequestAt time.Time
}

// NewServer returns a handler over config's board, at the board's current revision.
// It returns an error when the revision cannot be read.
func NewServer(config ServerConfig) (*Server, error) {
	revision, err := boardengine.BoardRevision(config.BoardPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		config:       config,
		policy:       contentSecurityPolicy(boardengine.ViewerPage()),
		drain:        make(chan struct{}),
		revision:     revision,
		changed:      make(chan struct{}),
		apiRequestAt: config.Now(),
	}, nil
}

// BoardChanged records the board's revision and wakes every waiting board request.
func (s *Server) BoardChanged(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	close(s.changed)
	s.changed = make(chan struct{})
}

// beginDrain answers every waiting board request "unchanged" at once, and every later one without waiting.
func (s *Server) beginDrain() {
	s.draining.Do(func() { close(s.drain) })
}

// lastAPIRequest is the time of the latest API request, or of the server's making before the first.
func (s *Server) lastAPIRequest() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.apiRequestAt
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	api := strings.HasPrefix(r.URL.Path, routeAPI)
	if api {
		s.mu.Lock()
		s.apiRequestAt = s.config.Now()
		s.mu.Unlock()
	}

	address := loopbackHost + ":" + strconv.Itoa(s.config.Port)
	// A page reaching the server under another name, as after DNS rebinding, is turned away before anything else.
	if r.Host != address {
		refuse(w, http.StatusMisdirectedRequest, "the request's Host is not the server's address")
		return
	}
	file := strings.HasPrefix(r.URL.Path, routeFile)
	if api {
		bearer, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || !s.tokenMatches(bearer) {
			refuse(w, http.StatusUnauthorized, "the request carries no valid token")
			return
		}
	}
	if file && !s.tokenMatches(r.URL.Query().Get("t")) {
		refuse(w, http.StatusUnauthorized, "the request carries no valid token")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == routePage:
		s.servePage(w)
	case r.Method == http.MethodGet && r.URL.Path == routeBoard:
		s.serveBoard(w, r)
	case r.Method == http.MethodGet && file:
		s.serveFile(w, strings.TrimPrefix(r.URL.Path, routeFile))
	case r.Method == http.MethodPost && r.URL.Path == routeWrite:
		s.serveWrite(w, r, "http://"+address)
	default:
		refuse(w, http.StatusNotFound, "not found")
	}
}

// tokenMatches reports whether token is the server's, in constant time.
func (s *Server) tokenMatches(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

func (s *Server) servePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", s.policy)
	writeBody(w, boardengine.ViewerPage())
}

func (s *Server) serveBoard(w http.ResponseWriter, r *http.Request) {
	// An `after` of "" is the revision of an absent board.json, so it waits like any other.
	if query := r.URL.Query(); query.Has("after") {
		if revision, changed := s.awaitRevisionOtherThan(r.Context(), query.Get("after")); !changed {
			answer(w, http.StatusOK, map[string]any{"ok": true, "unchanged": true, "revision": revision})
			return
		}
	}
	// The revision is read before the view, so a write between the two is asked about again rather than missed.
	s.mu.Lock()
	revision := s.revision
	s.mu.Unlock()
	view, err := s.config.Board.View()
	if err != nil {
		refuse(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "board": view})
}

// awaitRevisionOtherThan waits until the board's revision differs from after, and reports whether it does.
// It gives up at the long-poll bound, when ctx ends and when the server drains.
// The revision it returns is the board's latest.
func (s *Server) awaitRevisionOtherThan(ctx context.Context, after string) (string, bool) {
	bound := time.NewTimer(s.config.LongPoll)
	defer bound.Stop()
	for {
		s.mu.Lock()
		revision, changed := s.revision, s.changed
		s.mu.Unlock()
		if revision != after {
			return revision, true
		}
		select {
		case <-changed:
		case <-bound.C:
			return revision, false
		case <-ctx.Done():
			return revision, false
		case <-s.drain:
			return revision, false
		}
	}
}

// serveFile serves name from the board directory only when the current projection names it,
// so no other file of the directory is reachable.
func (s *Server) serveFile(w http.ResponseWriter, name string) {
	view, err := s.config.Board.View()
	if err != nil {
		refuse(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	listed := map[string]bool{}
	for _, project := range view.Projects {
		listed[project.DesignFile] = true
	}
	for _, entry := range view.Entries {
		listed[entry.DesignFile] = true
		listed[entry.DecisionFile] = true
	}
	if name == "" || strings.ContainsAny(name, `/\`) || !listed[name] {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	content, err := os.ReadFile(filepath.Join(s.config.BoardPath, name))
	if errors.Is(err, fs.ErrNotExist) {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		refuse(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeBody(w, content)
}

func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request, origin string) {
	if r.Header.Get("Content-Type") != "application/json" {
		refuse(w, http.StatusUnsupportedMediaType, "a write is sent as application/json")
		return
	}
	if sent := r.Header.Get("Origin"); sent != "" && sent != origin {
		refuse(w, http.StatusForbidden, "the write comes from another origin")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		refuse(w, http.StatusRequestEntityTooLarge, "the write is larger than the server takes")
		return
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	request, err := decodeWriteRequest(body)
	if err != nil {
		refuse(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	entry, err := s.applyWrite(request)
	var stale *boardengine.StaleEntryError
	switch {
	case errors.As(err, &stale):
		answer(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error(), "entry": stale.Fresh})
	case err != nil:
		refuse(w, http.StatusUnprocessableEntity, err.Error())
	default:
		answer(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
	}
}

// refuse answers status with message as the error.
func refuse(w http.ResponseWriter, status int, message string) {
	answer(w, status, map[string]any{"ok": false, "error": message})
}

// answer writes body as the JSON response of the given status.
func answer(w http.ResponseWriter, status int, body map[string]any) {
	data, err := json.Marshal(body)
	if err != nil {
		logger.Warn("boardserve: a response did not encode", "error", err)
		status, data = http.StatusInternalServerError, []byte(`{"ok":false,"error":"the response did not encode"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeBody(w, data)
}

// writeBody writes data to the response.
func writeBody(w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		// The client went away, so nothing is left to answer.
		logger.Debug("boardserve: a response was not delivered", "error", err)
	}
}

// inlineElements match the page's inline script and style elements; an element with attributes is not inline.
var inlineElements = map[string]*regexp.Regexp{
	"script": regexp.MustCompile(`(?s)<script>(.*?)</script>`),
	"style":  regexp.MustCompile(`(?s)<style>(.*?)</style>`),
}

// contentSecurityPolicy is the policy of page.
// It admits page's own inline elements by their hashes, requests to the server itself, and nothing else.
func contentSecurityPolicy(page []byte) string {
	hashes := func(element string) string {
		var sources []string
		for _, match := range inlineElements[element].FindAllSubmatch(page, -1) {
			sum := sha256.Sum256(match[1])
			sources = append(sources, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
		return strings.Join(sources, " ")
	}
	return "default-src 'none'; script-src " + hashes("script") + "; style-src " + hashes("style") +
		"; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}
