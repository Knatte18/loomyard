// Package warp owns the host↔weft topology for lyx-managed git repositories.
//
// warp is the structural counterpart to the content-focused weft module — named
// for the weaving warp threads, the load-bearing skeleton the weft passes through.
// Where weft owns every git write into the weft repo (config sync, codeguide
// commit/push/pull), warp owns everything that governs which worktrees and branches
// exist and how they pair:
//
//   - clone: bootstrap a new hub (host prime + board passenger + weft prime).
//   - add/remove: create or destroy a dual host+weft worktree pair as an atomic unit.
//   - checkout: coordinated branch switch across host+weft with junction re-point
//     (the correctness gap that motivated the module — raw git checkout desyncs).
//   - reconcile: repair an already-managed pair whose weft side drifted or broke.
//   - status: paired view of every host↔weft worktree with branch, drift, and
//     junction-health fields.
//   - prune: identify and optionally remove stale or orphaned pairs.
//   - cleanup: delete weft branches whose host sibling is gone, gated on codeguide
//     merge-back (never destroy a weft branch with un-merged codeguide content).
//
// # Topology model
//
// Everything warp creates is dormant: warp add produces a paired but junction-less
// worktree. Junction wiring is deferred to lyx init, which is run in the working
// subdirectory the developer wants active. This dormant-pairing / lyx-init-activation
// split is deliberate — a monorepo may activate several subfolders, and warp cannot
// know the cwd at creation time. lyx init is the activator; it calls warp's junction
// primitive (topology) and then configsync (config layer) in that order, so config
// lands in the weft through the already-wired junction.
//
// # Dependency direction
//
// warp sits below the config layer: warp must NOT import initcli or configsync.
// initcli imports warp (calls the junction primitive). configreg imports warp for the
// config template, just as it imports board and weft. This keeps topology below config
// and prevents import cycles.
//
// # Coordinated operations are all-or-nothing
//
// Every warp operation that touches both sides checks preconditions first and rolls
// back the host side when the weft side fails. The pair is always consistent or
// untouched — never half-switched. The rollbackAdd discipline (junction removed before
// weft teardown, to avoid Windows junction-lock hazard) applies to every operation.
//
// # warp.go
//
// warp.go is the cobra Command() entry point and the RunCLI seam. Command() builds
// a parent "warp" command with one subcommand per verb; per-verb flags (--force,
// --apply) are registered as local flags on the subcommands that own them.
package warp

import (
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/spf13/cobra"
)

// Command builds the cobra command tree for the warp module.
//
// The parent command carries no persistent flags or PersistentPreRunE because warp
// has no shared cwd pre-dispatch — only specific verbs resolve the layout. Per-verb
// flags (remove --force, prune --apply, cleanup --apply/--force) are registered as
// local flags on their subcommands. Positional arguments are available as args[0]
// inside each RunE because cobra strips the subcommand token before calling RunE.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "warp",
		Short: "host↔weft coordination",
		Long: `warp manages the host↔weft topology for lyx-managed git repositories.
It owns worktree pairing, coordinated branch switching, and cleanup.`,
		// RunE is set so that bare "lyx warp" lists subcommands and "lyx warp bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
	}

	// clone [--reset] <host-url> <weft-url> [board-url]
	var cloneCmd *cobra.Command
	cloneCmd = &cobra.Command{
		Use:   "clone [--reset] <host-url> <weft-url> [board-url]",
		Short: "bootstrap a new hub (host prime + board passenger + weft prime)",
		Long: `Clone three repositories into a new hub directory (<parent>/<host-name>-HUB):

  <host-name>      — host prime (the main working repo)
  <host-name>-weft — weft prime (lyx artefacts: config, codeguide, weft commits)
  _board           — board passenger (task-tracker wiki)

The board URL defaults to <weft-url>.wiki.git when omitted.
Use --reset to tear down an existing hub before cloning (idempotent re-clone).

After cloning, run "lyx init" inside the host worktree to activate junctions and config.

Example:
  lyx warp clone https://github.com/user/repo https://github.com/user/repo-weft`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			// Read the --reset flag from the cobra flag set via closure over cloneCmd.
			reset, _ := cloneCmd.Flags().GetBool("reset")
			return runCloneWithReset(out, args, reset)
		}),
	}
	cloneCmd.Flags().Bool("reset", false, "remove an existing hub before cloning (idempotent re-clone)")
	cmd.AddCommand(cloneCmd)

	// add <slug>
	cmd.AddCommand(&cobra.Command{
		Use:   "add <slug>",
		Short: "create a dual host+weft worktree pair",
		RunE:  clihelp.WrapRun(runAdd),
	})

	// list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "list all host↔weft worktree pairs",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runList(out, args) }),
	})

	// remove [--force] <slug>
	var removeCmd *cobra.Command
	removeCmd = &cobra.Command{
		Use:   "remove [--force] <slug>",
		Short: "destroy a dual host+weft worktree pair",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			// The --force flag is read from the cobra flag set via closure over removeCmd.
			force, _ := removeCmd.Flags().GetBool("force")
			return runRemoveWithFlag(out, args, force)
		}),
	}
	removeCmd.Flags().Bool("force", false, "forcefully remove worktree with uncommitted changes")
	cmd.AddCommand(removeCmd)

	// checkout <branch>
	cmd.AddCommand(&cobra.Command{
		Use:   "checkout <branch>",
		Short: "coordinated branch switch across host+weft with junction re-point",
		RunE:  clihelp.WrapRun(runCheckout),
	})

	// status
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "show paired host↔weft worktree status with drift and junction-health fields",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runStatus(out, args) }),
	})

	// reconcile
	cmd.AddCommand(&cobra.Command{
		Use:   "reconcile",
		Short: "repair a managed pair whose weft side drifted or broke",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runReconcile(out, args) }),
	})

	// prune [--apply]
	var pruneCmd *cobra.Command
	pruneCmd = &cobra.Command{
		Use:   "prune [--apply]",
		Short: "identify and optionally remove stale or orphaned host↔weft pairs",
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			apply, _ := pruneCmd.Flags().GetBool("apply")
			return runPruneWithFlag(out, apply)
		}),
	}
	pruneCmd.Flags().Bool("apply", false, "remove stale weft worktrees (default is dry-run/report)")
	cmd.AddCommand(pruneCmd)

	// cleanup [--apply] [--force]
	var cleanupCmd *cobra.Command
	cleanupCmd = &cobra.Command{
		Use:   "cleanup [--apply] [--force]",
		Short: "delete weft branches whose host sibling is gone",
		Long: `cleanup finds weft branches with no corresponding host worktree sibling.

Flag matrix:
  (no flags)          dry-run: report orphaned weft branches only.
  --apply             delete non-gate-protected orphan branches.
  --apply --force     also delete gate-protected task branches.
  --force (alone)     report only; --force does not imply --apply.`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			apply, _ := cleanupCmd.Flags().GetBool("apply")
			force, _ := cleanupCmd.Flags().GetBool("force")
			return runCleanupWithFlags(out, apply, force)
		}),
	}
	cleanupCmd.Flags().Bool("apply", false, "delete non-gate-protected orphaned weft branches")
	cleanupCmd.Flags().Bool("force", false, "also delete gate-protected task branches (requires --apply)")
	cmd.AddCommand(cleanupCmd)

	return cmd
}

// RunCLI is the public seam for the warp module.
//
// It delegates to clihelp.Execute(Command(), out, args) so in-process tests can
// capture all output via a single io.Writer. Returns the exit code (0 on success,
// 1 on cobra-level error such as unknown command or bad flag).
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runAdd parses and executes the warp add subcommand.
// Under cobra, args[0] is the slug (cobra has already stripped the "add" token).
func runAdd(out io.Writer, args []string) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp add <slug>")
	}

	// args[0] is the slug; cobra has already consumed "add" from the argument list.
	slug := args[0]
	r, err := w.Add(l, slug, addOptionsFromEnv())
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"slug":   r.Slug,
		"branch": r.Branch,
		"path":   r.Path,
		"pushed": r.Pushed,
	})
}

// runList parses and executes the warp list subcommand.
func runList(out io.Writer, _ []string) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	_, err = paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	entries, err := w.List(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"worktrees": entries,
	})
}

// runCheckout parses and executes the warp checkout subcommand.
//
// It resolves the layout and warp config from the current working directory,
// then calls Checkout with the supplied branch argument. When no branch is
// supplied (e.g. when invoked from the warp-checkout.cmd launcher shortcut),
// the current host branch is resolved via git and used as the target — this
// performs an in-place re-checkout that re-points junctions and re-syncs the
// weft side without requiring the user to supply a branch name. On success it
// emits a JSON object with branch and weft_worktree fields.
func runCheckout(out io.Writer, args []string) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Resolve the branch: use the supplied argument when present; otherwise
	// derive it from the current host HEAD so the launcher shortcut (which
	// emits no branch argument) performs a valid in-place re-checkout.
	// Under cobra, args[0] is the branch (cobra stripped the "checkout" token).
	var branch string
	if len(args) >= 1 {
		branch = args[0]
	} else {
		branchOut, _, exitCode, runErr := gitexec.RunGit(
			[]string{"branch", "--show-current"},
			l.WorktreeRoot,
		)
		if runErr != nil {
			return output.Err(out, runErr.Error())
		}
		if exitCode != 0 {
			return output.Err(out, "usage: lyx warp checkout <branch>")
		}
		branch = strings.TrimSpace(branchOut)
		if branch == "" {
			// Detached HEAD — cannot resolve a branch to re-checkout.
			return output.Err(out, "usage: lyx warp checkout <branch>")
		}
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	r, err := w.Checkout(l, branch)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"branch":        r.Branch,
		"weft_worktree": r.WeftWorktree,
	})
}

// runStatus parses and executes the warp status subcommand.
//
// Resolves the layout and warp config from the current working directory,
// calls Status to enumerate all host↔weft pairs with drift and pollution data,
// and emits the result via output.Ok.
func runStatus(out io.Writer, _ []string) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	r, err := w.Status(l)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"pairs": r.Pairs,
	})
}

// runReconcile parses and executes the warp reconcile subcommand.
//
// Resolves the layout and warp config from the current working directory,
// calls Reconcile to walk and repair all host↔weft pairs, and emits the
// result via output.Ok.
func runReconcile(out io.Writer, _ []string) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	r, err := w.Reconcile(l)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"pairs": r.Pairs,
	})
}

// runPruneWithFlag executes the prune logic with the resolved apply flag.
// It is called from the pruneCmd RunE after reading --apply from the cobra flag set.
func runPruneWithFlag(out io.Writer, apply bool) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	r, err := w.Prune(l, apply)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"entries": r.Entries,
	})
}

// runCleanupWithFlags executes the cleanup logic with the resolved apply and force flags.
// It is called from the cleanupCmd RunE after reading --apply and --force from the cobra flag set.
func runCleanupWithFlags(out io.Writer, apply, force bool) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	r, err := w.Cleanup(l, apply, force)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"entries": r.Entries,
	})
}

// runRemoveWithFlag executes the remove logic with the resolved force flag.
// It is called from the removeCmd RunE after reading --force from the cobra flag set.
// Under cobra, args[0] is the slug (cobra has already consumed "remove" from the list).
func runRemoveWithFlag(out io.Writer, args []string, force bool) int {
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := LoadConfig(cwd, "warp")
	if err != nil {
		return output.Err(out, err.Error())
	}

	w := New(cfg)

	// args[0] is the slug; cobra has already consumed "remove" from the argument list.
	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp remove [--force] <slug>")
	}
	slug := args[0]

	r, err := w.Remove(l, slug, force)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"slug":          r.Slug,
		"path":          r.Path,
		"links_removed": r.LinksRemoved,
	})
}
