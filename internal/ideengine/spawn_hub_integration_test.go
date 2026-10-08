//go:build integration

// spawn_hub_integration_test.go holds the Spawn checks that run against the prime and against a task pair of one hubforge hub:
// the prime opens a lyx-generated hub workspace, a task slug still opens its bare folder.
// Each check is a step of a scenario in spawn_scenario_integration_test.go, which names why they run serially.

package ideengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
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
	if err := os.RemoveAll(fabricengine.HubWorkspacePath(h.Location, prime)); err != nil {
		t.Fatalf("remove hub workspace: %v", err)
	}
}

// checkPrimeWritesHubWorkspace covers the workspace file's location, folders and launch argument at the hub's anchor.
func checkPrimeWritesHubWorkspace(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	launched := recordLauncher(t)

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	wsPath := fabricengine.HubWorkspacePath(l, prime)
	if len(*launched) != 1 || (*launched)[0] != wsPath {
		t.Fatalf("CodeLauncher calls = %v, want exactly [%s]", *launched, wsPath)
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
	wantNames := []string{prime, "_board", "_portals"}
	wantDirs := []string{
		filepath.Join(h.Path, prime, l.AnchorRel),
		fabricengine.BoardDir(h.Path),
		filepath.Join(h.Path, "_portals", l.AnchorRel),
	}
	if len(ws.Folders) != 3 {
		t.Fatalf("folders = %+v, want 3", ws.Folders)
	}
	for i, f := range ws.Folders {
		if f.Name != wantNames[i] {
			t.Errorf("folder %d name = %q, want %q", i, f.Name, wantNames[i])
		}
		got := filepath.Join(filepath.Dir(wsPath), filepath.FromSlash(f.Path))
		if got != wantDirs[i] {
			t.Errorf("folder %d resolves to %s, want %s", i, got, wantDirs[i])
		}
	}
	if info, err := os.Stat(wantDirs[2]); err != nil || !info.IsDir() {
		t.Errorf("_portals/<AnchorRel> missing: %v", err)
	}
}

// checkTaskSlugOpensBareFolder asserts a task spawn launches its bare folder and writes no workspace file.
func checkTaskSlugOpensBareFolder(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	want := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Fatalf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
	if _, err := os.Stat(fabricengine.HubWorkspacePath(l, primeOf(t, h))); !os.IsNotExist(err) {
		t.Errorf("workspace file exists after a task spawn (stat err = %v)", err)
	}
}

// checkPrimeSplicesSettingsAndRegenerates covers JSONC splice, idempotence, settings changes and hand-edit overwrite.
func checkPrimeSplicesSettingsAndRegenerates(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	recordLauncher(t)
	wsPath := fabricengine.HubWorkspacePath(l, prime)
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
	first, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if !strings.Contains(string(first), jsonc) {
		t.Fatalf("workspace file does not carry the JSONC settings verbatim:\n%s", first)
	}

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("unchanged Spawn: %v", err)
	}
	again, _ := os.ReadFile(wsPath)
	if string(again) != string(first) {
		t.Errorf("bytes changed on an unchanged spawn:\n%s", again)
	}

	changed := "{\n  \"editor.tabSize\": 8\n}"
	if err := os.WriteFile(settingsPath, []byte(changed), 0o644); err != nil {
		t.Fatalf("rewrite settings: %v", err)
	}
	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn after settings change: %v", err)
	}
	got, _ := os.ReadFile(wsPath)
	if !strings.Contains(string(got), changed) || strings.Contains(string(got), "keep me") {
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
	if err == nil || !strings.Contains(err.Error(), "read prime settings") {
		t.Fatalf("Spawn error = %v, want the read cause", err)
	}
	if len(*launched) != 0 {
		t.Errorf("CodeLauncher called: %v", *launched)
	}
	if _, err := os.Stat(fabricengine.HubWorkspacePath(l, prime)); !os.IsNotExist(err) {
		t.Errorf("workspace file written despite the failure (stat err = %v)", err)
	}
}

// checkPrimeWorkspaceSurvivesTopologyVerbs asserts Remove and a Prune dry run leave the workspace file alone.
func checkPrimeWorkspaceSurvivesTopologyVerbs(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	recordLauncher(t)
	hubforge.AddPair(t, h, slug)

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	wsPath := fabricengine.HubWorkspacePath(l, prime)

	if _, err := h.Topology.Remove(l, slug, true, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(wsPath); err != nil {
		t.Fatalf("workspace file gone after Remove: %v", err)
	}

	res, err := h.Topology.Prune(l, false, false)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, e := range res.Entries {
		if strings.Contains(e.CodeWorktree, ".code-workspace") || strings.Contains(e.RecordsWorktree, ".code-workspace") {
			t.Errorf("Prune entry names the workspace file: %+v", e)
		}
	}
	if _, err := os.Stat(wsPath); err != nil {
		t.Fatalf("workspace file gone after Prune: %v", err)
	}
}
