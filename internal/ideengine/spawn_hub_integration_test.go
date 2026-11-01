//go:build tier1

// spawn_hub_integration_test.go holds the Spawn and OpenTask checks that run against the prime and against a task pair of one hubforge hub:
// the prime opens the lym-generated hub workspace, a task slug still opens its bare folder, and the window command follows a launch.
// Each check is a step of a scenario in spawn_scenario_integration_test.go, which names why they run serially.

package ideengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// recordLauncher swaps CodeLauncher for a recorder and returns the launched paths, restoring the seam on cleanup.
func recordLauncher(t *testing.T) *[]string {
	t.Helper()
	original := CodeLauncher
	t.Cleanup(func() { CodeLauncher = original })
	var launched []string
	CodeLauncher = func(path string) error {
		launched = append(launched, path)
		return nil
	}
	return &launched
}

// primeOf returns the hub's prime worktree name.
func primeOf(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	name, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("PrimeName: %v", err)
	}
	return name
}

// primeSettingsPath returns the prime's .vscode/settings.json.
func primeSettingsPath(h *hubforge.Hub, prime string) string {
	return filepath.Join(h.Path, prime, h.Location.AnchorRel, ".vscode", "settings.json")
}

// resetPrimeEditorState removes the prime's untracked .vscode directory and the hub workspace file, returning the prime to the state of a freshly built hub for a step that depends on it.
func resetPrimeEditorState(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	prime := primeOf(t, h)
	if err := os.RemoveAll(filepath.Dir(primeSettingsPath(h, prime))); err != nil {
		t.Fatalf("remove prime .vscode: %v", err)
	}
	if err := os.RemoveAll(fabricengine.HubWorkspacePath(h.Location)); err != nil {
		t.Fatalf("remove hub workspace: %v", err)
	}
}

// checkPrimeWritesHubWorkspace covers the workspace file's location, folders and launch argument at the hub's anchor:
// the prime's branch, Bolt and every task pair the scenario added so far, and no _portals folder.
func checkPrimeWritesHubWorkspace(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	launched := recordLauncher(t)
	markerPath := setDesktopWindowCommand(t, h)

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	wsPath := fabricengine.HubWorkspacePath(l)
	if len(*launched) != 1 || (*launched)[0] != wsPath {
		t.Fatalf("CodeLauncher calls = %v, want exactly [%s]", *launched, wsPath)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Errorf("window command runs after a prime Spawn = %d; want 1, since the command runs in the hub directory beside the workspace file", runs)
	}
	data, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	var ws struct {
		Folders []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("workspace file is not valid JSON: %v\n%s", err, data)
	}
	primeBranch := gitkit.Git(t, filepath.Join(h.Path, prime), "branch", "--show-current")
	wantNames := []string{primeBranch, "Bolt"}
	wantDirs := []string{
		filepath.Join(h.Path, prime, l.AnchorRel),
		fabricengine.BoardDir(h.Path),
	}
	entries, err := fabricengine.List(l.AnchorPath())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var pairNames []string
	for _, entry := range entries {
		if !entry.Main && !entry.Prunable {
			pairNames = append(pairNames, filepath.Base(filepath.FromSlash(entry.Path)))
		}
	}
	sort.Strings(pairNames)
	for _, name := range pairNames {
		wantNames = append(wantNames, name)
		wantDirs = append(wantDirs, filepath.Join(h.Path, name, l.AnchorRel))
	}
	if len(ws.Folders) != len(wantNames) {
		t.Fatalf("folders = %+v, want names %v", ws.Folders, wantNames)
	}
	for i, f := range ws.Folders {
		if f.Name != wantNames[i] {
			t.Errorf("folder %d name = %q, want %q", i, f.Name, wantNames[i])
		}
		got := filepath.Join(filepath.Dir(wsPath), filepath.FromSlash(f.Path))
		if got != wantDirs[i] {
			t.Errorf("folder %d resolves to %s, want %s", i, got, wantDirs[i])
		}
		if strings.Contains(f.Path, "_portals") {
			t.Errorf("folder %d path %q names _portals", i, f.Path)
		}
	}
}

// readOptionalFile returns the bytes of path and whether it exists.
func readOptionalFile(t *testing.T, path string) (data []byte, exists bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data, true
}

// checkTaskSlugOpensBareFolder asserts a task spawn launches its bare folder, and that a task seed and spawn leave the workspace file's bytes, or its absence, unchanged.
// It also asserts Seed writes the same on-demand attach task Spawn does and seeds the keybindings.json as Spawn does, each into a missing file.
// It also asserts Spawn, and never Seed, runs the machine's desktop window command.
func checkTaskSlugOpensBareFolder(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	keybindingsPath, err := KeybindingsPath()
	if err != nil {
		t.Fatalf("KeybindingsPath: %v", err)
	}
	if err := os.RemoveAll(keybindingsPath); err != nil {
		t.Fatalf("remove keybindings: %v", err)
	}
	markerPath := setDesktopWindowCommand(t, h)
	workspaceBefore, workspaceExisted := readOptionalFile(t, fabricengine.HubWorkspacePath(l))

	keybindings, err := Seed(l, slug)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	assertAttachTaskOnly(t, filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel))
	if keybindings.Outcome != vscode.KeybindingsCreated {
		t.Errorf("Seed keybindings outcome = %+v; want %q", keybindings, vscode.KeybindingsCreated)
	}
	if _, err := os.Stat(keybindingsPath); err != nil {
		t.Errorf("keybindings.json missing after Seed: %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Errorf("window command marker after Seed: stat error %v; want it absent, since Seed opens no window", err)
	}
	if err := os.RemoveAll(keybindingsPath); err != nil {
		t.Fatalf("remove keybindings: %v", err)
	}

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := os.Stat(keybindingsPath); err != nil {
		t.Errorf("keybindings.json missing after Spawn: %v", err)
	}

	want := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Fatalf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
	if marker, err := os.ReadFile(markerPath); err != nil || strings.TrimSpace(string(marker)) != "ran" {
		t.Errorf("window command marker = %q, %v; want one run of the window command", marker, err)
	}
	workspaceAfter, workspaceExists := readOptionalFile(t, fabricengine.HubWorkspacePath(l))
	if workspaceExists != workspaceExisted || !bytes.Equal(workspaceAfter, workspaceBefore) {
		t.Errorf("workspace file changed by a task seed and spawn: existed %v then, %v now; bytes before %q, after %q", workspaceExisted, workspaceExists, workspaceBefore, workspaceAfter)
	}
}

// checkWindowCommandFollowsALaunch asserts OpenTask runs the machine's window command once after a launch,
// and that a failing CodeLauncher makes Spawn and OpenTask return its error and run no command.
func checkWindowCommandFollowsALaunch(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	hubforge.AddPair(t, h, slug)
	markerPath := setDesktopWindowCommand(t, h)

	launched := recordLauncher(t)
	if err := OpenTask(l, slug); err != nil {
		t.Fatalf("OpenTask: %v", err)
	}
	want := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Fatalf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Fatalf("window command runs after OpenTask = %d; want 1", runs)
	}

	launchErr := errors.New("code is not installed")
	CodeLauncher = func(string) error { return launchErr }
	if err := OpenTask(l, slug); !errors.Is(err, launchErr) {
		t.Errorf("OpenTask error = %v; want the launcher's error", err)
	}
	if _, err := Spawn(l, slug); !errors.Is(err, launchErr) {
		t.Errorf("Spawn error = %v; want the launcher's error", err)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Errorf("window command runs after failed launches = %d; want still 1, since a failed launch opens no window", runs)
	}
}

// windowCommandRuns returns how many times the window command appended to the marker at markerPath.
func windowCommandRuns(t *testing.T, markerPath string) int {
	t.Helper()
	data, exists := readOptionalFile(t, markerPath)
	if !exists {
		return 0
	}
	return len(strings.Fields(string(data)))
}

// checkPrimeSeedWritesHubWorkspace asserts a seed of the prime writes the hub workspace file with the workspace title and launches nothing.
func checkPrimeSeedWritesHubWorkspace(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched := recordLauncher(t)

	if _, err := Seed(l, primeOf(t, h)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	if len(*launched) != 0 {
		t.Errorf("CodeLauncher calls = %v, want none", *launched)
	}
	want := "TST: " + l.RepoName + " workspace"
	if got := readWorkspaceLook(t, fabricengine.HubWorkspacePath(l)).Title; got != want {
		t.Errorf("hub workspace window.title = %q, want %q", got, want)
	}
}

// checkPrimeSplicesSettingsAndRegenerates covers the splice of JSONC settings with the workspace look, idempotence, settings changes and hand-edit overwrite.
func checkPrimeSplicesSettingsAndRegenerates(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	recordLauncher(t)
	wsPath := fabricengine.HubWorkspacePath(l)
	settingsPath := primeSettingsPath(h, prime)

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("first Spawn: %v", err)
	}
	jsonc := "{\n  // keep me\n  \"editor.tabSize\": 2,\n}"
	if err := os.WriteFile(settingsPath, []byte(jsonc), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn with JSONC settings: %v", err)
	}
	settings := readPrimeLook(t, settingsPath)
	if settings.TabSize != 2 || settings.Title != "TST: "+prime || settings.Colors["titleBar.activeBackground"] != vscode.PrimeColor || settings.Colors["activityBar.background"] != vscode.PrimeColor {
		t.Errorf("prime settings after Spawn = %+v; want tabSize 2 kept, title %q and green on both bars", settings, "TST: "+prime)
	}
	first, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	workspaceTitle := "TST: " + l.RepoName + " workspace"
	if ws := readWorkspaceLook(t, wsPath); ws.TabSize != 2 || ws.Title != workspaceTitle || ws.Colors["titleBar.activeBackground"] != vscode.PrimeColor || ws.Colors["activityBar.background"] != vscode.WorkspaceColor {
		t.Fatalf("workspace settings = %+v; want the prime's tabSize 2, title %q, green on the title bar and %s on the activity bar", ws, workspaceTitle, vscode.WorkspaceColor)
	}

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("unchanged Spawn: %v", err)
	}
	again, _ := os.ReadFile(wsPath)
	if string(again) != string(first) {
		t.Errorf("bytes changed on an unchanged spawn:\n%s", again)
	}

	if err := os.WriteFile(settingsPath, []byte("{\n  \"editor.tabSize\": 8\n}"), 0o644); err != nil {
		t.Fatalf("rewrite settings: %v", err)
	}
	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn after settings change: %v", err)
	}
	changed := readPrimeLook(t, settingsPath)
	got, _ := os.ReadFile(wsPath)
	if changed.TabSize != 8 || readWorkspaceLook(t, wsPath).TabSize != 8 || strings.Contains(string(got), "keep me") {
		t.Errorf("changed settings not reflected:\n%s", got)
	}

	if err := os.WriteFile(wsPath, []byte("hand edit"), 0o644); err != nil {
		t.Fatalf("hand edit: %v", err)
	}
	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn after hand edit: %v", err)
	}
	restored, _ := os.ReadFile(wsPath)
	if string(restored) != string(got) {
		t.Errorf("hand edit not overwritten:\n%s", restored)
	}
}

// checkPrimeSettingsReadFailure plants a directory at the prime's settings.json and asserts the spawn fails.
func checkPrimeSettingsReadFailure(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	launched := recordLauncher(t)
	if err := os.MkdirAll(primeSettingsPath(h, prime), 0o755); err != nil {
		t.Fatalf("plant directory: %v", err)
	}

	_, err := Spawn(l, prime)
	if err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Fatalf("Spawn error = %v, want the read cause", err)
	}
	if len(*launched) != 0 {
		t.Errorf("CodeLauncher called: %v", *launched)
	}
	if _, err := os.Stat(fabricengine.HubWorkspacePath(l)); !os.IsNotExist(err) {
		t.Errorf("workspace file written despite the failure (stat err = %v)", err)
	}
}

// checkPrimeWriteFailure plants a read-only workspace file of different bytes and asserts the prime spawn and the prime seed return the write error, the spawn launches nothing and both leave the file's bytes.
// It skips as root, where mode bits refuse nothing.
func checkPrimeWriteFailure(t *testing.T, h *hubforge.Hub) {
	if os.Geteuid() == 0 {
		t.Skip("mode bits refuse nothing as root")
	}
	resetPrimeEditorState(t, h)
	l := h.Location
	launched := recordLauncher(t)
	wsPath := fabricengine.HubWorkspacePath(l)
	planted := []byte("planted workspace bytes\n")
	if err := os.WriteFile(wsPath, planted, 0o444); err != nil {
		t.Fatalf("plant workspace file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(wsPath) })

	_, err := Spawn(l, primeOf(t, h))
	if err == nil || !strings.Contains(err.Error(), "write workspace file") {
		t.Fatalf("Spawn error = %v, want the write cause", err)
	}
	if len(*launched) != 0 {
		t.Errorf("CodeLauncher called: %v", *launched)
	}
	got, readErr := os.ReadFile(wsPath)
	if readErr != nil || !bytes.Equal(got, planted) {
		t.Errorf("workspace file = %q (read err %v), want the planted bytes", got, readErr)
	}

	_, err = Seed(l, primeOf(t, h))
	if err == nil || !strings.Contains(err.Error(), "write workspace file") {
		t.Fatalf("Seed error = %v, want the write cause", err)
	}
	got, readErr = os.ReadFile(wsPath)
	if readErr != nil || !bytes.Equal(got, planted) {
		t.Errorf("workspace file after Seed = %q (read err %v), want the planted bytes", got, readErr)
	}
}

// primeLook is the part of a seeded settings.json, or of a workspace file's settings, the prime checks read.
type primeLook struct {
	TabSize int               `json:"editor.tabSize"`
	Title   string            `json:"window.title"`
	Colors  map[string]string `json:"workbench.colorCustomizations"`
}

// readWorkspaceLook reads the workspace file at path and decodes the settings it carries.
func readWorkspaceLook(t *testing.T, path string) primeLook {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	var ws struct {
		Settings primeLook `json:"settings"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("decode workspace: %v\n%s", err, data)
	}
	return ws.Settings
}

// readPrimeLook reads and decodes the settings.json at path.
func readPrimeLook(t *testing.T, path string) primeLook {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var look primeLook
	if err := json.Unmarshal(data, &look); err != nil {
		t.Fatalf("decode settings: %v\n%s", err, data)
	}
	return look
}

// setDesktopWindowCommand writes h's desktop machine override with a window command that appends "ran" to a marker file, in a form sh and cmd both run, and no wait.
// It returns the marker's path and removes the override on cleanup.
func setDesktopWindowCommand(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	markerPath := filepath.Join(t.TempDir(), "marker")
	board := fabricengine.BoltDir(h.Location.HubPath)
	if err := os.MkdirAll(configengine.ScratchDir(board), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", configengine.ScratchDir(board), err)
	}
	machinePath := configengine.MachineFile(board, "desktop")
	contents := "window_command: 'echo ran>> \"" + markerPath + "\"'\nwindow_wait_ms: 0\n"
	if err := os.WriteFile(machinePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", machinePath, err)
	}
	t.Cleanup(func() {
		if err := os.Remove(machinePath); err != nil {
			t.Errorf("remove %s: %v", machinePath, err)
		}
	})
	return markerPath
}
