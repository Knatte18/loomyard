// state.go — muxpoc state persistence layer.
//
// MuxpocState holds psmux session, socket, stripped environment keys, and pane
// metadata. LoadState/SaveState/DeleteState manage persistence with atomic writes
// and advisory locking. sanitizeEnv removes CLAUDECODE and CLAUDE_CODE_* variables
// to prevent child processes from inheriting them. socketName derives a stable,
// sanitised socket name from cwd. newSessionID generates UUID v4 for sessions.

package muxpoccli

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/state"
)

const (
	stateRelPath = ".lyx/muxpoc-state.json"
)

// Pane represents a single psmux pane in the session.
type Pane struct {
	ID        string `json:"id"`         // psmux pane ID, e.g. "%3"
	SessionID string `json:"session_id"` // claude --session-id value
	Kind      string `json:"kind"`       // "main" or "review"
}

// MuxpocState holds the persistent state of a muxpoc session.
type MuxpocState struct {
	Session     string   `json:"session"`      // psmux session name
	Socket      string   `json:"socket"`       // psmux -L socket name
	StrippedEnv []string `json:"stripped_env"` // keys removed from env at server spawn
	Panes       []Pane   `json:"panes"`        // panes in the session
}

// LoadState reads the muxpoc state from cwd/.lyx/muxpoc-state.json under a
// shared read lock. Returns (nil, nil) if the file is absent. Returns (nil, error)
// if the file is corrupt/unparseable or on other read errors. Returns (*state, nil) on success.
func LoadState(cwd string) (*MuxpocState, error) {
	statePath := filepath.Join(cwd, stateRelPath)
	lockPath := statePath + ".lock"

	v, found, err := state.ReadJSON[MuxpocState](statePath, lockPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &v, nil
}

// SaveState creates .lyx/ if absent, acquires an exclusive write lock on
// .lyx/muxpoc-state.json.lock, and writes the state atomically (temp file + rename).
// Releases the lock via defer.
func SaveState(cwd string, s *MuxpocState) error {
	if s == nil {
		return fmt.Errorf("cannot save nil state")
	}

	statePath := filepath.Join(cwd, stateRelPath)
	lockPath := statePath + ".lock"

	return state.WriteJSON(statePath, lockPath, s)
}

// DeleteState removes .lyx/muxpoc-state.json. Returns nil if the file is absent.
func DeleteState(cwd string) error {
	statePath := filepath.Join(cwd, stateRelPath)
	if err := os.Remove(statePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete state: %w", err)
	}
	return nil
}

// newSessionID generates a UUID v4 from crypto/rand: read 16 bytes, set version
// bits (4) and variant bits (RFC 4122), and format as
// xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx.
func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}

	// Set version to 4 (bits 12-15 of time_hi_and_version)
	b[6] = (b[6] & 0x0f) | 0x40

	// Set variant to RFC 4122 (bits 6-7 of clock_seq_hi_and_reserved)
	b[8] = (b[8] & 0x3f) | 0x80

	// Format as xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// sanitizeEnv returns a new slice with every entry whose key is CLAUDECODE
// or starts with CLAUDE_CODE_ removed. Key is the part before =.
func sanitizeEnv(environ []string) []string {
	result := []string{}
	for _, entry := range environ {
		key := strings.SplitN(entry, "=", 2)[0]
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// strippedEnvKeys returns the keys (not full KEY=VALUE strings) that sanitizeEnv
// would remove, in the same order as they appear in environ.
func strippedEnvKeys(environ []string) []string {
	result := []string{}
	for _, entry := range environ {
		key := strings.SplitN(entry, "=", 2)[0]
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			result = append(result, key)
		}
	}
	return result
}

// socketName derives a stable socket name: take filepath.Base(cwd), replace every
// character that is not [a-zA-Z0-9_-] with -, lowercase, and prefix with muxpoc-.
// Example: C:\Code\loomyard\wts\loomyard-mux-design → muxpoc-loomyard-mux-design.
func socketName(cwd string) string {
	baseName := filepath.Base(cwd)
	// Replace non-alphanumeric, non-dash, non-underscore characters with dash
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	sanitised := re.ReplaceAllString(baseName, "-")
	// Lowercase
	sanitised = strings.ToLower(sanitised)
	// Prefix
	return "muxpoc-" + sanitised
}
