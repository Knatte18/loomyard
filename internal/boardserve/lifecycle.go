// lifecycle.go holds what finds one board server: its state directory, state file, liveness, address and launcher.

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

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// The names of the files in a server's state directory.
const (
	stateFileName    = "state.json"
	lockFileName     = "server.lock"
	launcherFileName = "launcher.html"
	logFileName      = "server.log"
)

// stateLockSuffix is appended to the state file's path to name the lock its reads and writes take.
const stateLockSuffix = ".lock"

// probeTimeout bounds one probe of a server.
const probeTimeout = 2 * time.Second

// StatePaths are the paths of one board server's state.
type StatePaths struct {
	// Dir is the state directory.
	Dir string
	// State is the file naming the serving process, its port and its token.
	State string
	// Lock is the file whose lock the serving process holds.
	Lock string
	// Launcher is the page that opens the server's page with its token.
	Launcher string
	// Log is the file the server logs to.
	Log string
}

// Paths returns the state paths of the board server under hubScratchDir, the hub's never-tracked directory.
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

// State is what a serving process records about itself.
type State struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
	// Token is the secret a request for board data carries.
	Token string `json:"token"`
}

// ReadState returns the recorded state, and false when none is recorded.
// A recorded state says nothing about whether its process still serves.
func ReadState(paths StatePaths) (State, bool, error) {
	return state.ReadJSON[State](paths.State, paths.State+stateLockSuffix)
}

// writeState records st, readable by its owner alone.
func writeState(paths StatePaths, st State) error {
	if err := state.WriteJSON(paths.State, paths.State+stateLockSuffix, st); err != nil {
		return err
	}
	return hardenFile(paths.State)
}

// Live reports whether a server is running or starting: its lock is held.
// The lock is tried without blocking and released at once when it was free.
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

// PageAddress returns the address of the page a server on port serves.
// It names the loopback IP, which is the only host such a server answers to.
func PageAddress(port int) string {
	return "http://" + loopbackAddress(port) + routePage
}

// loopbackAddress returns the loopback IP and port a server on port is reached at.
func loopbackAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// WriteLauncher writes the page that sends a browser to the server of st, and returns its path.
// It hands the browser the token in the address fragment, so the token appears on no command line.
func WriteLauncher(paths StatePaths, st State) (string, error) {
	page := "<!doctype html>\n<meta charset=\"utf-8\">\n<script>location.replace(" +
		strconv.Quote(PageAddress(st.Port)+"#"+st.Token) + ")</script>\n"
	if err := os.WriteFile(paths.Launcher, []byte(page), 0o600); err != nil {
		return "", fmt.Errorf("write the board launcher: %w", err)
	}
	if err := hardenFile(paths.Launcher); err != nil {
		return "", err
	}
	return paths.Launcher, nil
}

// Probe asks the server of st for its board, and succeeds only when it answers with the board.
func Probe(st State) error {
	request, err := http.NewRequest(http.MethodGet, "http://"+loopbackAddress(st.Port)+routeBoard, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+st.Token)
	// A proxy of the environment must never see a loopback request or its token.
	client := &http.Client{Timeout: probeTimeout, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if err := response.Body.Close(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the board server answered %s", response.Status)
	}
	return nil
}
