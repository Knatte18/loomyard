// state.go holds the server's state directory and its paths.
// It also holds the state record, the liveness read, the launcher and the probe.

package boardserve

import (
	"encoding/json"
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
	stateFileName    = "state.json"
	lockFileName     = "server.lock"
	launcherFileName = "launcher.html"
	logFileName      = "server.log"
)

// stateLockSuffix names the lock of the state record's reads and writes after the record's own path.
const stateLockSuffix = ".lock"

// probeTimeout bounds one probe of a server.
const probeTimeout = 5 * time.Second

// StatePaths are the server's state directory and the files it holds.
type StatePaths struct {
	Dir string
	// State records the serving process.
	State string
	// Lock is held for as long as one server lives.
	Lock string
	// Launcher is the page that sends a browser to the server.
	Launcher string
	Log      string
}

// Paths returns the server's paths under hubScratchDir, the hub's machine-local scratch directory.
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

// State is what a serving server records of itself.
// It holds the token, so its file is readable by its user alone.
type State struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// writeState records current as the serving process, readable by its user alone.
func writeState(paths StatePaths, current State) error {
	if err := state.WriteJSON(paths.State, paths.State+stateLockSuffix, current); err != nil {
		return fmt.Errorf("write the board server's state: %w", err)
	}
	return hardenFile(paths.State)
}

// ReadState reads the state a server recorded, and reports whether there is one.
// A record outlives its server, so it says nothing of whether the server still runs.
func ReadState(paths StatePaths) (State, bool, error) {
	return state.ReadJSON[State](paths.State, paths.State+stateLockSuffix)
}

// Live reports whether a server is starting or serving, by whether its lock is held.
// It creates no directory.
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

// PageAddress is the address of the page a server serves on port, without a token.
func PageAddress(port int) string {
	return "http://" + loopbackHost + ":" + strconv.Itoa(port) + routePage
}

// WriteLauncher writes the page that sends a browser to the server on port with token, and returns its path.
// The token travels in the address fragment of a file readable by its user alone,
// so it appears on no command line.
func WriteLauncher(paths StatePaths, port int, token string) (string, error) {
	// A JSON string is a script string literal, and its escaping keeps it inside the script element.
	address, err := json.Marshal(PageAddress(port) + "#" + token)
	if err != nil {
		return "", fmt.Errorf("encode the launcher's address: %w", err)
	}
	page := "<!doctype html>\n<meta charset=\"utf-8\">\n<title>Board</title>\n<script>location.replace(" + string(address) + ")</script>\n"
	if err := fsx.AtomicWriteBytes(paths.Launcher, []byte(page)); err != nil {
		return "", fmt.Errorf("write the board launcher: %w", err)
	}
	if err := hardenFile(paths.Launcher); err != nil {
		return "", err
	}
	return paths.Launcher, nil
}

// Probe asks the server current describes for the board, and returns nil when it answers it.
func Probe(current State) error {
	address := loopbackHost + ":" + strconv.Itoa(current.Port)
	request, err := http.NewRequest(http.MethodGet, "http://"+address+routeBoard, nil)
	if err != nil {
		return fmt.Errorf("build the probe: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+current.Token)
	// No proxy and no kept connection: the probe is one direct request to this machine.
	client := &http.Client{Timeout: probeTimeout, Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("probe the board server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe the board server at %s: it answered %s", address, response.Status)
	}
	return nil
}
