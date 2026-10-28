// spawn.go implements the ide entry point Spawn (`ide spawn`), and its two halves, Seed and OpenTask.
// It resolves a worktree's window title and colours from the prime, the board's beams and the worktree's parent, generates the worktree's .vscode/ config, and launches VS Code;
// it regenerates tasks.json on every call and keeps .vscode/ out of git through info/exclude.
// A task slug opens its bare folder.
// The prime instead opens the lym-generated hub workspace <hub>/<RepoName>.code-workspace (prime, Bolt, every task pair), regenerated on every prime spawn.

package ideengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// CodeLauncher is the exported package-level injectable seam that can be overridden in tests.
// It defaults to vscode.Launch but can be stubbed to record its argument for testing.
// Exported so that cli_test.go in the idecli package can swap it.
var CodeLauncher = vscode.Launch

// KeybindingsPath is the injectable seam that names the user's VS Code keybindings.json.
// It defaults to vscode.UserKeybindingsPath and is exported so that tests in other packages can point it at a temporary file.
var KeybindingsPath = vscode.UserKeybindingsPath

// Spawn regenerates a worktree's .vscode/ config on every call and launches VS Code.
// It keeps .vscode/ out of git through info/exclude and never writes a .gitignore;
// a tracked .vscode/tasks.json is left alone with a warning, and the launch still happens.
// A task slug launches its bare folder <hub>/<slug>/<its own anchor> and writes no workspace file.
// When slug names the prime, Spawn regenerates the hub workspace file <hub>/<RepoName>.code-workspace, whose settings carry the prime's .vscode/settings.json, and launches that file instead;
// every error on that path is returned, never degraded to the bare folder.
// A failure to resolve the prime's name is logged and degrades to the bare-folder path.
// Before launching it seeds the lym block of key bindings into the user's VS Code keybindings.json and returns the outcome beside the error;
// a skipped seed never fails the spawn.
func Spawn(l *lymcwd.Location, slug string) (vscode.KeybindingsResult, error) {
	own, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return vscode.KeybindingsResult{}, fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	worktreeDir, look, primeName, primeResolved := resolveSpawnTarget(l, own, slug)

	if err := writeWorktreeConfig(own, worktreeDir, slug, look); err != nil {
		return vscode.KeybindingsResult{}, err
	}

	keybindings := seedKeybindings()

	openDir := filepath.Join(worktreeDir, own.AnchorRel)
	if primeResolved && slug == primeName {
		workspacePath, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(l.HubPath), l)
		if err != nil {
			return keybindings, err
		}
		return keybindings, CodeLauncher(workspacePath)
	}

	return keybindings, CodeLauncher(openDir)
}

// Seed writes the window title and colours and the .vscode/ config of the worktree slug, the prime or a pair, as Spawn does, and launches nothing.
// It leaves the user's keybindings.json alone.
func Seed(l *lymcwd.Location, slug string) error {
	own, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	worktreeDir, look, _, _ := resolveSpawnTarget(l, own, slug)
	return writeWorktreeConfig(own, worktreeDir, slug, look)
}

// OpenTask launches VS Code on task slug's bare folder at the task's own anchor, as Spawn does for a task pair, and writes nothing.
func OpenTask(l *lymcwd.Location, slug string) error {
	own, err := lymcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return fmt.Errorf("read the anchor of %q: %w", slug, err)
	}
	return CodeLauncher(filepath.Join(fabricengine.WorktreePath(l, slug), own.AnchorRel))
}

// writeWorktreeConfig writes slug's .vscode/ config at own's anchor through writeVSCodeConfig, stamping in the absolute lym path.
// The path degrades to the empty string on error so vscode.WriteConfig's own bare-name fallback rule applies.
func writeWorktreeConfig(own *lymcwd.Location, worktreeDir, slug string, look vscode.Look) error {
	lymPath, _ := os.Executable()
	return writeVSCodeConfig(own, worktreeDir, slug, look, lymPath)
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
func writeVSCodeConfig(l *lymcwd.Location, worktreeDir, slug string, look vscode.Look, lymPath string) error {
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
	if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, look, lymPath); err != nil {
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
	if shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(l.HubPath)); found {
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
