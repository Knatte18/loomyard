// state.go places the board server's files in its state directory and reads what they say:
// the server's state, whether a server holds the lock, the page's address, the launcher and the probe.

package boardserve

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// The files of the state directory.
const (
	stateFileName     = "state.json"
	stateLockFileName = "state.lock"
	lockFileName      = "server.lock"
	launcherFileName  = "launcher.html"
	logFileName       = "server.log"
)

// probeTimeout bounds one Probe.
const probeTimeout = 5 * time.Second

// StatePaths are the board server's state directory and the files in it.
type StatePaths struct {
	Dir string
	// State is the file naming the running server.
	State string
	// Lock is held by the one server that serves.
	Lock string
	// Launcher is the page that sends a browser on to the server.
	Launcher string
	// Log is the log of the server that last held Lock.
	Log string
}

// Paths returns the board server's files under the hub's scratch directory.
func Paths(hubScratchDir string) StatePaths {
	dir := filepath.Join(hubScratchDir, lymdirs.BoardServeDirName)
	return StatePaths{
		Dir:      dir,
		State:    filepath.Join(dir, stateFileName),
		Lock:     filepath.Join(dir, lockFileName),
		Launcher: filepath.Join(dir, launcherFileName),
		Log:      filepath.Join(dir, logFileName),
	}
}

// State is what a running server says of itself: how to reach it and the secret it asks for.
type State struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// stateLockPath is the lock that guards the state file; the server's own lock is held for its whole life.
func stateLockPath(paths StatePaths) string {
	return filepath.Join(paths.Dir, stateLockFileName)
}

// ReadState returns the state the last server wrote, and false when there is none.
// The state may be left over from a server that has exited.
func ReadState(paths StatePaths) (State, bool, error) {
	return state.ReadJSON[State](paths.State, stateLockPath(paths))
}

// Live reports whether a server runs or is starting: its lock is held.
// It creates nothing when the state directory does not exist.
func Live(paths StatePaths) (bool, error) {
	if _, err := os.Stat(paths.Dir); errors.Is(err, fs.ErrNotExist) {
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

// loopbackAddress is the address a server on port listens on and answers to.
func loopbackAddress(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

// PageAddress is the address of the page a server on port serves, without the token.
func PageAddress(port int) string {
	return "http://" + loopbackAddress(port) + routePage
}

// WriteLauncher writes the page that sends a browser to server's page with its token, and returns the page's path.
// The token stands only in the address's fragment, which a browser never sends, and in this owner-only file,
// so it appears on no command line.
func WriteLauncher(paths StatePaths, server State) (string, error) {
	page := fmt.Sprintf("<!doctype html>\n<meta charset=\"utf-8\">\n<title>Board</title>\n<script>location.replace(%q)</script>\n",
		PageAddress(server.Port)+"#"+server.Token)
	if err := fsx.AtomicWriteBytes(paths.Launcher, []byte(page)); err != nil {
		return "", fmt.Errorf("write the launcher: %w", err)
	}
	if err := hardenFile(paths.Launcher); err != nil {
		return "", err
	}
	return paths.Launcher, nil
}

// Probe reports, by a nil error, that server answers a request for the board with its token.
func Probe(server State) error {
	request, err := http.NewRequest(http.MethodGet, "http://"+loopbackAddress(server.Port)+routeBoard, nil)
	if err != nil {
		return fmt.Errorf("probe the board server: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token)
	client := &http.Client{Timeout: probeTimeout, Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("probe the board server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe the board server: it answered %s", response.Status)
	}
	return nil
}
