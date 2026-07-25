// fabric.go is the cobra Command() entry point and the RunCLI seam for the fabric
// module. It builds the "fabric" parent command and its hub-scoped topology verbs
// (add, list, remove, checkout, pairs, reconcile, prune, cleanup) — each mirroring
// its warpcli counterpart one-to-one, but driving fabricengine.Topology instead of
// warpengine.Worktree. The weft-git content-sync verbs (status, commit, push, pull,
// sync) are wired in by weft_verbs.go, which also extends this file's Command()
// build with the --weft-path bypass flag and its PersistentPreRunE.

// Package fabriccli owns the unified host↔weft cobra surface for lyx: the flat
// 14-verb "lyx fabric" tree combining warp's topology verbs and weft's content-sync
// verbs over the fabricengine package. fabric is built and registered ALONGSIDE
// warp and weft during the parallel-build period (see docs/overview.md); it is not
// yet the default and does not replace either module. Every fabric weft branch
// carries the uniform "-weft" suffix (fabricengine.WeftBranchName), unlike warp's
// mirrored (identical) branch names.
package fabriccli

import (
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command builds the cobra command tree for the fabric module.
//
// The parent command carries no persistent flags or PersistentPreRunE for the
// topology verbs built here — like warp, fabric's topology verbs have no shared
// cwd pre-dispatch and each resolves its own layout and config. weft_verbs.go
// extends the returned command with the weft-git verbs, their own hidden
// --weft-path persistent flag, and a PersistentPreRunE scoped to those verbs only.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fabric",
		Short: "unified host↔weft git-coordination (parallel build alongside warp/weft)",
		Long: `fabric manages the host↔weft topology and weft content-sync for lyx-managed
git repositories, unifying warp's worktree pairing and weft's commit/push/pull.

It owns worktree pairing, coordinated branch switching with junction re-point,
reconcile/prune/cleanup of managed pairs, and weft-side status/commit/push/pull/
sync — the same operations warp and weft provide today, under one module.

Branch scheme: every fabric weft branch is named after the paired host branch
plus a fixed suffix (e.g. host branch "wt-foo" pairs with weft branch
"wt-foo` + hubgeometry.WeftSuffix + `") — uniform for every pair, including the
clone-time primary, unlike warp's mirrored (identical) branch names.

fabric exists alongside warp and weft during the parallel-build period; it is
not yet the default. See docs/overview.md and manifest/designs/fabric.md.

Example:
  lyx fabric add my-task
  lyx fabric checkout my-task`,
		// RunE is set so that bare "lyx fabric" lists subcommands and "lyx fabric bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
	}

	// clone [--reset] <host-url> <weft-url> [board-url]
	var cloneCmd *cobra.Command
	cloneCmd = &cobra.Command{
		Use:   "clone [--reset] <host-url> <weft-url> [board-url]",
		Short: "bootstrap a new hub (host prime + board passenger + weft prime)",
		Long: `Clone three repositories into a new hub directory (<parent>/<host-name>-HUB):

  <host-name>            — host prime (the main working repo)
  <host-name>` + hubgeometry.WeftSuffix + `       — weft prime (lyx artefacts: config, raddle, weft commits)
  _board                 — board passenger (task-tracker wiki)

The board URL defaults to <weft-url>.wiki.git when omitted.
Use --reset to tear down an existing hub before cloning (idempotent re-clone).

Unlike "lyx warp clone", the weft prime's checked-out branch is immediately
renamed onto its suffixed pairing (e.g. "main" -> "main` + hubgeometry.WeftSuffix + `") —
fabric's uniform branch scheme applies from the very first pair.

After cloning, run "lyx init" inside the host worktree to activate junctions and config.

Example:
  lyx fabric clone https://github.com/user/repo https://github.com/user/repo-weft`,
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
		Long: `Create a new paired host and weft git worktree for the given slug.

The new weft branch is forked from the HEAD of the worktree you run
"lyx fabric add" from — that worktree's current checked-out branch, not main
and not prime's branch. This makes the new pair an exact continuation of
the context you were working in. The weft branch name is always the host
branch's name with fabric's uniform suffix appended.

The command errors if the worktree is on a detached HEAD or an unborn branch,
because a fork point cannot be determined in either case.

Example:
  lyx fabric add my-task`,
		RunE: clihelp.WrapRun(runAdd),
	})

	// list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "list host worktrees (use 'lyx fabric pairs' for full pair geometry)",
		Long: `List all host worktrees registered in the current hub.

This command outputs host worktree paths only. For the full host↔weft pair
geometry view — including weft pairing, branch drift, and junction health —
use "lyx fabric pairs".`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runList(out, args) }),
	})

	// remove [--force] <slug>
	var removeCmd *cobra.Command
	removeCmd = &cobra.Command{
		Use:   "remove [--force] <slug>",
		Short: "destroy a dual host+weft worktree pair",
		Long: `Remove a paired host and weft git worktree, plus all associated portal
junctions and launchers.

By default the command refuses to remove a worktree with uncommitted changes
on either the host or weft side. Use --force to remove anyway.

Example:
  lyx fabric remove my-task
  lyx fabric remove --force my-task`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			// The --force flag is read from the cobra flag set via closure over removeCmd.
			force, _ := removeCmd.Flags().GetBool("force")
			return runRemoveWithFlag(out, args, force)
		}),
	}
	removeCmd.Flags().Bool("force", false, "forcefully remove worktree with uncommitted changes")
	cmd.AddCommand(removeCmd)

	// checkout [branch]
	cmd.AddCommand(&cobra.Command{
		Use:   "checkout [branch]",
		Short: "coordinated branch switch across host+weft with junction re-point",
		Long: `Switch the host worktree to <branch> and its weft sibling to the
suffix-paired weft branch, re-pointing junctions in the same operation.

When no branch is given, the current host branch is re-resolved and used as
the target — this performs an in-place re-checkout that re-points junctions
and re-syncs the weft side, which is how the fabric-checkout launcher
shortcut invokes this command.

The switch is all-or-nothing: on any weft-side or junction failure the host
switch is rolled back so the pair is never left half-switched.

Example:
  lyx fabric checkout my-branch`,
		RunE: clihelp.WrapRun(runCheckout),
	})

	// pairs
	cmd.AddCommand(&cobra.Command{
		Use:   "pairs",
		Short: "show full host↔weft pair geometry with drift and junction-health fields",
		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runPairs(out, args) }),
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
  --force (alone)     report only; --force does not imply --apply.

During the parallel-build period the weft repo also holds warp-created weft
branches (mirrored names, no fabric suffix); those are reported but never
deleted here, since they are not fabric-managed.`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			apply, _ := cleanupCmd.Flags().GetBool("apply")
			force, _ := cleanupCmd.Flags().GetBool("force")
			return runCleanupWithFlags(out, apply, force)
		}),
	}
	cleanupCmd.Flags().Bool("apply", false, "delete non-gate-protected orphaned weft branches")
	cleanupCmd.Flags().Bool("force", false, "also delete gate-protected task branches (requires --apply)")
	cmd.AddCommand(cleanupCmd)

	// Wire the weft-git content-sync verbs (status/commit/push/pull/sync), their
	// own --weft-path bypass flag, and their scoped PersistentPreRunE.
	addWeftVerbs(cmd)

	return cmd
}

// RunCLI is the public seam for the fabric module.
//
// It delegates to clihelp.Execute(Command(), out, args) so in-process tests can
// capture all output via a single io.Writer. Returns the exit code (0 on success,
// 1 on cobra-level error such as unknown command or bad flag).
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runAdd parses and executes the fabric add subcommand.
// Under cobra, args[0] is the slug (cobra has already stripped the "add" token).
func runAdd(out io.Writer, args []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	if len(args) < 1 {
		return output.Err(out, "usage: lyx fabric add <slug>")
	}

	// args[0] is the slug; cobra has already consumed "add" from the argument list.
	slug := args[0]
	r, err := top.Add(l, slug, addOptionsFromEnv())
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

// runList parses and executes the fabric list subcommand.
func runList(out io.Writer, _ []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	_, err = hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	entries, err := top.List(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"worktrees": entries,
	})
}

// runCheckout parses and executes the fabric checkout subcommand.
//
// It resolves the layout and fabric config from the current working directory,
// then calls Checkout with the supplied branch argument. When no branch is
// supplied (e.g. when invoked from the fabric-checkout launcher shortcut),
// the current host branch is resolved via git and used as the target — this
// performs an in-place re-checkout that re-points junctions and re-syncs the
// weft side without requiring the user to supply a branch name. On success it
// emits a JSON object with branch and weft_worktree fields.
func runCheckout(out io.Writer, args []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
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
			return output.Err(out, "usage: lyx fabric checkout <branch>")
		}
		branch = strings.TrimSpace(branchOut)
		if branch == "" {
			// Detached HEAD — cannot resolve a branch to re-checkout.
			return output.Err(out, "usage: lyx fabric checkout <branch>")
		}
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Checkout(l, branch)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"branch":        r.Branch,
		"weft_worktree": r.WeftWorktree,
	})
}

// runPairs parses and executes the fabric pairs subcommand.
//
// Resolves the layout and fabric config from the current working directory,
// calls Status to enumerate all host↔weft pairs with drift and pollution data,
// and emits the result via output.Ok.
func runPairs(out io.Writer, _ []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Status(l)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"pairs": r.Pairs,
	})
}

// runReconcile parses and executes the fabric reconcile subcommand.
//
// Resolves the layout and fabric config from the current working directory,
// calls Reconcile to walk and repair all host↔weft pairs, and emits the
// result via output.Ok.
func runReconcile(out io.Writer, _ []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Reconcile(l)
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
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Prune(l, apply)
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
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Cleanup(l, apply, force)
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
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	// args[0] is the slug; cobra has already consumed "remove" from the argument list.
	if len(args) < 1 {
		return output.Err(out, "usage: lyx fabric remove [--force] <slug>")
	}
	slug := args[0]

	r, err := top.Remove(l, slug, force)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"slug":          r.Slug,
		"path":          r.Path,
		"links_removed": r.LinksRemoved,
	})
}

// addOptionsFromEnv populates AddOptions from environment variables at the CLI edge.
// Tests pass AddOptions directly to avoid relying on environment state.
func addOptionsFromEnv() fabricengine.AddOptions {
	return fabricengine.AddOptions{}
}
