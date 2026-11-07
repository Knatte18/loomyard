// verbs.go holds the heddle verbs: list, get, set, remove, show and render, and the locked write set and remove share.

package heddlecli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
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

// layerFlagUsage is the help text of the --layer flag set and remove share.
const layerFlagUsage = "the layer to change: empty for the repository's entries, " + heddleengine.OverrideLayer + " for the hub's override of a shipped entry"

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
in warnings and ignored.

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
			fields := map[string]any{"entries": entries}
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

An entry aimed at claude-settings holds one JSON object and no other target;
every render merges it into the worktree's .claude/settings.local.json.

The change is saved under the Bolt write lock, and the board's detached Bolt
sync then commits and pushes it. A refused change saves nothing.

Examples:
  lym heddle set house-rules --text-file rules.md --order 10 --targets claude-file,claude-prompt
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
				return c.setOverride(cmd, slug, textFile)
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
			return c.finishWrite(cmd, slug, c.repositoryFile(), func(store *heddleengine.Store) error { return store.Set(slug, patch) })
		},
	}
	cmd.Flags().StringVar(&textFile, "text-file", "", "read the entry's text from this file, or from stdin for -")
	cmd.Flags().IntVar(&order, "order", 0, "the entry's place in render order")
	cmd.Flags().StringVar(&targets, "targets", "", "comma-separated targets the entry is delivered to")
	cmd.Flags().StringVar(&layer, "layer", "", layerFlagUsage)
	return cmd
}

// setOverride stores the text of textFile as the hub's override of the shipped entry slug.
// It refuses --order and --targets, a missing --text-file, and a hub whose heddle.yaml leaves lym_override off.
// A refusal saves nothing.
func (c *heddleCLI) setOverride(cmd *cobra.Command, slug, textFile string) error {
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
	return c.finishWrite(cmd, slug, c.overrideFile(), func(overrides *heddleengine.Store) error { return overrides.SetOverride(slug, text) })
}

// removeCmd returns the `heddle remove` verb.
func (c *heddleCLI) removeCmd() *cobra.Command {
	var layer string
	cmd := &cobra.Command{
		Use:         "remove <slug>",
		Short:       "remove an entry",
		Annotations: operatorAnnotation,
		Long: `Removes the repository entry with the given slug, saves the store under the
Bolt write lock, and lets the board's detached Bolt sync commit and push it.
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
				return c.finishWrite(cmd, slug, c.overrideFile(), func(overrides *heddleengine.Store) error { return overrides.RemoveOverride(slug) })
			}
			return c.finishWrite(cmd, slug, c.repositoryFile(), func(store *heddleengine.Store) error { return store.Remove(slug) })
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", layerFlagUsage)
	return cmd
}

// showCmd returns the `heddle show` verb.
func (c *heddleCLI) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "show <target>",
		Short:       "print what a target renders to now for the current worktree",
		Annotations: operatorAnnotation,
		Long: `Prints the text the named target renders to now for the current worktree and
writes nothing.

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
			event, err := fabricengine.EventFor(location, nil)
			if err != nil {
				return c.fail(cmd, fmt.Errorf("heddle: seed the exclude lines of %s: %w", location.WorktreeName, err))
			}
			report, err := c.renderer.Render(heddleengine.RenderRequest{
				Anchor:     event.Anchor,
				ScratchDir: event.ScratchDir,
				BoltDir:    event.BoltDir,
				Excluded:   event.Excluded,
			})
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

// finishWrite applies change to file through write and prints the result for slug.
// A refusal prints as the verb's error; a sync that did not start prints beside the result.
func (c *heddleCLI) finishWrite(cmd *cobra.Command, slug string, file storeFile, change func(*heddleengine.Store) error) error {
	c.syncWarning = ""
	if err := c.write(file, change); err != nil {
		return c.fail(cmd, err)
	}
	fields := map[string]any{"slug": slug}
	if c.syncWarning != "" {
		fields["warning"] = c.syncWarning
	}
	clihelp.SetExit(cmd.Context(), output.Ok(c.out, fields))
	return nil
}

// write applies change to file under the Bolt write lock and saves it.
// It then starts the board's detached Bolt sync once the lock is released.
// It pulls and commits nothing in the foreground and touches no worktree.
// A change the store refuses saves nothing and starts no sync.
// A sync that fails to start leaves the saved file in place and sets syncWarning.
func (c *heddleCLI) write(file storeFile, change func(*heddleengine.Store) error) error {
	if err := c.saveLocked(file, change); err != nil {
		return err
	}
	if err := c.startSync(); err != nil {
		logger.Warn("heddle: the Bolt sync did not start after the save", "file", file.name, "error", err)
		c.syncWarning = fmt.Sprintf("%s is saved but the Bolt sync did not start (%v); way forward: run `lym board sync` to commit and push it", file.name, err)
	}
	return nil
}

// saveLocked loads file, applies change and saves it, all under the Bolt write lock.
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
	return file.save(store)
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
