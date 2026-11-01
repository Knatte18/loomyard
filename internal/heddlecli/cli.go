// cli.go builds the heddle cobra command with its pre-run that resolves the hub, and the RunCLI/RunCLIIn seams that wire it into the io.Writer-based call contract.
// The verbs live in verbs.go.

package heddlecli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// groupName is the command name of the module's group.
const groupName = "heddle"

// heddleCLI holds what the verbs share once the pre-run has resolved the hub.
// A nil save means heddleengine.Save, a nil spawnSync means boardengine.SpawnSync and a zero renderer waits heddleengine.DefaultBoltLockWait, so a receiver a test builds by hand behaves as Command()'s does.
type heddleCLI struct {
	location *lymcwd.Location
	boltDir  string
	out      io.Writer
	renderer heddleengine.Renderer
	// save writes the store into the Bolt checkout.
	save func(boltDir string, store heddleengine.Store) error
	// spawnSync starts the board's detached Bolt sync of the checkout.
	spawnSync func(boltDir string) error
	// syncWarning is the text a failed sync start left behind for the verb to print beside its result.
	syncWarning string
}

// Command returns the cobra command for the heddle module.
func Command() *cobra.Command {
	return commandOf(&heddleCLI{})
}

// commandOf builds the heddle command tree over c, so a test can hand it a receiver with its seams replaced.
func commandOf(c *heddleCLI) *cobra.Command {
	heddle := &cobra.Command{
		Use:   groupName,
		Short: "list, set, remove, show and render the instruction entries heddle stores for the hub",
		Long: `heddle delivers the instruction text agents get from two layers: the entries
lym ships in its binary, read-only, and the repository's own in heddle.json in
the hub's Bolt checkout. It renders file targets into git-excluded files at a
worktree's root, the shipped entries first, and it appends prompt targets to
the system prompt of the Claude sessions launched in the hub: claude-prompt to
every one, orch-prompt to an orch session alone. A hub whose heddle.yaml sets
lym_override may override a shipped entry's text with set --layer lym.

set and remove change the store under the Bolt write lock and start the board's
detached Bolt sync, which commits and pushes the saved file. A worktree takes a
change only when lym makes the worktree, at a take-in render or at a hand
render; a Claude session takes it at its launch or adoption, or at its next
compaction after a file render. Every verb runs from a code worktree of a hub.`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}
	heddle.AddCommand(c.listCmd(), c.getCmd(), c.setCmd(), c.removeCmd(), c.showCmd(), c.renderCmd())
	return heddle
}

// resolvePersistentPreRun resolves the hub for a verb and stores it on c.
// The group itself resolves nothing, so a bare listing or an unknown-subcommand error needs no hub.
func (c *heddleCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == groupName {
		return nil
	}
	ctx := cmd.Context()
	c.out = cmd.OutOrStdout()
	cwd, err := lymcwd.CwdFrom(ctx)
	if err == nil {
		err = c.wire(cwd)
	}
	if err != nil {
		clihelp.Abort(ctx, output.Err(c.out, err.Error()))
		return nil
	}
	return nil
}

// wire resolves the hub of cwd into c's location and Bolt checkout.
// A cwd that does not resolve is refused with the way forward, a worktree fabric does not drive keeps fabric's own text, and a hub without a Bolt checkout is refused naming the clone that creates one.
func (c *heddleCLI) wire(cwd string) error {
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return fmt.Errorf("heddle: %w; way forward: run lym heddle from a code worktree of a hub, such as its prime", err)
	}
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return err
	}
	boltDir := fabricengine.BoltDir(location.HubPath)
	if info, err := os.Stat(boltDir); err != nil || !info.IsDir() {
		return fmt.Errorf("heddle: the hub's Bolt checkout %s is missing; way forward: lym heddle runs only in a hub `lym fabric clone` made, which creates one", boltDir)
	}
	c.location = location
	c.boltDir = boltDir
	return nil
}

// namedWorktree returns the location of the worktree of c's hub that name is the last path element of.
// A name matching no listed worktree, or only one git marks prunable, is refused, so the render writes and seeds only in an existing worktree of its own hub.
func (c *heddleCLI) namedWorktree(name string) (*lymcwd.Location, error) {
	entries, err := fabricengine.List(c.location.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("heddle: list the hub's worktrees: %w", err)
	}
	prunable := false
	for _, entry := range entries {
		if filepath.Base(filepath.FromSlash(entry.Path)) != name {
			continue
		}
		if !entry.Prunable {
			return lymcwd.NamedWorktree(c.location.HubPath, name)
		}
		prunable = true
	}
	if prunable {
		return nil, fmt.Errorf("heddle: worktree %q is listed but its directory is gone; way forward: run `git worktree prune` in the hub's prime to clear the stale entry, and see `lym fabric list`", name)
	}
	return nil, fmt.Errorf("heddle: no worktree named %q in this hub; way forward: pick the last element of a path `lym fabric list` prints", name)
}

// RunCLI is the public seam for the heddle module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// ChangeStore applies change to the store of the hub cwd lies in, the way `lym heddle set` and `remove` write: under the Bolt write lock, followed by the board's detached Bolt sync.
// It lets another module's verb write heddle entries of its own through heddle's one write path.
// A cwd that is no drivable worktree of a hub, or a change the store refuses, returns the error and saves nothing;
// a sync that did not start after the save returns a warning naming `lym board sync`, with no error.
func ChangeStore(cwd string, change func(*heddleengine.Store) error) (warning string, err error) {
	return (&heddleCLI{}).changeStore(cwd, change)
}

// changeStore is ChangeStore over c's seams.
func (c *heddleCLI) changeStore(cwd string, change func(*heddleengine.Store) error) (warning string, err error) {
	if err := c.wire(cwd); err != nil {
		return "", err
	}
	if err := c.write(c.repositoryFile(), change); err != nil {
		return "", err
	}
	return c.syncWarning, nil
}

// saveStore writes the store through c's save seam.
func (c *heddleCLI) saveStore(store heddleengine.Store) error {
	if c.save != nil {
		return c.save(c.boltDir, store)
	}
	return heddleengine.Save(c.boltDir, store)
}

// startSync starts the Bolt sync through c's spawnSync seam.
func (c *heddleCLI) startSync() error {
	if c.spawnSync != nil {
		return c.spawnSync(c.boltDir)
	}
	return boardengine.SpawnSync(c.boltDir)
}
