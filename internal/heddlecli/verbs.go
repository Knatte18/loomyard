// verbs.go holds the heddle verbs: list, get, set, remove, show and render, and the locked write set and remove share.

package heddlecli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
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
	Slug    string   `json:"slug"`
	Order   int      `json:"order"`
	Targets []string `json:"targets"`
	Text    string   `json:"text,omitempty"`
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
		Short:       "list every entry in render order with its order and targets",
		Annotations: operatorAnnotation,
		Long: `Prints every entry in render order with its slug, order and targets, without its text.

Example:
  lym heddle list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			store, err := heddleengine.Load(c.boltDir)
			if err != nil {
				return c.fail(cmd, err)
			}
			entries := []entryView{}
			for _, entry := range store.Ordered() {
				entries = append(entries, entryView{Slug: entry.Slug, Order: entry.Order, Targets: entry.Targets})
			}
			clihelp.SetExit(cmd.Context(), output.Ok(c.out, map[string]any{"entries": entries}))
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
		Long: `Prints the entry with the given slug in full, its text included.

Example:
  lym heddle get house-rules`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			store, err := heddleengine.Load(c.boltDir)
			if err != nil {
				return c.fail(cmd, err)
			}
			entry, found := store.Get(args[0])
			if !found {
				return c.fail(cmd, fmt.Errorf("heddle: no entry %q; way forward: run `lym heddle list` to see the entries", args[0]))
			}
			clihelp.SetExit(cmd.Context(), output.Ok(c.out, map[string]any{
				"entry": entryView{Slug: entry.Slug, Order: entry.Order, Targets: entry.Targets, Text: entry.Text},
			}))
			return nil
		},
	}
}

// setCmd returns the `heddle set` verb.
func (c *heddleCLI) setCmd() *cobra.Command {
	var textFile, targets string
	var order int
	cmd := &cobra.Command{
		Use:         "set <slug>",
		Short:       "create an entry or change the fields given",
		Annotations: operatorAnnotation,
		Long: `Creates the entry with the given slug, or changes the fields given of an
existing one. A new entry needs all three flags.

  --text-file <path>   the entry's text from a file, or from stdin for -
  --order <n>          the entry's place in render order
  --targets <a,b>      the targets the entry is delivered to

The change is saved under the Bolt write lock, and the board's detached Bolt
sync then commits and pushes it. A refused change saves nothing.

Example:
  lym heddle set house-rules --text-file rules.md --order 10 --targets claude-file,claude-prompt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
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
			slug := args[0]
			return c.finishWrite(cmd, slug, func(store *heddleengine.Store) error { return store.Set(slug, patch) })
		},
	}
	cmd.Flags().StringVar(&textFile, "text-file", "", "read the entry's text from this file, or from stdin for -")
	cmd.Flags().IntVar(&order, "order", 0, "the entry's place in render order")
	cmd.Flags().StringVar(&targets, "targets", "", "comma-separated targets the entry is delivered to")
	return cmd
}

// removeCmd returns the `heddle remove` verb.
func (c *heddleCLI) removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "remove <slug>",
		Short:       "remove an entry",
		Annotations: operatorAnnotation,
		Long: `Removes the entry with the given slug, saves the store under the Bolt write
lock, and lets the board's detached Bolt sync commit and push it.

Example:
  lym heddle remove house-rules`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			slug := args[0]
			return c.finishWrite(cmd, slug, func(store *heddleengine.Store) error { return store.Remove(slug) })
		},
	}
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
			text, err := c.renderer.Show(c.boltDir, args[0], c.location.WorktreePath())
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
		Long: `Renders every file target into the named worktree of the current hub, the
current one by default. The name is the last element of a path lym fabric list
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
				Root:       event.Root,
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

// finishWrite applies change through write and prints the result for slug.
// A refusal prints as the verb's error; a sync that did not start prints beside the result.
func (c *heddleCLI) finishWrite(cmd *cobra.Command, slug string, change func(*heddleengine.Store) error) error {
	c.syncWarning = ""
	if err := c.write(change); err != nil {
		return c.fail(cmd, err)
	}
	fields := map[string]any{"slug": slug}
	if c.syncWarning != "" {
		fields["warning"] = c.syncWarning
	}
	clihelp.SetExit(cmd.Context(), output.Ok(c.out, fields))
	return nil
}

// write applies change to the store under the Bolt write lock and saves it, then starts the board's detached Bolt sync once the lock is released.
// It pulls and commits nothing in the foreground and touches no worktree.
// A change the store refuses saves nothing and starts no sync; a sync that fails to start leaves the saved file in place and sets syncWarning, because the board's own write path ignores that failure the same way.
func (c *heddleCLI) write(change func(*heddleengine.Store) error) error {
	if err := c.saveLocked(change); err != nil {
		return err
	}
	if err := c.startSync(); err != nil {
		logger.Warn("heddle: the Bolt sync did not start after the save", "error", err)
		c.syncWarning = fmt.Sprintf("heddle.json is saved but the Bolt sync did not start (%v); way forward: run `lym board sync` to commit and push it", err)
	}
	return nil
}

// saveLocked loads the store, applies change and saves it, all under the Bolt write lock.
func (c *heddleCLI) saveLocked(change func(*heddleengine.Store) error) error {
	boltLock, acquired, err := lock.AcquireWriteLockWithin(filepath.Join(c.boltDir, fabricengine.BoardWriteLockFile), heddleengine.DefaultBoltLockWait)
	if err != nil {
		return fmt.Errorf("heddle: %w", err)
	}
	if !acquired {
		return fmt.Errorf("heddle: the Bolt checkout stayed locked for %s, so nothing was saved; way forward: run the verb again", heddleengine.DefaultBoltLockWait)
	}
	defer func() { _ = boltLock.Release() }()

	store, err := heddleengine.Load(c.boltDir)
	if err != nil {
		return err
	}
	if err := change(&store); err != nil {
		return err
	}
	return c.saveStore(store)
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
