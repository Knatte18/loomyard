// spawn_hub_integration_test.go holds the Spawn and OpenTask checks of one hubforge hub.
// They run against the prime and against a task pair.
// The prime opens the lym-generated hub workspace, and a beam opens its own workspace.
// A task slug and a beam with no readable origin record open their bare folder.
// The window command follows a launch.
// Each check is a step of a scenario in spawn_scenario_integration_test.go, which names why they run serially.

package ideengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// recordLauncher swaps CodeLauncher for a recorder and returns the launched paths, restoring the seam on cleanup.
func recordLauncher(t *testing.T) *[]string {
	t.Helper()
	launched, _ := recordLaunches(t)
	return launched
}

// recordLaunches swaps CodeLauncher for a recorder, restoring the seam on cleanup.
// It returns the launched paths and, for each launch, the files opened beside the path.
func recordLaunches(t *testing.T) (launched *[]string, opened *[][]string) {
	t.Helper()
	original := CodeLauncher
	t.Cleanup(func() { CodeLauncher = original })
	var paths []string
	var files [][]string
	CodeLauncher = func(path string, openFiles ...string) error {
		paths = append(paths, path)
		files = append(files, openFiles)
		return nil
	}
	return &paths, &files
}

// setIDEConfig writes h's hub ide.yaml with contents and removes it on cleanup.
func setIDEConfig(t *testing.T, h *hubforge.Hub, contents string) {
	t.Helper()
	board := fabricengine.BoltDir(h.Location.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", configengine.ConfigDir(board), err)
	}
	configPath := configengine.ConfigFile(board, "ide")
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, err)
	}
	t.Cleanup(func() {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove %s: %v", configPath, err)
		}
	})
}

// setLocalIDEConfig writes the local override of the ide module under anchor, and removes it on cleanup.
// That is where a worktree's local config layer is read from.
func setLocalIDEConfig(t *testing.T, anchor, contents string) {
	t.Helper()
	for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	localPath := configengine.LocalFile(anchor, "ide")
	if err := os.WriteFile(localPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", localPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(localPath) })
}

// writeFileUnder writes a small file at rel under dir, creating its directory, and removes the file on cleanup.
func writeFileUnder(t *testing.T, dir, rel string) string {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte("# "+rel+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	t.Cleanup(func() { _ = os.Remove(full) })
	return full
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

// resetPrimeEditorState removes the prime's untracked .vscode directory and the hub workspace file.
// The prime is then in the state of a freshly built hub, for a step that depends on it.
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

// checkPrimeWritesHubWorkspace asserts the hub workspace file's location, folders and launch argument.
// It sits at the hub's anchor and lists the prime's branch, Bolt and every task pair the scenario added so far.
// It has no _portals folder.
// The launch opens beside the workspace, in order, the existing files the hub's ide.yaml lists for the prime.
// {beam} resolves to the prime's name, and an absent file is skipped.
func checkPrimeWritesHubWorkspace(t *testing.T, h *hubforge.Hub) {
	resetPrimeEditorState(t, h)
	l := h.Location
	prime := primeOf(t, h)
	launched, opened := recordLaunches(t)
	markerPath := setDesktopWindowCommand(t, h)
	setIDEConfig(t, h, "open_files:\n  prime: [notes.md, absent.md, \"{beam}.md\"]\n")
	primeAnchor := filepath.Join(h.Path, prime, l.AnchorRel)
	wantFiles := []string{writeFileUnder(t, primeAnchor, "notes.md"), writeFileUnder(t, primeAnchor, prime+".md")}

	if _, err := Spawn(l, prime); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	wsPath := fabricengine.HubWorkspacePath(l)
	if len(*launched) != 1 || (*launched)[0] != wsPath {
		t.Fatalf("CodeLauncher calls = %v, want exactly [%s]", *launched, wsPath)
	}
	if files := (*opened)[0]; !slices.Equal(files, wantFiles) {
		t.Errorf("CodeLauncher files = %v; want %v", files, wantFiles)
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

// checkTaskSlugOpensBareFolder asserts a task spawn launches its bare folder.
// It also asserts a task seed and a task spawn leave the workspace file's bytes, or its absence, unchanged.
// It also asserts Seed and Spawn both write the on-demand attach task and seed keybindings.json into missing files.
// It also asserts Spawn, and never Seed, runs the machine's desktop window command.
// An ide.yaml that fails to load fails Seed, Spawn and OpenTask, with nothing launched and no settings.json written.
// A valid ide.yaml opens the folder beside the existing files it lists for a task.
// A directory entry is skipped, and {beam} resolves to the prime for a task forked from it.
func checkTaskSlugOpensBareFolder(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched, opened := recordLaunches(t)
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
	taskAnchor := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)

	setIDEConfig(t, h, "open_files:\n  task: [\"/absolute.md\"]\n")
	verbs := []struct {
		name string
		run  func() error
	}{
		{"Seed", func() error { _, err := Seed(l, slug); return err }},
		{"Spawn", func() error { _, err := Spawn(l, slug); return err }},
		{"OpenTask", func() error { return OpenTask(l, slug) }},
	}
	for _, verb := range verbs {
		if err := verb.run(); err == nil || !strings.Contains(err.Error(), "load the ide config") {
			t.Errorf("%s error = %v; want the ide.yaml load failure", verb.name, err)
		}
	}
	if len(*launched) != 0 {
		t.Errorf("CodeLauncher calls with an invalid ide.yaml = %v; want none", *launched)
	}
	if _, err := os.Stat(filepath.Join(taskAnchor, ".vscode", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json after an invalid ide.yaml: stat error %v; want it absent, since the load runs before any write", err)
	}

	setIDEConfig(t, h, "open_files:\n  task: [\"{beam}.md\", docs, absent.md]\n")
	writeFileUnder(t, taskAnchor, "docs/guide.md")
	wantFiles := []string{writeFileUnder(t, taskAnchor, primeOf(t, h)+".md")}

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
	if files := (*opened)[0]; !slices.Equal(files, wantFiles) {
		t.Errorf("CodeLauncher files = %v; want %v", files, wantFiles)
	}
	if marker, err := os.ReadFile(markerPath); err != nil || strings.TrimSpace(string(marker)) != "ran" {
		t.Errorf("window command marker = %q, %v; want one run of the window command", marker, err)
	}
	workspaceAfter, workspaceExists := readOptionalFile(t, fabricengine.HubWorkspacePath(l))
	if workspaceExists != workspaceExisted || !bytes.Equal(workspaceAfter, workspaceBefore) {
		t.Errorf("workspace file changed by a task seed and spawn: existed %v then, %v now; bytes before %q, after %q", workspaceExisted, workspaceExists, workspaceBefore, workspaceAfter)
	}
}

// checkBeamOpensBeamWorkspace asserts that a beam seed regenerates the beam's workspace file with its workspace title,
// and that it launches nothing.
// A beam spawn regenerates that file, launches it with the beam as its first folder, and runs the window command once.
// The spawn opens beside it, in order, the existing files the hub's ide.yaml lists for a beam.
// {beam} resolves to the beam's slug, and an absent file and a directory are skipped.
// The workspace's settings open a markdown file rendered.
// A task forked from the beam opens, through OpenTask and Spawn, the file ide.yaml lists for a task;
// in that list, {beam} resolves to the beam's slug.
func checkBeamOpensBeamWorkspace(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched, opened := recordLaunches(t)
	hubforge.AddPair(t, h, slug)
	if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Path), l, slug); err != nil || !marked {
		t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
	}
	markerPath := setDesktopWindowCommand(t, h)
	beamPath := fabricengine.BeamWorkspacePath(l, slug)
	setIDEConfig(t, h, "open_files:\n  beam: [README.md, absent.md, docs, \"docs/{beam}.md\"]\n  task: [\"{beam}.md\"]\n")
	beamAnchor := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)
	wantFiles := []string{writeFileUnder(t, beamAnchor, "README.md"), writeFileUnder(t, beamAnchor, "docs/"+slug+".md")}

	if _, err := Seed(l, slug); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(*launched) != 0 {
		t.Errorf("CodeLauncher calls after Seed = %v, want none", *launched)
	}
	if want, got := "[tst] "+slug+" workspace", readWorkspaceLook(t, beamPath).Title; got != want {
		t.Errorf("beam workspace window.title = %q, want %q", got, want)
	}
	if err := os.Remove(beamPath); err != nil {
		t.Fatalf("remove beam workspace: %v", err)
	}

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if len(*launched) != 1 || (*launched)[0] != beamPath {
		t.Fatalf("CodeLauncher calls = %v, want exactly [%s]", *launched, beamPath)
	}
	if files := (*opened)[0]; !slices.Equal(files, wantFiles) {
		t.Errorf("CodeLauncher files = %v; want %v", files, wantFiles)
	}
	if got := readWorkspaceLook(t, beamPath).Associations["*.md"]; got != "vscode.markdown.preview.editor" {
		t.Errorf("beam workspace *.md editor association = %q; want the markdown preview", got)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Errorf("window command runs after a beam Spawn = %d; want 1", runs)
	}
	data, err := os.ReadFile(beamPath)
	if err != nil {
		t.Fatalf("read beam workspace: %v", err)
	}
	var ws struct {
		Folders []struct {
			Name string `json:"name"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("beam workspace is not valid JSON: %v\n%s", err, data)
	}
	if len(ws.Folders) == 0 || ws.Folders[0].Name != slug {
		t.Errorf("beam workspace folders = %+v, want %q first", ws.Folders, slug)
	}

	child := slug + "-task"
	hubforge.AddPairWith(t, h, child, fabricengine.AddOptions{Parent: slug})
	// The forked task is removed on cleanup, since the hub workspace check of a later step lists every pair of the hub.
	t.Cleanup(func() {
		if _, err := h.Topology.Remove(l, child, true, false); err != nil {
			t.Errorf("remove the task forked from the beam: %v", err)
		}
	})
	childAnchor := filepath.Join(fabricengine.WorktreePath(l, child), l.AnchorRel)
	wantChildFiles := []string{writeFileUnder(t, childAnchor, slug+".md")}
	_, childOpened := recordLaunches(t)
	if err := OpenTask(l, child); err != nil {
		t.Fatalf("OpenTask of a task forked from the beam: %v", err)
	}
	if _, err := Spawn(l, child); err != nil {
		t.Fatalf("Spawn of a task forked from the beam: %v", err)
	}
	if len(*childOpened) != 2 || !slices.Equal((*childOpened)[0], wantChildFiles) || !slices.Equal((*childOpened)[1], wantChildFiles) {
		t.Errorf("OpenTask and Spawn files of a task forked from %q = %v; want %v each", slug, *childOpened, wantChildFiles)
	}
}

// checkBeamWithoutReadableOriginOpensBareFolder asserts that Spawn returns no error,
// and that it launches the pair's bare anchor folder and no workspace file.
// It covers a marked beam whose origin record cannot be read: its records worktree's lock directory is missing.
// It also covers a marked beam whose record is absent.
// It also asserts a {beam} entry of ide.yaml's task list opens nothing for such a pair.
// Its parent cannot be read, so the entry opens nothing, even beside a file named ".md".
// It restores the lock directory before returning, so later steps regenerate the hub workspace over a readable pair.
func checkBeamWithoutReadableOriginOpensBareFolder(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	launched, opened := recordLaunches(t)
	hubforge.AddPair(t, h, slug)
	if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Path), l, slug); err != nil || !marked {
		t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
	}
	setDesktopWindowCommand(t, h)
	setIDEConfig(t, h, "open_files:\n  task: [\"{beam}.md\"]\n")
	// The entry reads as ".md" when {beam} is replaced by nothing, so this file is planted under that name.
	writeFileUnder(t, filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel), ".md")
	pair, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		t.Fatalf("NamedWorktree(%q): %v", slug, err)
	}
	lockDir := fabricengine.RecordsLockDirPath(fabricengine.RecordsWorktreePath(pair, slug))
	bareFolder := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)

	if _, err := os.Stat(lockDir); err != nil {
		t.Fatalf("records lock directory %s missing before the check removes it: %v", lockDir, err)
	}
	if err := os.RemoveAll(lockDir); err != nil {
		t.Fatalf("remove records lock directory: %v", err)
	}
	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn with a missing records lock directory: %v", err)
	}
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("restore records lock directory: %v", err)
	}

	if err := os.Remove(fabricengine.OriginRecordPathFor(pair, slug)); err != nil {
		t.Fatalf("remove origin record: %v", err)
	}
	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn with an absent origin record: %v", err)
	}

	if want := []string{bareFolder, bareFolder}; !slices.Equal(*launched, want) {
		t.Errorf("CodeLauncher calls = %v, want %v", *launched, want)
	}
	for i, files := range *opened {
		if len(files) != 0 {
			t.Errorf("CodeLauncher files of launch %d = %v; want none, since no beam resolves the {beam} entry", i, files)
		}
	}
	if _, err := os.Stat(fabricengine.BeamWorkspacePath(l, slug)); !os.IsNotExist(err) {
		t.Errorf("beam workspace file written for a beam with no readable origin record (stat err = %v)", err)
	}
}

// checkWindowCommandFollowsALaunch asserts OpenTask runs the machine's window command once after a launch,
// and that a failing CodeLauncher makes Spawn and OpenTask return its error and run no command.
// It also asserts OpenTask passes the launcher the files ide.yaml lists for a task,
// and that {beam} resolves to the prime for a task forked from it.
// It also asserts the task worktree's own local override of ide.yaml applies,
// and that it still applies when OpenTask runs from another worktree.
func checkWindowCommandFollowsALaunch(t *testing.T, h *hubforge.Hub, slug string) {
	resetPrimeEditorState(t, h)
	l := h.Location
	hubforge.AddPair(t, h, slug)
	markerPath := setDesktopWindowCommand(t, h)
	setIDEConfig(t, h, "open_files:\n  task: [\"{beam}.md\"]\n")
	slugAnchor := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel)
	wantSlugFiles := []string{writeFileUnder(t, slugAnchor, primeOf(t, h)+".md")}

	launched, opened := recordLaunches(t)
	if err := OpenTask(l, slug); err != nil {
		t.Fatalf("OpenTask: %v", err)
	}
	want := slugAnchor
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Fatalf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
	if files := (*opened)[0]; !slices.Equal(files, wantSlugFiles) {
		t.Errorf("OpenTask files = %v; want %v", files, wantSlugFiles)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Fatalf("window command runs after OpenTask = %d; want 1", runs)
	}

	launchErr := errors.New("code is not installed")
	CodeLauncher = func(string, ...string) error { return launchErr }
	if err := OpenTask(l, slug); !errors.Is(err, launchErr) {
		t.Errorf("OpenTask error = %v; want the launcher's error", err)
	}
	if _, err := Spawn(l, slug); !errors.Is(err, launchErr) {
		t.Errorf("Spawn error = %v; want the launcher's error", err)
	}
	if runs := windowCommandRuns(t, markerPath); runs != 1 {
		t.Errorf("window command runs after failed launches = %d; want still 1, since a failed launch opens no window", runs)
	}

	localFile := writeFileUnder(t, slugAnchor, "local.md")
	setLocalIDEConfig(t, slugAnchor, "open_files:\n  task: [local.md]\n")
	_, localOpened := recordLaunches(t)
	if err := OpenTask(l, slug); err != nil {
		t.Fatalf("OpenTask with a local override: %v", err)
	}
	if want := []string{localFile}; len(*localOpened) != 1 || !slices.Equal((*localOpened)[0], want) {
		t.Errorf("OpenTask files with a local override = %v; want [%v]", *localOpened, want)
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

// checkPrimeSeedWritesHubWorkspace asserts a seed of the prime writes the hub workspace file with the workspace title,
// and that the seed launches nothing.
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
	want := "[tst] " + l.RepoName + " workspace"
	if got := readWorkspaceLook(t, fabricengine.HubWorkspacePath(l)).Title; got != want {
		t.Errorf("hub workspace window.title = %q, want %q", got, want)
	}
}

// checkPrimeSplicesSettingsAndRegenerates covers the splice of JSONC settings with the workspace look.
// It also covers idempotence, settings changes and hand-edit overwrite.
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
	if settings.TabSize != 2 || settings.Title != "ORCH [tst:P0] root" || settings.Colors["titleBar.activeBackground"] != vscode.PrimeColor || settings.Colors["activityBar.background"] != vscode.PrimeColor {
		t.Errorf("prime settings after Spawn = %+v; want tabSize 2 kept, title %q and green on both bars", settings, "ORCH [tst:P0] root")
	}
	first, err := os.ReadFile(wsPath)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	workspaceTitle := "[tst] " + l.RepoName + " workspace"
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

// checkPrimeWriteFailure plants a read-only workspace file of different bytes.
// It asserts the prime spawn and the prime seed return the write error,
// and that the spawn launches nothing and both leave the file's bytes as they were.
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
	TabSize      int               `json:"editor.tabSize"`
	Title        string            `json:"window.title"`
	Colors       map[string]string `json:"workbench.colorCustomizations"`
	Associations map[string]string `json:"workbench.editorAssociations"`
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

// setDesktopWindowCommand writes h's desktop machine override with a window command.
// The command appends "ran" to a marker file in a form that sh and cmd both run, and it does not wait.
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
