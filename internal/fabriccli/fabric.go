// fabric.go is the cobra Command() entry point and the RunCLI seam for the fabric
// module. It builds the "fabric" parent command and its hub-scoped topology verbs
// (add, list, remove, checkout, pairs, reconcile, prune, cleanup), each driving
// fabricengine.Topology for the host↔weft worktree pairing. The weft-git
// content-sync verbs (status, commit, push, pull, sync, diff) are wired in by
// weft_verbs.go, which also extends this file's Command() build with the
// --weft-path bypass flag and its PersistentPreRunE.

// Package fabriccli owns the unified host↔weft cobra surface for lyx: the flat
// 16-verb "lyx fabric" tree combining host↔weft topology verbs and weft
// content-sync verbs over the fabricengine package. fabric is the sole
// host↔weft git-coordination module (see docs/overview.md). Every fabric weft
// branch carries the uniform "-weft" suffix (fabricengine.WeftBranchName).
package fabriccli

import (
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/weftname"
	"github.com/spf13/cobra"
)

// Command builds the cobra command tree for the fabric module. The parent
// command carries no persistent flags for topology verbs; each resolves its own
// layout and config. weft_verbs.go extends the command with weft-git verbs and
// their scoped PersistentPreRunE.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fabric",
		Short: "unified host↔weft git-coordination",
		Long: `fabric manages the host↔weft topology and weft content-sync for lyx-managed
git repositories, unifying worktree pairing with commit/push/pull.

It owns worktree pairing, coordinated branch switching with junction re-point,
reconcile/prune/cleanup of managed pairs, and weft-side status/commit/push/pull/
sync, all under one module.

Branch scheme: every fabric weft branch is named after the paired host branch
plus a fixed suffix (e.g. host branch "wt-foo" pairs with weft branch
"wt-foo` + weftname.Suffix + `") — uniform for every pair, including the
clone-time primary.

fabric is the sole host↔weft git-coordination module. See docs/overview.md.

Example:
  lyx fabric add my-task
  lyx fabric checkout my-task`,
		RunE: clihelp.GroupRunE,
	}

	// clone [--reset] <host-url> <weft-url> [board-url]
	var cloneCmd *cobra.Command
	cloneCmd = &cobra.Command{
		Use:   "clone [--reset] [--subpath <rel>] <host-url> <weft-url>",
		Short: "bootstrap a new hub, wiring the entire topology in one shot",
		Long: `Clone two repositories into a new hub directory (<parent>/<host-name>-HUB)
and wire everything: the host prime, weft prime, _board worktree, lyx-anchor
subpath, repo-wide config, host junctions, .gitignore, and per-worktree module
configs — a single command, no follow-up activation step required.

  <host-name>            — host prime (the main working repo)
  <host-name>` + weftname.Suffix + `       — weft prime (lyx artefacts: config, raddle, weft commits)

Use --reset to tear down an existing hub before cloning (idempotent re-clone).

Use --subpath <rel> (default ".") to anchor lyx at a subdirectory of the host
repo instead of its root — e.g. --subpath backend for a monorepo where lyx
only manages the backend/ tree. On a re-clone, the previously recorded
subpath is adopted from weft:main; an explicit --subpath that disagrees with
it is a hard error.

The weft prime is immediately checked out onto its suffixed pairing (e.g.
"main` + weftname.Suffix + `" for default branch "main") — fabric's
uniform branch scheme applies from the very first pair. When the weft remote
already carries that suffixed branch (a re-clone of a hub with synced weft
history), it is adopted as a tracking branch, inheriting the existing weft
state; otherwise the branch is created fresh at the cloned HEAD. The cloned
default branch itself remains, unclaimed.

_board is then materialized as a second worktree of the weft repo, on the
host's unsuffixed default branch — adopted if the weft remote already carries
board history, freshly orphan-created otherwise.

Clone wires everything automatically — no follow-up command is needed to
activate junctions or config.

Example:
  lyx fabric clone https://github.com/user/repo https://github.com/user/repo-weft
  lyx fabric clone --subpath backend https://github.com/user/mono https://github.com/user/mono-weft`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			reset, _ := cloneCmd.Flags().GetBool("reset")
			subpath, _ := cloneCmd.Flags().GetString("subpath")
			return runCloneWithReset(out, args, reset, subpath)
		}),
	}
	cloneCmd.Flags().Bool("reset", false, "remove an existing hub before cloning (idempotent re-clone)")
	cloneCmd.Flags().String("subpath", ".", "anchor lyx at this subdirectory of the host repo")
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
		Long: `Remove a paired host and weft git worktree, plus every host junction
(_lyx and _pattern), portal junctions, and launchers.

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
		Long: `Show every host↔weft pair's branch, in-sync verdict, junction health, and
host-pollution scan.

junction_healthy and junction_reason cover BOTH host junctions (_lyx and
_pattern): a pair is only healthy when every junction resolves to its own
weft directory, and junction_reason names the first unhealthy one by name
when it is not. The pollution scan likewise covers _lyx, _pattern, and
_raddle paths accidentally tracked in the host index; _lyx and _pattern
matches carry an automated git rm --cached remedy, _raddle matches are
report-only.`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runPairs(out, args) }),
	})

	// reconcile
	cmd.AddCommand(&cobra.Command{
		Use:   "reconcile",
		Short: "repair a managed pair whose weft side drifted or broke",
		Long: `Reconcile walks every host worktree and applies the minimal corrective
action needed to restore a valid paired topology: recreate a missing weft
worktree, re-point a broken junction, adopt a raw (non-lyx) host worktree, or
report an unmanaged branch untouched.

Junction repair covers BOTH host junctions (_lyx and _pattern): if either is
missing, not a link, or points elsewhere, this re-wires every junction for
that pair in one call — a pair with only one junction broken is repaired,
not reported already-healthy. This is also the upgrade path for a worktree
wired before the _pattern junction existed: one reconcile call adds the
missing junction and materialises its weft-side target.`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runReconcile(out, args) }),
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

A weft branch currently checked out at a worktree is always reported as
protected and never deleted, in every mode — git cannot delete a checked-out
branch, and its being checked out means the pair is still on disk.

The weft repo may also hold weft branches without the fabric suffix (e.g.
inherited from history predating fabric's uniform naming scheme); those are
reported but never deleted here, since they are not fabric-managed.`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			apply, _ := cleanupCmd.Flags().GetBool("apply")
			force, _ := cleanupCmd.Flags().GetBool("force")
			return runCleanupWithFlags(out, apply, force)
		}),
	}
	cleanupCmd.Flags().Bool("apply", false, "delete non-gate-protected orphaned weft branches")
	cleanupCmd.Flags().Bool("force", false, "also delete gate-protected task branches (requires --apply)")
	cmd.AddCommand(cleanupCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "unwire",
		Short: "fully deactivate fabric wiring for this worktree",
		Long: `unwire is a full per-host-worktree deactivation: it removes every host
junction (_lyx and _pattern), clears the weft-side _lyx content, and reverts
the managed .gitignore ".lyx/" entry.

This is distinct from "lyx fabric reconcile", which converges wiring toward
the repo-wide pathspec (adding or re-pointing junctions as needed); unwire
always tears wiring down. It leaves the repo's anchor and repo-wide config
(.lyx-anchor, fabric.yaml on weft:main) intact, so a later
"lyx fabric reconcile" can re-wire this worktree.

Example:
  lyx fabric unwire`,
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runUnwire(out, args) }),
	})

	// Wire the weft-git content-sync verbs (status/commit/push/pull/sync), their
	// own --weft-path bypass flag, and their scoped PersistentPreRunE.
	addWeftVerbs(cmd)

	return cmd
}

// RunCLI is the public seam for the fabric module. It delegates to
// clihelp.Execute, allowing in-process tests to capture output. Returns the
// exit code (0 on success, 1 on error).
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runAdd executes the fabric add subcommand. Under cobra, args[0] is the slug.
func runAdd(out io.Writer, args []string) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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

// runCheckout executes the fabric checkout subcommand. When no branch is
// supplied, it resolves the current host branch and performs an in-place
// re-checkout, re-pointing junctions and re-syncing weft.
func runCheckout(out io.Writer, args []string) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	var branch string
	if len(args) >= 1 {
		branch = args[0]
	} else {
		branchOut, _, exitCode, runErr := gitexec.RunGit(
			[]string{"branch", "--show-current"},
			l.WorktreePath(),
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

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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

// runPairs executes the fabric pairs subcommand, enumerating all host↔weft
// pairs with drift and pollution data.
func runPairs(out io.Writer, _ []string) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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

// runReconcile executes the fabric reconcile subcommand, walking and repairing
// all host↔weft pairs.
func runReconcile(out io.Writer, _ []string) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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
func runPruneWithFlag(out io.Writer, apply bool) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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

// runCleanupWithFlags executes the cleanup logic with the resolved apply and
// force flags.
func runCleanupWithFlags(out io.Writer, apply, force bool) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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
func runRemoveWithFlag(out io.Writer, args []string, force bool) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
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

// addOptionsFromEnv returns the AddOptions for a CLI-driven `lyx fabric add`,
// always returning the zero value. The add subcommand always pushes both sides;
// bypass gates do not apply.
func addOptionsFromEnv() fabricengine.AddOptions {
	return fabricengine.AddOptions{}
}
