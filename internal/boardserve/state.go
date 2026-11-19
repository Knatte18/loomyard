// state.go holds the board server's files under the hub's scratch directory and the reads that describe a running server:
// the state record, the liveness test, the launcher page and the probe.

package boardserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// File names under the server's state directory.
const (
	stateFileName    = "state.json"
	stateLockName    = "state.lock"
	serverLockName   = "server.lock"
	launcherFileName = "launcher.html"
	serverLogName    = "server.log"
)

// StatePaths holds where the board server keeps its files.
type StatePaths struct {
	// Dir is the state directory.
	Dir string
	// State is the record of the running server's pid, port and token.
	State string
	// StateLock guards State; it is never the server's own lock.
	StateLock string
	// Lock is held for as long as a server serves the hub.
	Lock string
	// Launcher is the page that sends the browser to the server with its token.
	Launcher string
	// Log is the server's log.
	Log string
}

// Paths returns the board server's files under hubScratchDir, the hub's board-level `.lym`.
func Paths(hubScratchDir string) StatePaths {
	dir := filepath.Join(hubScratchDir, lymdirs.BoardServeDirName)
	return StatePaths{
		Dir:       dir,
		State:     filepath.Join(dir, stateFileName),
		StateLock: filepath.Join(dir, stateLockName),
		Lock:      filepath.Join(dir, serverLockName),
		Launcher:  filepath.Join(dir, launcherFileName),
		Log:       filepath.Join(dir, serverLogName),
	}
}

// State is the record a serving board server leaves for `lym board open`.
// The token is the bearer credential of the server's API, so the file holding it is readable by its owner alone.
type State struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// ReadState reads the state file.
// It reports false, with no error, when none exists.
// A file that holds a key State does not declare reads without it.
func ReadState(paths StatePaths) (State, bool, error) {
	return state.ReadJSON[State](paths.State, paths.StateLock)
}

// writeState writes the state file readable by its owner alone.
func writeState(paths StatePaths, st State) error {
	if err := state.WriteJSON(paths.State, paths.StateLock, st); err != nil {
		return fmt.Errorf("write the board server state: %w", err)
	}
	return hardenFile(paths.State)
}

// Live reports whether a board server holds the hub's lock, which is a serving or a starting server.
// The lock is tried without blocking and released at once when it was free.
// A free lock reads not live, whatever a state file says.
// A state directory that does not exist reads as no server and is not created.
func Live(paths StatePaths) (bool, error) {
	if _, err := os.Stat(paths.Dir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(paths.Lock)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// PageAddress returns the address of the page a server on port serves.
// It names the loopback address and never localhost, which the server's Host check refuses.
func PageAddress(port int) string {
	return fmt.Sprintf("http://%s:%d/", loopbackHost, port)
}

// WriteLauncher writes the launcher page for the server st describes and returns its path.
// The page's script replaces its location with the server's address and the token after a `#`,
// so the token travels in the fragment, which a browser never sends to a server.
// The file is readable by its owner alone, since it holds the token.
func WriteLauncher(paths StatePaths, st State) (string, error) {
	target, err := json.Marshal(PageAddress(st.Port) + "#" + st.Token)
	if err != nil {
		return "", fmt.Errorf("encode the launcher address: %w", err)
	}
	page := "<!doctype html>\n<meta charset=\"utf-8\">\n<title>board</title>\n<script>location.replace(" + string(target) + ")</script>\n"
	if err := os.MkdirAll(paths.Dir, 0o755); err != nil {
		return "", fmt.Errorf("create the board server state directory: %w", err)
	}
	if err := os.WriteFile(paths.Launcher, []byte(page), 0o600); err != nil {
		return "", fmt.Errorf("write the launcher page: %w", err)
	}
	if err := hardenFile(paths.Launcher); err != nil {
		return "", err
	}
	return paths.Launcher, nil
}

// Probe asks the server st describes for its board and succeeds when it answers 200 within timeout.
func Probe(st State, timeout time.Duration) error {
	request, err := http.NewRequest(http.MethodGet, PageAddress(st.Port)+"api/board", nil)
	if err != nil {
		return fmt.Errorf("build the probe request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+st.Token)
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return fmt.Errorf("probe the board server on port %d: %w", st.Port, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe the board server on port %d: it answered %s", st.Port, response.Status)
	}
	return nil
}
