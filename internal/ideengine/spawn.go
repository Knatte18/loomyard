// spawn.go implements the ide entry point Spawn (`ide spawn`), and its two halves, Seed and OpenTask.
// It resolves a worktree's window title and colours from the prime, the board's beams and the worktree's parent, generates the worktree's .vscode/ config, and launches VS Code with the files the hub's ide.yaml lists for the worktree's place;
// after a launch it runs the machine's desktop window command through internal/desktophook;
// it regenerates tasks.json on every call and keeps .vscode/ out of git through info/exclude.
// A task slug opens its bare folder.
// The prime instead opens the lym-generated hub workspace <hub>/<RepoName>.code-workspace (prime, Bolt, every task pair), regenerated on every prime spawn or seed,
// and a beam opens its own lym-generated workspace <hub>/<RepoName>-<beam>.code-workspace, regenerated on every beam spawn or seed.

package ideengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/ideconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// CodeLauncher is the exported package-level injectable seam that can be overridden in tests.
// It defaults to vscode.Launch, told the folder or workspace file to open and the files to open beside it, and can be stubbed to record its arguments for testing.
// Exported so that cli_test.go in the idecli package can swap it.
var CodeLauncher = vscode.Launch

// KeybindingsPath is the injectable seam that names the user's VS Code keybindings.json.
// It defaults to vscode.UserKeybindingsPath and is exported so that tests in other packages can point it at a temporary file.
var KeybindingsPath = vscode.UserKeybindingsPath

// Spawn writes the window title and colours and the .vscode/ config of the worktree slug and the user's lym key bindings, then launches VS Code and runs the machine's desktop window command.
// A task slug launches its bare folder <hub>/<slug>/<its own anchor>.
// When slug names the prime, Spawn first regenerates the hub workspace file <hub>/<RepoName>.code-workspace and launches that file instead;
// when slug names a beam, one whose origin record carries the beam mark, it regenerates the same way and launches the beam's workspace file <hub>/<RepoName>-<beam>.code-workspace.
// The launch also opens the files ide.yaml lists for the worktree's place, the prime, a beam or a task, that exist under the worktree's anchored directory.
// Every error on either path is returned, never degraded to the bare folder, and so is an ide.yaml that fails to load.
// A failure to resolve the prime's name, or to read a pair's origin record, is logged and degrades to the bare-folder path and a task's files.
// It returns the keybindings seed's outcome beside the error; a skipped keybindings seed never fails the spawn.
func Spawn(l *lymcwd.Location, slug string) (vscode.KeybindingsResult, error) {
	result, err := seed(l, slug)
	if err != nil {
		return result.keybindings, err
	}
	if err := CodeLauncher(result.launchPath, result.openFiles...); err != nil {
		return result.keybindings, err
	}
	placeWindow(l, result.windowDir)
	return result.keybindings, nil
}

// Seed writes the window title and colours and the .vscode/ config of the worktree slug, the prime or a pair, and launches nothing.
// It keeps .vscode/ out of git through info/exclude and never writes a .gitignore;
// a tracked .vscode/tasks.json is left alone with a warning.
// The settings make a markdown file open rendered while ide.yaml's render_markdown is on, and an ide.yaml that fails to load fails the seed.
// When slug names the prime or a beam, Seed then regenerates the hub workspace file and every beam's, whose settings carry the prime's or the beam's with the workspace look, and returns any error on that path;
// a task slug leaves the workspace files as they are.
// It seeds the lym block of key bindings, the Alt-arrow keys the integrated terminal passes to reed, into the user's VS Code keybindings.json and returns the outcome beside the error;
// a skipped keybindings seed never fails the seed.
func Seed(l *lymcwd.Location, slug string) (vscode.KeybindingsResult, error) {
	result, err := seed(l, slug)
	return result.keybindings, err
}

// seedResult is what seed did: the keybindings seed's outcome, the path a launch of the worktree opens, the directory the opened window is on and the files the launch opens beside the path.
// The window directory is the launch path itself unless that is a workspace file.
type seedResult struct {
	keybindings vscode.KeybindingsResult
	launchPath  string
	windowDir   string
	openFiles   []string
}

// seed writes slug's .vscode/ config, seeds the user's keybindings.json and, for the prime or a beam, regenerates the workspace files;
// the launch path is then the hub workspace file for the prime and the beam's own for a beam.
// A task's launch path is its bare folder at its own anchor.
// The config is written before the regeneration, so a beam's file splices the settings just written.
// The files to open are those of the worktree's place in ide.yaml, read over the config stack of slug's own worktree, so its local override applies.
func seed(l *lymcwd.Location, slug string) (seedResult, error) {
	own, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return seedResult{}, fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	ide, err := ideconfig.LoadConfig(fabricengine.ConfigDirs(own))
	if err != nil {
		return seedResult{}, fmt.Errorf("load the ide config: %w", err)
	}
	worktreeDir, look, primeName, primeResolved := resolveSpawnTarget(l, own, slug)

	if err := writeWorktreeConfig(own, worktreeDir, slug, look, ide.RenderMarkdown); err != nil {
		return seedResult{}, err
	}

	anchorPath := filepath.Join(worktreeDir, own.AnchorRel)
	result := seedResult{keybindings: seedKeybindings(), launchPath: anchorPath, windowDir: anchorPath}
	isPrime := primeResolved && slug == primeName
	if !isPrime && (!primeResolved || !isBeam(l, slug)) {
		result.openFiles = filesToOpen(anchorPath, ide.OpenFiles.Task, taskBeam(l, slug))
		return result, nil
	}
	hubWorkspacePath, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(l.HubPath), l)
	if err != nil {
		return result, err
	}
	result.launchPath = hubWorkspacePath
	result.openFiles = filesToOpen(anchorPath, ide.OpenFiles.Prime, func() string { return primeName })
	if !isPrime {
		result.launchPath = fabricengine.BeamWorkspacePath(l, slug)
		result.openFiles = filesToOpen(anchorPath, ide.OpenFiles.Beam, func() string { return slug })
	}
	result.windowDir = filepath.Dir(result.launchPath)
	return result, nil
}

// isBeam reports whether the origin record of slug's pair carries the beam mark, the mark the workspace regeneration writes a beam's file by.
// An unreadable record is logged at Warn and reads as a task, so the spawn degrades to the bare folder.
func isBeam(l *lymcwd.Location, slug string) bool {
	origin, found, err := fabricengine.ReadOriginFor(l, slug)
	if err != nil {
		logger.Warn("ide: read the origin record; opening the bare folder", "slug", slug, "error", err)
		return false
	}
	return found && origin.Beam
}

// OpenTask launches VS Code on task slug's bare folder at the task's own anchor, with the files ide.yaml lists for a task, as Spawn does for a task pair, runs the machine's desktop window command and writes nothing.
// The ide.yaml is read over the config stack of slug's own worktree, and one that fails to load fails it before the launch.
func OpenTask(l *lymcwd.Location, slug string) error {
	own, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	ide, err := ideconfig.LoadConfig(fabricengine.ConfigDirs(own))
	if err != nil {
		return fmt.Errorf("load the ide config: %w", err)
	}
	launchPath := filepath.Join(fabricengine.WorktreePath(l, slug), own.AnchorRel)
	if err := CodeLauncher(launchPath, filesToOpen(launchPath, ide.OpenFiles.Task, taskBeam(l, slug))...); err != nil {
		return err
	}
	placeWindow(l, launchPath)
	return nil
}

// filesToOpen returns the absolute paths under anchorPath that entries name, in order, keeping only those naming an existing regular file.
// An entry naming ideconfig.BeamPlaceholder has it replaced by what beam returns; beam runs once, and only when an entry names the placeholder.
// An entry beam answers "" for, and one naming no file, is skipped and logged at Info, since VS Code would open an empty editor on a path that does not exist.
func filesToOpen(anchorPath string, entries []string, beam func() string) []string {
	var files []string
	beamName, beamAsked := "", false
	for _, entry := range entries {
		if strings.Contains(entry, ideconfig.BeamPlaceholder) {
			if !beamAsked {
				beamName, beamAsked = beam(), true
			}
			if beamName == "" {
				logger.Info("ide: no beam resolves an open_files entry; skipping it", "entry", entry)
				continue
			}
			entry = strings.ReplaceAll(entry, ideconfig.BeamPlaceholder, beamName)
		}
		filePath := filepath.Join(anchorPath, filepath.FromSlash(entry))
		if info, err := os.Stat(filePath); err != nil || !info.Mode().IsRegular() {
			logger.Info("ide: an open_files entry names no file; skipping it", "path", filePath)
			continue
		}
		files = append(files, filePath)
	}
	return files
}

// taskBeam returns the function naming task slug's beam: the pair it was forked from, or the prime's name for a task forked from the prime.
// A prime or parent it cannot resolve is logged at Warn and answers "".
func taskBeam(l *lymcwd.Location, slug string) func() string {
	return func() string {
		primeName, err := fabricengine.PrimeName(l)
		if err != nil {
			logger.Warn("ide: resolve the prime for a task's beam", "slug", slug, "error", err)
			return ""
		}
		parent, err := parentOf(l.HubPath, primeName, slug)
		if err != nil {
			logger.Warn("ide: read the parent of a task for its beam", "slug", slug, "error", err)
			return ""
		}
		if parent == "" {
			return primeName
		}
		return parent
	}
}

// placeWindow runs the machine's desktop window command, in the directory windowDir, for the VS Code window just launched, under l's config stack.
// The window is open by then, so a failure of the command only warns.
func placeWindow(l *lymcwd.Location, windowDir string) {
	if err := desktophook.AfterWindow(fabricengine.ConfigDirs(l), windowDir); err != nil {
		logger.Warn("ide: the window command failed; the window is open", "dir", windowDir, "cause", err)
	}
}

// writeWorktreeConfig writes slug's .vscode/ config at own's anchor through writeVSCodeConfig, stamping in the absolute lym path.
// The path degrades to the empty string on error so vscode.WriteConfig's own bare-name fallback rule applies.
func writeWorktreeConfig(own *lymcwd.Location, worktreeDir, slug string, look vscode.Look, renderMarkdown bool) error {
	lymPath, _ := os.Executable()
	return writeVSCodeConfig(own, worktreeDir, slug, look, renderMarkdown, lymPath)
}

// seedKeybindings seeds the user's keybindings.json through the KeybindingsPath seam; a path it cannot resolve is a skip.
func seedKeybindings() vscode.KeybindingsResult {
	keybindingsPath, err := KeybindingsPath()
	if err != nil {
		return vscode.KeybindingsResult{Outcome: vscode.KeybindingsSkipped, Reason: fmt.Sprintf("resolve the keybindings path: %v", err)}
	}
	return vscode.SeedKeybindings(keybindingsPath)
}

// writeVSCodeConfig keeps .vscode/ out of git and writes the .vscode/ config at the worktree's anchor.
// When .vscode/tasks.json is already tracked, it logs one warning and writes neither file nor the exclude line.
// Otherwise it first adds an anchored .vscode line to the shared info/exclude, then calls vscode.WriteConfig.
func writeVSCodeConfig(l *lymcwd.Location, worktreeDir, slug string, look vscode.Look, renderMarkdown bool, lymPath string) error {
	tasksRel := path.Join(filepath.ToSlash(l.AnchorRel), ".vscode", "tasks.json")
	tracked, err := fabricengine.PathTracked(worktreeDir, tasksRel)
	if err != nil {
		return fmt.Errorf("check %s tracked: %w", tasksRel, err)
	}
	if tracked {
		logger.Warn("tracked .vscode/tasks.json; leaving .vscode/ untouched", "slug", slug, "path", tasksRel)
		return nil
	}
	excludePath, changed, err := fabricengine.ExcludeAnchoredDir(worktreeDir, l.AnchorRel, ".vscode")
	if err != nil {
		return fmt.Errorf("exclude .vscode: %w", err)
	}
	if changed {
		logger.Info("excluded .vscode/ from git", "slug", slug, "exclude", excludePath)
	}
	if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, look, renderMarkdown, lymPath); err != nil {
		return fmt.Errorf("write vscode config: %w", err)
	}
	return nil
}

// resolveSpawnTarget resolves slug's worktree path and window look, own being slug's location, and the prime's name.
// A prime-resolution failure is logged here, once, and degrades rather than failing the spawn:
// slug reads as a task under the prime, and the returned flag is false.
// A wrong colour or a missing hub workspace is not worth aborting over.
func resolveSpawnTarget(l, own *lymcwd.Location, slug string) (worktreeDir string, look vscode.Look, primeName string, primeResolved bool) {
	worktreeDir = fabricengine.WorktreePath(l, slug)
	primeName, primeErr := fabricengine.PrimeName(l)
	if primeErr != nil {
		logger.Warn("resolve prime name; opening the bare folder", "slug", slug, "error", primeErr)
	}
	return worktreeDir, lookFor(l, own, slug, primeName), primeName, primeErr == nil
}

// lookFor returns the window look of the worktree slug, whose location is own: its title, its project's colour and its own colour.
// The prime takes green on both, and a beam its board-recorded colour on both.
// A task takes its parent's colour, green under the prime or the beam's colour under a beam, as its project colour, and as its own colour keeps the one its settings.json holds while that is allowed and free, else picks one avoiding its siblings'.
// An empty primeName, an unreadable board or an unresolvable parent reads slug as a task under the prime, logged at Warn.
func lookFor(l, own *lymcwd.Location, slug, primeName string) vscode.Look {
	title := slug
	if shortname, found := fabricengine.ReadShortname(fabricengine.BoltDir(l.HubPath)); found {
		title = strings.ToUpper(shortname) + ": " + slug
	}
	if primeName != "" && slug == primeName {
		return vscode.Look{Title: title, ProjectColor: vscode.PrimeColor, WorktreeColor: vscode.PrimeColor}
	}

	beams, err := beamColors(l.HubPath)
	if err != nil {
		logger.Warn("read the board's beams; colouring as a task under the prime", "slug", slug, "error", err)
	}
	if color, isBeam := beams[slug]; isBeam {
		return vscode.Look{Title: title, ProjectColor: color, WorktreeColor: color}
	}

	projectColor := vscode.PrimeColor
	if primeName != "" {
		if parent, err := parentOf(l.HubPath, primeName, slug); err != nil {
			logger.Warn("read the parent of a worktree; colouring it as a task under the prime", "slug", slug, "error", err)
		} else if color, isBeam := beams[parent]; isBeam {
			projectColor = color
		}
	}
	current := vscode.ActivityBarColor(own.AnchorPath())
	siblings := vscode.SiblingWorktreeColors(l, projectColor, slug)
	return vscode.Look{Title: title, ProjectColor: projectColor, WorktreeColor: vscode.PickWorktreeColor(projectColor, current, siblings)}
}

// parentOf returns the slug of the pair slug was forked from, "" for the prime, read over the prime primeName of the hub at hubPath.
func parentOf(hubPath, primeName, slug string) (string, error) {
	prime, err := lymcwd.NamedWorktree(hubPath, primeName)
	if err != nil {
		return "", fmt.Errorf("read the prime's anchor: %w", err)
	}
	return hubgeom.ParentOf(prime, slug)
}

// beamColors maps the slug of every open beam on the hub's board to its colour:
// its recorded colour, or for a beam with none the colour a new beam would record, without recording it.
// An absent board holds no beam.
func beamColors(hubPath string) (map[string]string, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(hubPath))
	if err != nil {
		return nil, err
	}
	colors := map[string]string{}
	var held, unrecorded []string
	for _, task := range tasks {
		if task.Kind != boardengine.KindProject || !task.Beam || (task.Status != nil && *task.Status == "done") {
			continue
		}
		if task.BeamColor == "" {
			unrecorded = append(unrecorded, task.Slug)
			continue
		}
		colors[task.Slug] = task.BeamColor
		held = append(held, task.BeamColor)
	}
	for _, slug := range unrecorded {
		color := vscode.PickBeamColor(held)
		colors[slug] = color
		held = append(held, color)
	}
	return colors, nil
}
