//go:build !windows

// placewindowchild_integration_test.go runs the detached placement child of a built binary against real hubs.
// It also resolves a placement request and the retile's mode lookup over a real hub.
// One test runs the retile verb outside a hub.
// The child's PATH holds no program reaching a desktop.
// The window command is POSIX shell, so Windows is left out.

package reedcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/kwinplace"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

const (
	markerFile  = "marker"
	releaseFile = "release"
)

// setMachineDesktop sets the desktop keys of the hub's machine layer.
func setMachineDesktop(t *testing.T, h *hubforge.Hub, pairs ...yamlengine.KV) {
	t.Helper()
	board := fabriclayout.BoltDir(h.Location.HubPath)
	if err := os.MkdirAll(configengine.ScratchDir(board), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", configengine.ScratchDir(board), err)
	}
	dirs := configengine.Dirs{Board: board, Anchor: h.Location.AnchorPath()}
	if _, err := configengine.Set(dirs, desktophook.ConfigSpec(), configengine.LayerMachine, pairs); err != nil {
		t.Fatalf("set the machine's desktop keys: %v", err)
	}
}

// eventually polls until done reports true, failing the test after a minute.
func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.After(time.Minute)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for !done() {
		select {
		case <-deadline:
			t.Fatalf("still waiting for %s after a minute", what)
		case <-tick.C:
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// logsHold reports whether any log of the hub's logs directory holds every one of the wanted texts on one line.
func logsHold(hubPath string, wanted ...string) bool {
	logs, _ := filepath.Glob(filepath.Join(fabriclayout.HubLogsDir(hubPath), "*"))
	return slices.ContainsFunc(logs, func(log string) bool {
		data, _ := os.ReadFile(log)
		return slices.ContainsFunc(strings.Split(string(data), "\n"), func(line string) bool {
			return !slices.ContainsFunc(wanted, func(text string) bool { return !strings.Contains(line, text) })
		})
	})
}

func placementRecords(t *testing.T, hubPath, owner string) int {
	t.Helper()
	records, err := runproc.List(fabriclayout.RunProcDir(hubPath), owner)
	if err != nil {
		t.Fatalf("List(%s) = %v", owner, err)
	}
	count := 0
	for _, record := range records {
		if record.Role == runproc.RolePlacement {
			count++
		}
	}
	return count
}

func settled(hubPath, worktree string) bool {
	entry, found := kwinplace.ReadPlaces(kwinplace.PlacesPath(fabriclayout.BoltDir(hubPath)))[worktree]
	return found && entry.Settled()
}

// TestPlacementRequest resolves the prime's window and a task's under it, over one hub.
func TestPlacementRequest(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task")
	setMachineDesktop(t, h,
		yamlengine.KV{Key: "window_placement", Value: "same"},
		yamlengine.KV{Key: "tiling.min_tile_width", Value: "400"},
		yamlengine.KV{Key: "tiling.min_tile_height", Value: "300"})
	taskDirs := configengine.Dirs{Board: fabriclayout.BoltDir(h.Location.HubPath), Anchor: h.PairCodeWorktree("task")}
	if _, err := configengine.Set(taskDirs, dobbyengine.ConfigSpec(), configengine.LayerWorktree,
		[]yamlengine.KV{{Key: dobbyengine.WindowPlacementKey, Value: "next"}}); err != nil {
		t.Fatalf("set the task's beam key: %v", err)
	}
	primeName := h.Location.WorktreeName
	wantWorktrees := []string{primeName, "task"}
	slices.Sort(wantWorktrees)

	tests := []struct {
		name       string
		worktree   string
		prime      bool
		wantParent string
		wantMode   string
	}{
		{name: "PrimeTakesTheMachinesMode", worktree: primeName, prime: true, wantMode: "same"},
		{name: "TaskTakesItsOwnModeUnderThePrime", worktree: "task", wantParent: primeName, wantMode: "next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location, err := lymcwd.NamedWorktree(h.Location.HubPath, tt.worktree)
			if err != nil {
				t.Fatal(err)
			}
			window := handedWindow{hubPath: h.Location.HubPath, worktree: tt.worktree, title: "title", pid: 7, prime: tt.prime}

			got, err := placementRequest(location, fabricengine.ConfigDirs(location), window)
			if err != nil {
				t.Fatalf("placementRequest() error = %v", err)
			}
			if got.Parent != tt.wantParent || got.Mode != tt.wantMode || got.MinTileWidth != 400 || got.MinTileHeight != 300 {
				t.Errorf("placementRequest() = parent %q, mode %q, tile %dx%d; want %q, %q, 400x300",
					got.Parent, got.Mode, got.MinTileWidth, got.MinTileHeight, tt.wantParent, tt.wantMode)
			}
			worktrees, err := got.Worktrees()
			slices.Sort(worktrees)
			if err != nil || !slices.Equal(worktrees, wantWorktrees) {
				t.Errorf("Worktrees() = %q, %v; want %q", worktrees, err, wantWorktrees)
			}
		})
	}
}

// TestRetileWindowMode reads the prime's mode and a task's over one hub.
// The prime records the task as its parent, so a prime read under its own name would take the task's mode.
func TestRetileWindowMode(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task")
	taskDirs := configengine.Dirs{Board: fabriclayout.BoltDir(h.Location.HubPath), Anchor: h.PairCodeWorktree("task")}
	if _, err := configengine.Set(taskDirs, dobbyengine.ConfigSpec(), configengine.LayerWorktree,
		[]yamlengine.KV{{Key: dobbyengine.WindowPlacementKey, Value: "next"}}); err != nil {
		t.Fatalf("set the task's beam key: %v", err)
	}
	primeName := h.Location.WorktreeName
	if err := fabricengine.WriteOrigin(nil, h.Location, primeName, fabriclayout.Origin{ParentWorktree: "task", ParentBranch: "task"}); err != nil {
		t.Fatalf("WriteOrigin(%q): %v", primeName, err)
	}
	mode := retileWindowMode(h.Location, primeName, "same")

	for worktree, want := range map[string]string{primeName: "same", "task": "next"} {
		if got, err := mode(worktree); err != nil || got != want {
			t.Errorf("mode(%q) = %q, %v; want %q", worktree, got, err, want)
		}
	}
}

func TestRetileVerb_RefusesOutsideAHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init")
	var out bytes.Buffer

	code := clihelp.ExecuteIn(Command(), dir, &out, []string{"retile"})

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	want := "retile: the working directory belongs to no hub; way forward: run it from a worktree of the hub whose windows are to be retiled"
	if code != 1 || envelope.OK || envelope.Error != want {
		t.Errorf("retile = exit %d, %q; want exit 1 and error %q", code, out.String(), want)
	}
}

// TestPlaceWindowChild runs a child whose placement cannot reach a desktop, for each kind of window.
// It is not parallel: PATH and the run mark of the environment are process-global.
func TestPlaceWindowChild(t *testing.T) {
	tiergate.Require(t, 1)

	lym := lymbin.Build(t)
	tools := map[string]string{}
	for _, name := range []string{"sh", "git", "sleep"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("no %s on PATH: %v", name, err)
		}
		tools[name] = path
	}
	// The command writes its pid to the marker, then waits for its release file to go.
	// Its own bound of three minutes outlasts every wait of this test.
	command := fmt.Sprintf("echo $$ > %s; i=0; while [ -e %s ] && [ $i -lt 1800 ]; do %s 0.1; i=$((i+1)); done",
		markerFile, releaseFile, tools["sleep"])
	windowKeys := []yamlengine.KV{
		{Key: "window_command", Value: command},
		{Key: "window_wait_ms", Value: "600000"},
	}

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	for _, slug := range []string{"task", "unmarked", "refused"} {
		hubforge.AddPair(t, h, slug)
	}
	setMachineDesktop(t, h, windowKeys...)
	refusedMachine := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	setMachineDesktop(t, refusedMachine, append(windowKeys, yamlengine.KV{Key: "window_placement", Value: "sideways"})...)
	refusedDirs := configengine.Dirs{Board: fabriclayout.BoltDir(h.Location.HubPath), Anchor: h.PairCodeWorktree("refused")}
	if _, err := configengine.Set(refusedDirs, dobbyengine.ConfigSpec(), configengine.LayerWorktree,
		[]yamlengine.KV{{Key: dobbyengine.WindowPlacementKey, Value: "sideways"}}); err != nil {
		t.Fatalf("set the task's beam key: %v", err)
	}

	window := exec.Command(tools["sleep"], "60")
	if err := window.Start(); err != nil {
		t.Fatalf("start the window's process: %v", err)
	}
	t.Cleanup(func() {
		_ = window.Process.Kill()
		_ = window.Wait()
	})

	bin := t.TempDir()
	for _, name := range []string{"sh", "git"} {
		if err := os.Symlink(tools[name], filepath.Join(bin, name)); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
	}
	t.Setenv("PATH", bin)

	tests := []struct {
		name     string
		hub      *hubforge.Hub
		worktree string
		prime    bool
		mark     string
		// held keeps the window command waiting while the run records are read.
		held        bool
		wantRecords int
		noMarker    bool
		wantLog     string
		// handed starts the child through handTerminalWindow, told the window's directory.
		handed bool
		// signalled sends the child SIGTERM while its window command is held.
		signalled bool
	}{
		{name: "TaskWindow", hub: h, worktree: "task", mark: pauserecord.RunProcessMarked, held: true, wantRecords: 1, handed: true, signalled: true},
		{name: "PrimeWindow", hub: h, worktree: h.Location.WorktreeName, prime: true, mark: pauserecord.RunProcessMarked, handed: true},
		{name: "UnmarkedTaskWindow", hub: h, worktree: "unmarked", held: true},
		{name: "RefusedTaskMode", hub: h, worktree: "refused", wantLog: "lym config beam --set window_placement="},
		{name: "RefusedMachineMode", hub: refusedMachine, worktree: refusedMachine.Location.WorktreeName, prime: true,
			noMarker: true, wantLog: "lym config desktop --layer machine --set window_placement="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(pauserecord.RunProcessEnv, tt.mark)
			hubPath := tt.hub.Location.HubPath
			dir := tt.hub.PairCodeWorktree(tt.worktree)
			release := filepath.Join(dir, releaseFile)
			if tt.held {
				if err := os.WriteFile(release, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(release) })
			}

			if tt.handed {
				if err := handTerminalWindow(lym, dir, "title", window.Process.Pid); err != nil {
					t.Fatalf("handTerminalWindow() = %v", err)
				}
			} else if err := kwinplace.Spawn(lym, hubPath, tt.worktree, "title", window.Process.Pid, tt.prime); err != nil {
				t.Fatalf("Spawn() = %v", err)
			}

			if !tt.noMarker {
				eventually(t, "the window command's marker", func() bool { return fileExists(filepath.Join(dir, markerFile)) })
			}
			eventually(t, "the settled entry", func() bool { return settled(hubPath, tt.worktree) })
			entry := kwinplace.ReadPlaces(kwinplace.PlacesPath(fabriclayout.BoltDir(hubPath)))[tt.worktree]
			if entry.Title != "title" || entry.PID != window.Process.Pid {
				t.Errorf("entry = %+v; want title %q and pid %d", entry, "title", window.Process.Pid)
			}
			if tt.wantLog != "" {
				eventually(t, "a warning naming "+tt.wantLog, func() bool { return logsHold(hubPath, "WARN", tt.wantLog) })
			}
			owners := []string{tt.worktree}
			if tt.prime {
				owners = append(owners, runproc.HubOwner)
			}
			for _, owner := range owners {
				if got := placementRecords(t, hubPath, owner); got != tt.wantRecords {
					t.Errorf("placement records of %s = %d; want %d", owner, got, tt.wantRecords)
				}
			}
			if tt.noMarker && fileExists(filepath.Join(dir, markerFile)) {
				t.Error("the window command ran; want its config refused")
			}
			if tt.signalled {
				var commandPID int
				eventually(t, "the window command's pid", func() bool {
					content, err := os.ReadFile(filepath.Join(dir, markerFile))
					// The newline marks the pid as written whole.
					if err != nil || !strings.HasSuffix(string(content), "\n") {
						return false
					}
					commandPID, err = strconv.Atoi(strings.TrimSpace(string(content)))
					return err == nil
				})
				records, err := runproc.List(fabriclayout.RunProcDir(hubPath), tt.worktree)
				if err != nil || len(records) != 1 {
					t.Fatalf("List(%s) = %+v, %v; want the child's record", tt.worktree, records, err)
				}
				// The child alone is signalled, so only its own handling of the signal can end the command.
				// The release file still holds the command.
				if err := syscall.Kill(records[0].PID, syscall.SIGTERM); err != nil {
					t.Fatalf("signal the child: %v", err)
				}
				eventually(t, "the window command's process to end", func() bool { return !proc.IsAlive(commandPID) })
			}
		})
	}
}
