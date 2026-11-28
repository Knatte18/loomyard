// server.go is the board server's HTTP handler: its request checks, its routes and the long poll.

package boardserve

import (
	"bytes"
	"context"
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

// The server's routes, which the viewer page spells too.
const (
	routePage  = "/"
	routeBoard = "/api/board"
	routeWrite = "/api/write"
	// routeFile is followed by the file's name.
	routeFile = "/file/"
)

// maxBodyBytes bounds the body of a write.
const maxBodyBytes = 1 << 20

// apiPrefix is the path every route that needs the bearer token starts with.
const apiPrefix = "/api/"

// ServerConfig is what a Server serves and how it judges a request.
type ServerConfig struct {
	// Board is the board the server reads and writes.
	Board *boardengine.Board
	// BoardPath is the board's directory.
	BoardPath string
	// Token is the secret a request for board data carries.
	Token string
	// Port is the bound loopback port, which the Host and Origin of a request must name.
	Port int
	// PollBound is the longest a long poll waits for a change.
	PollBound time.Duration
	// Now is the clock the server records requests with.
	Now func() time.Time
}

// Server is the board server's HTTP handler, safe for concurrent use.
type Server struct {
	config ServerConfig
	// policy is the viewer page's Content-Security-Policy.
	policy string

	mu       sync.Mutex
	revision string
	// changed is closed and replaced when the revision is set.
	changed chan struct{}
	// draining is closed when long polls must no longer wait.
	draining chan struct{}
	drain    sync.Once
	lastAPI  time.Time
}

// NewServer returns a Server over config, starting at the board's current revision.
func NewServer(config ServerConfig) (*Server, error) {
	revision, err := boardengine.BoardRevision(config.BoardPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		config:   config,
		policy:   contentSecurityPolicy(boardengine.ViewerPage()),
		revision: revision,
		changed:  make(chan struct{}),
		draining: make(chan struct{}),
	}, nil
}

// inlineElements match the viewer page's inline script and style elements, capturing their content.
var inlineElements = []struct {
	directive string
	element   *regexp.Regexp
}{
	{"script-src", regexp.MustCompile(`(?s)<script>(.*?)</script>`)},
	{"style-src", regexp.MustCompile(`(?s)<style>(.*?)</style>`)},
}

// contentSecurityPolicy returns the policy that admits only page's own inline script and style, by their hashes.
func contentSecurityPolicy(page []byte) string {
	// A browser hashes an element's content with its line endings normalized, whatever the checkout wrote.
	page = bytes.ReplaceAll(page, []byte("\r\n"), []byte("\n"))
	directives := []string{"default-src 'none'"}
	for _, inline := range inlineElements {
		directive := inline.directive
		for _, match := range inline.element.FindAllSubmatch(page, -1) {
			sum := sha256.Sum256(match[1])
			directive += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		}
		directives = append(directives, directive)
	}
	directives = append(directives, "connect-src 'self'", "base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'")
	return strings.Join(directives, "; ")
}

// BoardChanged records the board's revision and wakes every waiting long poll.
func (s *Server) BoardChanged(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = revision
	close(s.changed)
	s.changed = make(chan struct{})
}

// beginDrain answers every waiting long poll "unchanged" and makes later ones answer at once.
func (s *Server) beginDrain() {
	s.drain.Do(func() { close(s.draining) })
}

// lastAPIRequest returns the time of the latest /api request, zero before the first.
func (s *Server) lastAPIRequest() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAPI
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")

	address := fmt.Sprintf("127.0.0.1:%d", s.config.Port)
	if r.Host != address {
		writeRefusal(w, http.StatusMisdirectedRequest, "the request does not name this server as its host")
		return
	}

	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, apiPrefix):
		s.mu.Lock()
		s.lastAPI = s.config.Now()
		s.mu.Unlock()
		if !sameSecret(r.Header.Get("Authorization"), "Bearer "+s.config.Token) {
			writeRefusal(w, http.StatusUnauthorized, "the request carries no valid token")
			return
		}
	case strings.HasPrefix(path, routeFile):
		if !sameSecret(r.URL.Query().Get("t"), s.config.Token) {
			writeRefusal(w, http.StatusUnauthorized, "the request carries no valid token")
			return
		}
	}

	switch {
	case path == routePage && r.Method == http.MethodGet:
		s.servePage(w)
	case path == routeBoard && r.Method == http.MethodGet:
		s.serveBoard(w, r)
	case path == routeWrite && r.Method == http.MethodPost:
		s.serveWrite(w, r, "http://"+address)
	case strings.HasPrefix(path, routeFile) && r.Method == http.MethodGet:
		s.serveFile(w, strings.TrimPrefix(path, routeFile))
	default:
		writeRefusal(w, http.StatusNotFound, "no such route")
	}
}

// sameSecret reports whether got equals want, taking the same time wherever they differ.
func sameSecret(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// writeJSON answers status with body encoded as JSON.
func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status is sent already, so a failed write has nobody left to tell.
	_ = json.NewEncoder(w).Encode(body)
}

// writeRefusal answers status with the refusal message.
func writeRefusal(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message})
}

// servePage answers the viewer page under its Content-Security-Policy.
func (s *Server) servePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", s.policy)
	// The status is sent with the first byte, so a failed write has nobody left to tell.
	_, _ = w.Write(boardengine.ViewerPage())
}

// serveBoard answers the view projection with the revision it is at least as new as.
// A request naming the current revision in `after` first waits for another one.
func (s *Server) serveBoard(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var revision string
	if query.Has("after") {
		var changed bool
		revision, changed = s.awaitChange(r.Context(), query.Get("after"))
		if !changed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unchanged": true, "revision": revision})
			return
		}
	} else {
		s.mu.Lock()
		revision = s.revision
		s.mu.Unlock()
	}
	// The revision is read before the view, so a change in between is announced again rather than missed.
	view, err := s.config.Board.View()
	if err != nil {
		writeRefusal(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "board": view})
}

// awaitChange waits until the revision differs from after, and returns it with true.
// It returns the unchanged revision with false when the poll bound passes, ctx ends or the server drains.
func (s *Server) awaitChange(ctx context.Context, after string) (string, bool) {
	bound := time.NewTimer(s.config.PollBound)
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
		case <-s.draining:
			return revision, false
		}
	}
}

// serveFile answers the board file name as plain text, only when the current projection names it.
func (s *Server) serveFile(w http.ResponseWriter, name string) {
	view, err := s.config.Board.View()
	if err != nil {
		writeRefusal(w, http.StatusInternalServerError, err.Error())
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
	delete(listed, "")
	if !listed[name] {
		writeRefusal(w, http.StatusNotFound, "no such file")
		return
	}
	content, err := os.ReadFile(filepath.Join(s.config.BoardPath, name))
	if errors.Is(err, os.ErrNotExist) {
		writeRefusal(w, http.StatusNotFound, "no such file")
		return
	}
	if err != nil {
		writeRefusal(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// The status is sent with the first byte, so a failed write has nobody left to tell.
	_, _ = w.Write(content)
}

// serveWrite checks that the request is a write from the server's own origin, and applies it.
func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request, origin string) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeRefusal(w, http.StatusUnsupportedMediaType, "a write is sent as application/json")
		return
	}
	if named := r.Header.Get("Origin"); named != "" && named != origin {
		writeRefusal(w, http.StatusForbidden, "a write is sent by the server's own page")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeRefusal(w, http.StatusRequestEntityTooLarge, "the write is too large")
		return
	}
	if err != nil {
		writeRefusal(w, http.StatusBadRequest, err.Error())
		return
	}
	request, err := decodeWriteRequest(body)
	if err != nil {
		writeRefusal(w, http.StatusBadRequest, "decode the write: "+err.Error())
		return
	}

	entry, err := s.applyWrite(request)
	var stale *boardengine.StaleEntryError
	switch {
	case errors.As(err, &stale):
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": stale.Error(), "entry": stale.Fresh})
	case err != nil:
		writeRefusal(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
	}
}
