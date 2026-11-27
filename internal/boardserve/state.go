// state.go holds what a running board server is found by.
// That is its file paths, state record, lock liveness read, launcher page and probe.

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

// The names of the server's files in its state directory.
const (
	stateFileName    = "state.json"
	lockFileName     = "server.lock"
	launcherFileName = "launcher.html"
	logFileName      = "server.log"
)

// stateLockSuffix names the lock that guards the state file's reads and writes, beside that file.
const stateLockSuffix = ".lock"

// probeTimeout bounds one probe of a server.
const probeTimeout = 5 * time.Second

// StatePaths are the paths of one hub's board server files.
type StatePaths struct {
	// Dir is the state directory that holds the others.
	Dir      string
	State    string
	Lock     string
	Launcher string
	Log      string
}

// Paths returns the board server's paths under hubScratchDir, the hub's board-level never-tracked directory.
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

// State is how a serving board server is reached.
type State struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
	// Token is the secret every /api and file request of this server carries.
	Token string `json:"token"`
}

// writeState writes server as the state file, readable by the user alone.
func writeState(paths StatePaths, server State) error {
	if err := state.WriteJSON(paths.State, paths.State+stateLockSuffix, server); err != nil {
		return err
	}
	return hardenFile(paths.State)
}

// ReadState reads the state file, and reports false when there is none.
// The file may be a leftover of a server that has exited.
func ReadState(paths StatePaths) (State, bool, error) {
	return state.ReadJSON[State](paths.State, paths.State+stateLockSuffix)
}

// Live reports whether a board server holds the lock, so is starting or serving.
// A missing state directory reads as no server, and nothing is created.
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

// PageAddress is the address of the viewer page a server on port serves.
func PageAddress(port int) string {
	return "http://" + loopbackHost + ":" + strconv.Itoa(port) + routePage
}

// launcherPage replaces its own location with the address it is filled with.
const launcherPage = `<!doctype html>
<meta charset="utf-8">
<title>board</title>
<script>location.replace(%q);</script>
`

// WriteLauncher writes the page that sends a browser to the server on port, and returns its path.
// The token travels in the address's fragment, so it reaches no command line and no request line.
// The file is readable by the user alone.
func WriteLauncher(paths StatePaths, port int, token string) (string, error) {
	page := fmt.Sprintf(launcherPage, PageAddress(port)+"#"+token)
	if err := fsx.AtomicWriteBytes(paths.Launcher, []byte(page)); err != nil {
		return "", fmt.Errorf("write the board launcher: %w", err)
	}
	if err := hardenFile(paths.Launcher); err != nil {
		return "", err
	}
	return paths.Launcher, nil
}

// Probe asks the server that server describes for the board with its token.
// It returns nil only when the server answers 200.
func Probe(server State) error {
	request, err := http.NewRequest(http.MethodGet, "http://"+loopbackHost+":"+strconv.Itoa(server.Port)+routeBoard, nil)
	if err != nil {
		return err
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
