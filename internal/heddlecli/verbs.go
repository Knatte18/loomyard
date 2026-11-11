// verbs.go holds the heddle verbs: list, get, set, remove, show and render.
// It also holds the locked write set and remove share and their render into every live worktree of the hub.

package heddlecli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// stdinTextFile is the --text-file value that reads the text from stdin.
const stdinTextFile = "-"

// operatorAnnotation marks a verb as one an operator runs.
var operatorAnnotation = map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator}

// entryView is the JSON form of an entry the verbs print.
type entryView struct {
	Slug       string   `json:"slug"`
	Layer      string   `json:"layer"`
	Overridden bool     `json:"overridden,omitempty"`
	Order      int      `json:"order"`
	Targets    []string `json:"targets"`
	Text       string   `json:"text,omitempty"`
}

// viewOf returns the view of a layered entry, its text left out.
func viewOf(entry heddleengine.LayeredEntry) entryView {
	return entryView{Slug: entry.Slug, Layer: entry.Layer, Overridden: entry.Overridden, Order: entry.Order, Targets: entry.Targets}
}

// fileTargetView is the JSON form list prints of a file target and the anchor-relative files it renders into.
type fileTargetView struct {
	Target string   `json:"target"`
	Files  []string `json:"files"`
}

// fileTargets returns every file target in table order with the files it renders into.
func fileTargets() []fileTargetView {
	var views []fileTargetView
	for _, name := range heddleengine.TargetNames() {
		if files := heddleengine.TargetFiles(name); len(files) > 0 {
			views = append(views, fileTargetView{Target: name, Files: files})
		}
	}
	return views
}

// fileTargetHelp returns the help lines naming each file target and the files it renders into at a worktree's anchor.
func fileTargetHelp() string {
	var out strings.Builder
	out.WriteString("The file targets and the files each renders into at a worktree's anchor:\n")
	for _, view := range fileTargets() {
		fmt.Fprintf(&out, "  %-18s %s\n", view.Target, strings.Join(view.Files, ", "))
	}
	return out.String()
}

// layerFlagUsage is the help text of the --layer flag set and remove share.
const layerFlagUsage = "the layer to change: empty for the repository's entries, " + heddleengine.OverrideLayer + " for the hub's override of a shipped entry"

// noRenderFlag is the flag set and remove share that skips the render into every live worktree of the hub.
const noRenderFlag = "no-render"

// noRenderFlagUsage is the help text of the --no-render flag.
const noRenderFlagUsage = "save the change without rendering it into the hub's worktrees, such as for all but the last of a run of edits"

// worktreeFailure is a worktree whose render after a write failed or left a file unwritten, as the verbs print it.
type worktreeFailure struct {
	Worktree string `json:"worktree"`
	Error    string `json:"error"`
}

// overrideLayer reports whether layer names the lym overrides.
// It refuses any value but the empty layer and heddleengine.OverrideLayer.
func overrideLayer(layer string) (bool, error) {
	switch layer {
	case "":
		return false, nil
	case heddleengine.OverrideLayer:
		return true, nil
	}
	return false, fmt.Errorf("heddle: unknown layer %q; way forward: omit --layer for a repository entry, or give --layer %s to override a shipped entry", layer, heddleengine.OverrideLayer)
}

// requireOverrideOn refuses a new override while the hub's heddle.yaml leaves lym_override off.
// Every render would otherwise ignore it.
func (c *heddleCLI) requireOverrideOn() error {
	cfg, err := heddleengine.LoadConfig(configengine.Dirs{Board: c.boltDir})
	if err != nil {
		return fmt.Errorf("heddle: %w", err)
	}
	if !cfg.LymOverride {
		return fmt.Errorf("heddle: the hub's heddle.yaml leaves %s off, so renders would ignore the override; way forward: run `lym config heddle --set %s=true`, then this verb again", heddleengine.LymOverrideKey, heddleengine.LymOverrideKey)
	}
	return nil
}

// fail prints err as the verb's JSON refusal and records exit code 1.
func (c *heddleCLI) fail(cmd *cobra.Command, err error) error {
	clihelp.SetExit(cmd.Context(), output.Err(c.out, err.Error()))
	return nil
}

// listCmd returns the `heddle list` verb.
func (c *heddleCLI) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "list every entry in render order with its layer, order and targets",
		Annotations: operatorAnnotation,
		Long: `Prints every entry in render order with its slug, layer, order and targets,
without its text: the entries lym ships first, layer shipped, then the
repository's, layer repository. A shipped entry whose text the hub's override
replaces is marked overridden. An override file that does not load is reported
in warnings and ignored. file_targets names each file target and the files it
renders into at a worktree's anchor.

Example:
  lym heddle list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			layers, err := heddleengine.ReadLayers(c.boltDir)
			if err != nil {
				return c.fail(cmd, err)
			}
			entries := []entryView{}
			for _, entry := range layers.Ordered() {
				entries = append(entries, viewOf(entry))
			}
			fields := map[string]any{"entries": entries, "file_targets": fileTargets()}
			if layers.OverrideWarning != "" {
				fields["warnings"] = []string{layers.OverrideWarning}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(c.out, fields))
			return nil
		},
	}
}

// getCmd returns the `heddle get` verb.
func (c *heddleCLI) getCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "get <slug>",
		Short:       "print one entry in full",
		Annotations: operatorAnnotation,
		Long: `Prints the entry with the given slug in full, its layer and text included,
a shipped entry with the text renders use now. A repository entry that
shares the slug of a shipped one is printed in shadowed, and an override file
that does not load is reported in warnings and ignored.

Example:
  lym heddle get house-rules`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			layers, err := heddleengine.ReadLayers(c.boltDir)
			if err != nil {
				return c.fail(cmd, err)
			}
			var views []entryView
			for _, entry := range layers.Ordered() {
				if entry.Slug == args[0] {
					view := viewOf(entry)
					view.Text = entry.Text
					views = append(views, view)
				}
			}
			if len(views) == 0 {
				return c.fail(cmd, fmt.Errorf("heddle: no entry %q; way forward: run `lym heddle list` to see the entries", args[0]))
			}
			fields := map[string]any{"entry": views[0]}
			if len(views) > 1 {
				fields["shadowed"] = views[1:]
			}
			if layers.OverrideWarning != "" {
				fields["warnings"] = []string{layers.OverrideWarning}
			}
			clihelp.SetExit(cmd.Context(), output.Ok(c.out, fields))
			return nil
		},
	}
}

// setCmd returns the `heddle set` verb.
func (c *heddleCLI) setCmd() *cobra.Command {
	var textFile, targets, layer string
	var order int
	var noRender bool
	cmd := &cobra.Command{
		Use:         "set <slug>",
		Short:       "create an entry or change the fields given",
		Annotations: operatorAnnotation,
		Long: `Creates the repository entry with the given slug, or changes the fields given
of an existing one. A new entry needs all three flags. A new entry under a
slug lym ships is refused: shipped entries change only with a lym release.

  --text-file <path>   the entry's text from a file, or from stdin for -
  --order <n>          the entry's place in render order
  --targets <a,b>      the targets the entry is delivered to
  --layer lym          override the text of the shipped entry with the given
                       slug instead; it takes --text-file alone, and only a hub
                       whose heddle.yaml sets lym_override takes it
  --no-render          skip the render into the hub's worktrees

` + fileTargetHelp() + `
claude-file reaches Claude sessions alone, codex-file Codex sessions alone, and
agents-file both. An entry aimed at claude-settings holds one JSON object and
no other target; every render merges it into that file.

The change is saved under the Bolt write lock with HEDDLE.md, the overview of
every entry, rendered beside it. Every file target is then rendered into every
live worktree of the hub, the prime, beams and task pairs, and the worktrees
rendered and failed are printed, with render_warnings beside them for the
warnings of the rendered ones or a failure to list the worktrees; a session
there takes the change at its next compaction. The board's detached Bolt sync then commits and pushes
HEDDLE.md and the store. A refused change saves and renders nothing.

Examples:
  lym heddle set house-rules --text-file rules.md --order 10 --targets agents-file,claude-prompt
  lym heddle set house-deny --text-file deny.json --order 20 --targets claude-settings
  lym heddle set lym-batching --layer lym --text-file tuned.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			override, err := overrideLayer(layer)
			if err != nil {
				return c.fail(cmd, err)
			}
			slug := args[0]
			if override {
				return c.setOverride(cmd, slug, textFile, !noRender)
			}
			var patch heddleengine.Patch
			if cmd.Flags().Changed("text-file") {
				text, err := readText(cmd.InOrStdin(), textFile)
				if err != nil {
					return c.fail(cmd, err)
				}
				patch.Text = &text
			}
			if cmd.Flags().Changed("order") {
				patch.Order = &order
			}
			if cmd.Flags().Changed("targets") {
				patch.Targets = splitTargets(targets)
				if patch.Targets == nil {
					// A given but empty --targets must reach validation as an empty list, not read as "unchanged".
					patch.Targets = []string{}
				}
			}
			return c.finishWrite(cmd, slug, c.repositoryFile(), !noRender, func(store *heddleengine.Store) error { return store.Set(slug, patch) })
		},
	}
	cmd.Flags().StringVar(&textFile, "text-file", "", "read the entry's text from this file, or from stdin for -")
	cmd.Flags().IntVar(&order, "order", 0, "the entry's place in render order")
	cmd.Flags().StringVar(&targets, "targets", "", "comma-separated targets the entry is delivered to")
	cmd.Flags().StringVar(&layer, "layer", "", layerFlagUsage)
	cmd.Flags().BoolVar(&noRender, noRenderFlag, false, noRenderFlagUsage)
	return cmd
}

// setOverride stores the text of textFile as the hub's override of the shipped entry slug.
// With render set it then renders the hub's worktrees as every write does.
// It refuses --order and --targets, a missing --text-file, and a hub whose heddle.yaml leaves lym_override off.
// A refusal saves nothing.
func (c *heddleCLI) setOverride(cmd *cobra.Command, slug, textFile string, render bool) error {
	if cmd.Flags().Changed("order") || cmd.Flags().Changed("targets") {
		return c.fail(cmd, fmt.Errorf("heddle: --layer %s overrides a shipped entry's text alone, so it takes no --order or --targets; way forward: give --text-file only", heddleengine.OverrideLayer))
	}
	if !cmd.Flags().Changed("text-file") {
		return c.fail(cmd, fmt.Errorf("heddle: --layer %s needs the override's text; way forward: give it with --text-file", heddleengine.OverrideLayer))
	}
	if err := c.requireOverrideOn(); err != nil {
		return c.fail(cmd, err)
	}
	text, err := readText(cmd.InOrStdin(), textFile)
	if err != nil {
		return c.fail(cmd, err)
	}
	return c.finishWrite(cmd, slug, c.overrideFile(), render, func(overrides *heddleengine.Store) error { return overrides.SetOverride(slug, text) })
}

// removeCmd returns the `heddle remove` verb.
func (c *heddleCLI) removeCmd() *cobra.Command {
	var layer string
	var noRender bool
	cmd := &cobra.Command{
		Use:         "remove <slug>",
		Short:       "remove an entry",
		Annotations: operatorAnnotation,
		Long: `Removes the repository entry with the given slug, saves the store under the
Bolt write lock with HEDDLE.md rendered beside it, renders every file target
into every live worktree of the hub and prints the worktrees rendered and
failed and render_warnings, and lets the board's detached Bolt sync commit and push the store and
HEDDLE.md. --no-render skips the render into the worktrees.
A slug lym ships is refused unless the repository holds an entry under it.
With --layer lym it removes the hub's override of
the shipped entry instead, whatever heddle.yaml's lym_override holds.

Examples:
  lym heddle remove house-rules
  lym heddle remove lym-batching --layer lym`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			override, err := overrideLayer(layer)
			if err != nil {
				return c.fail(cmd, err)
			}
			slug := args[0]
			if override {
				return c.finishWrite(cmd, slug, c.overrideFile(), !noRender, func(overrides *heddleengine.Store) error { return overrides.RemoveOverride(slug) })
			}
			return c.finishWrite(cmd, slug, c.repositoryFile(), !noRender, func(store *heddleengine.Store) error { return store.Remove(slug) })
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", layerFlagUsage)
	cmd.Flags().BoolVar(&noRender, noRenderFlag, false, noRenderFlagUsage)
	return cmd
}

// showCmd returns the `heddle show` verb.
func (c *heddleCLI) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "show <target>",
		Short:       "print what a target renders to now for the current worktree",
		Annotations: operatorAnnotation,
		Long: `Prints the text the named target renders to now for the current worktree and
writes nothing. agents-file is refused, since its entries render into the files
of claude-file and codex-file; show one of those instead.

Example:
  lym heddle show claude-file`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			text, err := c.renderer.Show(c.boltDir, args[0], c.location.AnchorPath())
			if err != nil {
				return c.fail(cmd, fmt.Errorf("heddle: %w", err))
			}
			_, _ = io.WriteString(c.out, text)
			return nil
		},
	}
}

// renderCmd returns the `heddle render` verb.
func (c *heddleCLI) renderCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "render [<worktree>]",
		Short:       "render every file target into a worktree of the hub",
		Annotations: operatorAnnotation,
		Long: `Renders every file target into the anchor directory of the named worktree of
the current hub, the current one by default. The name is the last element of a path lym fabric list
prints. The worktree's exclude lines are seeded first, and the files written
and removed and the render's warnings are printed.

Example:
  lym heddle render`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			location := c.location
			if len(args) == 1 {
				named, err := c.namedWorktree(args[0])
				if err != nil {
					return c.fail(cmd, err)
				}
				location = named
			}
			report, err := c.renderInto(location)
			if err != nil {
				message := fmt.Sprintf("heddle: render %s: %v", location.WorktreeName, err)
				if !strings.Contains(message, "lym heddle render") {
					message += "; way forward: run `lym heddle render` again once the cause is fixed"
				}
				return c.fail(cmd, fmt.Errorf("%s", message))
			}
			clihelp.SetExit(cmd.Context(), output.Ok(c.out, map[string]any{
				"worktree": location.WorktreeName,
				"written":  nonNil(report.Written),
				"removed":  nonNil(report.Removed),
				"warnings": nonNil(report.Warnings),
			}))
			return nil
		},
	}
}

// storeFile is a Bolt file a write changes: how it is loaded and saved, and its name.
type storeFile struct {
	name string
	load func(boltDir string) (heddleengine.Store, error)
	save func(store heddleengine.Store) error
}

// repositoryFile returns heddle.json, saved through c's save seam.
func (c *heddleCLI) repositoryFile() storeFile {
	return storeFile{name: heddleengine.FileName, load: heddleengine.Load, save: c.saveStore}
}

// overrideFile returns the file of the hub's lym overrides.
func (c *heddleCLI) overrideFile() storeFile {
	return storeFile{
		name: heddleengine.OverrideFileName,
		load: heddleengine.LoadOverrides,
		save: func(store heddleengine.Store) error { return heddleengine.SaveOverrides(c.boltDir, store) },
	}
}

// finishWrite applies change to file under the Bolt write lock and prints the result for slug.
// With render set it then renders every file target into every live worktree of the hub.
// It prints the worktrees rendered and failed and the warnings of the rendered ones.
// It starts the board's detached Bolt sync last, so no render waits behind the sync's hold of the lock.
// A refusal prints as the verb's error and renders and syncs nothing.
// A sync that did not start prints beside the result.
// It pulls and commits nothing in the foreground.
func (c *heddleCLI) finishWrite(cmd *cobra.Command, slug string, file storeFile, render bool, change func(*heddleengine.Store) error) error {
	if err := c.saveLocked(file, change); err != nil {
		return c.fail(cmd, err)
	}
	fields := map[string]any{"slug": slug}
	if render {
		rendered, failed, warnings := c.renderEveryWorktree()
		if failed == nil {
			failed = []worktreeFailure{}
		}
		fields["rendered"] = nonNil(rendered)
		fields["failed"] = failed
		if len(warnings) > 0 {
			fields["render_warnings"] = warnings
		}
	}
	if warning := c.syncAfterSave(file); warning != "" {
		fields["warning"] = warning
	}
	clihelp.SetExit(cmd.Context(), output.Ok(c.out, fields))
	return nil
}

// syncAfterSave starts the board's detached Bolt sync of the checkout once file is saved and the lock released.
// It returns "" when the sync started.
// A sync that fails to start leaves the saved file in place;
// the failure is logged and returned as a warning naming `lym board sync`.
func (c *heddleCLI) syncAfterSave(file storeFile) string {
	if err := c.startSync(); err != nil {
		logger.Warn("heddle: the Bolt sync did not start after the save", "file", file.name, "error", err)
		return fmt.Sprintf("%s is saved but the Bolt sync did not start (%v); way forward: run `lym board sync` to commit and push it", file.name, err)
	}
	return ""
}

// renderEveryWorktree renders every file target into each live worktree of c's hub, in the order git lists them.
// A worktree git marks prunable is skipped, since its directory is gone.
// A worktree still being made, listed under a worktree staging directory, is skipped, since it has no final path yet.
// A worktree whose path is not a direct child of the hub is skipped, since the hub-level name does not locate it.
// It returns the names of the worktrees rendered with no file left unwritten.
// It returns the worktrees whose render failed or left a file unwritten, each with its cause and way forward.
// It returns the warnings of the rendered worktrees, each opening with its worktree's name.
// A hub whose worktrees do not list gives one warning and renders nothing.
func (c *heddleCLI) renderEveryWorktree() (rendered []string, failed []worktreeFailure, warnings []string) {
	entries, err := fabricengine.List(c.location.WorktreePath())
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("the hub's worktrees did not list, so none was rendered: %v; way forward: run `lym heddle render <worktree>` for each worktree `lym fabric list` prints", err)}
	}
	for _, entry := range entries {
		if entry.Prunable || fabricengine.InWorktreeStaging(entry.Path) {
			continue
		}
		path := filepath.Clean(filepath.FromSlash(entry.Path))
		if !samePath(filepath.Dir(path), c.location.HubPath) {
			continue
		}
		name := filepath.Base(path)
		location, err := lymcwd.NamedWorktree(c.location.HubPath, name)
		var report heddleengine.Report
		if err == nil {
			report, err = c.renderInto(location)
		}
		switch {
		case err != nil:
			failed = append(failed, worktreeFailure{Worktree: name, Error: withRenderWayForward(err.Error(), name)})
		case len(report.Unwritten) > 0:
			cause := fmt.Sprintf("%s left unwritten: %s", strings.Join(report.Unwritten, ", "), strings.Join(report.Warnings, "; "))
			failed = append(failed, worktreeFailure{Worktree: name, Error: withRenderWayForward(cause, name)})
		default:
			rendered = append(rendered, name)
			for _, warning := range report.Warnings {
				warnings = append(warnings, name+": "+warning)
			}
		}
	}
	return rendered, failed, warnings
}

// samePath reports whether a and b name the same directory once symlinks are resolved and, on Windows, case is folded.
// A path that does not resolve is compared as cleaned.
func samePath(a, b string) bool {
	normalize := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return filepath.Clean(path)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(normalize(a), normalize(b))
	}
	return normalize(a) == normalize(b)
}

// withRenderWayForward appends to cause the way forward of rendering the worktree name by hand.
// A cause that already names `lym heddle render` is returned as it is.
func withRenderWayForward(cause, name string) string {
	if strings.Contains(cause, "lym heddle render") {
		return cause
	}
	return fmt.Sprintf("%s; way forward: run `lym heddle render %s` once the cause is fixed", cause, name)
}

// renderInto seeds the exclude lines of location's worktree and renders every file target into its anchor.
func (c *heddleCLI) renderInto(location *lymcwd.Location) (heddleengine.Report, error) {
	event, err := fabricengine.EventFor(location, nil)
	if err != nil {
		return heddleengine.Report{}, fmt.Errorf("seed the exclude lines: %w", err)
	}
	return c.renderer.Render(heddleengine.RenderRequest{
		Anchor:     event.Anchor,
		ScratchDir: event.ScratchDir,
		BoltDir:    event.BoltDir,
		Excluded:   event.Excluded,
	})
}

// saveLocked loads file, applies change, saves it and renders the overview, all under the Bolt write lock.
func (c *heddleCLI) saveLocked(file storeFile, change func(*heddleengine.Store) error) error {
	lockPath, err := fabricengine.BoltLockPath(c.boltDir, fabricengine.BoardWriteLockFile)
	if err != nil {
		return fmt.Errorf("heddle: %w", err)
	}
	boltLock, acquired, err := lock.AcquireWriteLockWithin(lockPath, heddleengine.DefaultBoltLockWait)
	if err != nil {
		return fmt.Errorf("heddle: %w", err)
	}
	if !acquired {
		return fmt.Errorf("heddle: the Bolt checkout stayed locked for %s, so nothing was saved; way forward: run the verb again", heddleengine.DefaultBoltLockWait)
	}
	defer func() { _ = boltLock.Release() }()

	store, err := file.load(c.boltDir)
	if err != nil {
		return err
	}
	if err := change(&store); err != nil {
		return err
	}
	if err := file.save(store); err != nil {
		return err
	}
	if _, err := heddleengine.RenderOverview(c.boltDir); err != nil {
		return fmt.Errorf("heddle: %s is saved but %s was not rendered: %w; way forward: run the verb again once the cause is fixed", file.name, heddleengine.OverviewFileName, err)
	}
	return nil
}

// readText returns the text of the file at path, or of stdin for the stdin marker.
func readText(stdin io.Reader, path string) (string, error) {
	var data []byte
	var err error
	if path == stdinTextFile {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("heddle: read the entry text from %s: %w", path, err)
	}
	return string(data), nil
}

// splitTargets splits a comma-separated target list, trimming each name and dropping empty ones.
func splitTargets(list string) []string {
	var names []string
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// nonNil returns list, or an empty list for nil, so a JSON field prints [] rather than null.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}
