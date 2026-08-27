// fabric.go is the cobra Command() entry point and the RunCLI seam for the fabric module.
// It builds the "fabric" parent command and its hub-scoped topology verbs (add, list, remove,
// checkout, pairs, reconcile, prune, cleanup), each driving fabricengine.Topology for the warp↔weft
// worktree pairing.
// The weft-git content-sync verbs (status, commit, push, pull, sync, diff) are wired in by
// weft_verbs.go, which also extends this file's Command() build with the --weft-path bypass flag
// and its PersistentPreRunE.

// Package fabriccli owns the unified warp↔weft cobra surface for lyx: the flat 16-verb "lyx fabric"
// tree combining warp↔weft topology verbs and weft content-sync verbs over the fabricengine
// package.
// fabric is the sole warp↔weft git-coordination module (see docs/overview.md).
// Every fabric weft branch carries the uniform "-weft" suffix (fabricengine.WeftBranchName).
package fabriccli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/weftname"
	"github.com/spf13/cobra"
)

// Command builds the cobra command tree for the fabric module.
// The parent command carries no persistent flags for topology verbs;
// each resolves its own layout and config.
// weft_verbs.go extends the command with weft-git verbs and their scoped PersistentPreRunE.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fabric",
		Short: "unified warp↔weft git-coordination",
		Long: `fabric manages the warp↔weft topology and weft content-sync for lyx-managed
git repositories, unifying worktree pairing with commit/push/pull.

It owns worktree pairing, coordinated branch switching with junction re-point,
reconcile/prune/cleanup of managed pairs, and weft-side status/commit/push/pull/
sync, all under one module.

Branch scheme: every fabric weft branch is named after the paired warp branch
plus a fixed suffix (e.g. warp branch "wt-foo" pairs with weft branch
"wt-foo` + weftname.Suffix + `") — uniform for every pair, including the
clone-time primary.

fabric is the sole warp↔weft git-coordination module. See docs/overview.md.

Example:
  lyx fabric clone https://github.com/user/repo-weft
  lyx fabric add my-task
  lyx fabric remove my-task`,
		RunE: clihelp.GroupRunE,
	}

	// clone [--reset] [--subpath <rel>] [--force-bootstrap] [--into <dir>] <weft-url> [<warp-url>]
	var cloneCmd *cobra.Command
	cloneCmd = &cobra.Command{
		Use:   "clone [--reset] [--subpath <rel>] [--force-bootstrap] [--into <dir>] <weft-url> [<warp-url>]",
		Short: "bootstrap a new hub, wiring the entire topology in one shot",
		Long: `Clone two repositories into a new hub directory (<parent>/<warp-name>-HUB)
and wire everything: the warp prime, weft prime, _board worktree, lyx-anchor
subpath, repo-wide config, warp junctions, and per-worktree module configs —
a single command, no follow-up activation step required. Warp junctions are
excluded through the warp's .git/info/exclude, never a committed .gitignore.

There are two forms. "lyx fabric clone <weft-url>" derives the warp URL from
the binding recorded on weft:main. "lyx fabric clone <weft-url> <warp-url>"
supplies the warp URL explicitly, which is required the first time a weft is
bound and is a hard error when it disagrees with an existing binding.

The binding itself is a plain single-line record, ` + fabricengine.WarpBindingFileName + `,
kept at the board root and holding the warp URL only. It is committed onto
weft:main beside the recorded lyx-anchor subpath, written the first time a
warp URL is supplied for an unbound weft.

  <warp-name>            — warp prime (the main working repo); in the
                           one-argument form its name is derived from the
                           recorded binding
  <warp-name>` + weftname.Suffix + `       — weft prime (lyx artefacts: config, raddle, weft commits)

Use --reset to tear down an existing hub before cloning (idempotent re-clone).
The teardown is refused unless the target really is a fabric hub — it must hold
a _board entry or a weft sibling. The hub name is derived rather than typed (in
the one-argument form, from the binding recorded on the weft), so a directory
that merely happens to be named <name>-HUB is reported and left alone.

Use --subpath <rel> (default ".") to anchor lyx at a subdirectory of the warp
repo instead of its root — e.g. --subpath backend for a monorepo where lyx
only manages the backend/ tree. It must be a path relative to the warp repo
root that stays inside it: an absolute path, or one escaping via "..", is
refused before anything is cloned. On a re-clone, the previously recorded
subpath is adopted from weft:main; an explicit --subpath that disagrees with
it is a hard error.

Use --force-bootstrap to bypass the weft-candidate guard when bootstrapping a
brand-new weft remote that is neither empty nor already lyx-anchored (for
example one created with an auto-generated README), which the guard would
otherwise refuse. It applies to exactly that situation: it is ignored in the
one-argument form and whenever a binding is already recorded.

Use --into <dir> to name the directory the new hub is created in, instead of
the current working directory. A relative value resolves against the current
working directory; the default, when --into is omitted, is the current
working directory itself.

The weft prime is immediately checked out onto its suffixed pairing (e.g.
"main` + weftname.Suffix + `" for default branch "main") — fabric's
uniform branch scheme applies from the very first pair. When the weft remote
already carries that suffixed branch (a re-clone of a hub with synced weft
history), it is adopted as a tracking branch, inheriting the existing weft
state; otherwise the branch is created fresh at the cloned HEAD. The cloned
default branch itself remains, unclaimed.

_board is then materialized as a second worktree of the weft repo, on the
warp's unsuffixed default branch — adopted if the weft remote already carries
board history, freshly orphan-created otherwise.

Clone wires everything automatically — no follow-up command is needed to
activate junctions or config.

Example:
  lyx fabric clone --subpath backend https://github.com/user/mono-weft https://github.com/user/mono
  lyx fabric clone https://github.com/user/repo-weft
  lyx fabric clone --into ~/repos https://github.com/user/repo-weft`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			reset, _ := cloneCmd.Flags().GetBool("reset")
			subpath, _ := cloneCmd.Flags().GetString("subpath")
			forceBootstrap, _ := cloneCmd.Flags().GetBool("force-bootstrap")
			into, _ := cloneCmd.Flags().GetString("into")
			return runCloneWithReset(ctx, out, args, reset, subpath, forceBootstrap, into)
		}),
	}
	cloneCmd.Flags().Bool("reset", false, "remove an existing hub before cloning (idempotent re-clone)")
	// The default is the EMPTY string, not "." — CloneHub normalises empty to the "." root anchor
	// anyway, and only an empty default lets it tell "the operator typed nothing" apart from "the
	// operator typed --subpath .". With "." as the cobra default the two were identical, so an
	// explicit --subpath . against a hub recorded at a real subpath was silently adopted instead of
	// refused like every other disagreeing value.
	cloneCmd.Flags().String("subpath", "", `anchor lyx at this subdirectory of the warp repo (default ".", the repo root)`)
	cloneCmd.Flags().String("into", "", "directory the new hub is created in; a relative value resolves against the current working directory (default: the current working directory)")
	cloneCmd.Flags().Bool("force-bootstrap", false, "bypass the weft-candidate guard when bootstrapping a brand-new weft remote")
	cmd.AddCommand(cloneCmd)

	// add <slug>
	cmd.AddCommand(&cobra.Command{
		Use:   "add <slug>",
		Args:  cobra.MaximumNArgs(1),
		Short: "create a dual warp+weft worktree pair",
		Long: `Create a new paired warp and weft git worktree for the given slug.

The new weft branch is forked from the HEAD of the worktree you run
"lyx fabric add" from — that worktree's current checked-out branch, not main
and not prime's branch. This makes the new pair an exact continuation of
the context you were working in. The weft branch name is always the warp
branch's name with fabric's uniform suffix appended.

The command errors if the worktree is on a detached HEAD or an unborn branch,
because a fork point cannot be determined in either case.

Example:
  lyx fabric add my-task`,
		RunE: clihelp.WrapRunCtx(runAdd),
	})

	// list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Args:  cobra.NoArgs,
		Short: "list warp worktrees (use 'lyx fabric pairs' for full pair geometry)",
		Long: `List all warp worktrees registered in the current hub.

This command outputs warp worktree paths only. For the full warp↔weft pair
geometry view — including weft pairing, branch drift, and junction health —
use "lyx fabric pairs".`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runList(ctx, out, args) }),
	})

	// remove [--force] <slug>
	var removeCmd *cobra.Command
	removeCmd = &cobra.Command{
		Use:   "remove [--force] <slug>",
		Args:  cobra.MaximumNArgs(1),
		Short: "destroy a dual warp+weft worktree pair",
		Long: `Remove a paired warp and weft git worktree, plus every warp junction
(_lyx, .lyx), portal junctions, and launchers.

By default the command refuses to remove a worktree with uncommitted changes
on either the warp or weft side. Use --force to remove anyway.

<slug> must name a worktree pair, never hub geometry. The hub's prime
worktree (the warp repository itself), the reserved hub entries (_board,
_portals, _launchers, _lyx, .lyx), and any name ending in the weft suffix
are all refused — the same set "lyx fabric add" refuses. When git itself
declines to remove the worktree, fabric reports git's own reason and deletes
nothing unless the target is a registered linked worktree of this repo.

Example:
  lyx fabric remove my-task
  lyx fabric remove --force my-task`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			// The --force flag is read from the cobra flag set via closure over removeCmd.
			force, _ := removeCmd.Flags().GetBool("force")
			return runRemoveWithFlag(ctx, out, args, force)
		}),
	}
	removeCmd.Flags().Bool("force", false, "forcefully remove worktree with uncommitted changes")
	cmd.AddCommand(removeCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "checkout [branch]",
		Args:  cobra.MaximumNArgs(1),
		Short: "coordinated branch switch across warp+weft with junction re-point",
		Long: `Switch the warp worktree to <branch> and its weft sibling to the
suffix-paired weft branch, re-pointing junctions in the same operation.

When no branch is given, the current warp branch is re-resolved and used as
the target — this performs an in-place re-checkout that re-points junctions
and re-syncs the weft side, which is how the fabric-checkout launcher
shortcut invokes this command.

The command refuses before switching anything if the WEFT worktree has
uncommitted tracked changes: a half-switched pair is the one state this verb
must never produce, so commit or stash the weft side first. A dirty WARP
worktree is not refused — git carries those changes across the switch, as it
would for a plain "git switch".

The switch is all-or-nothing: on any weft-side or junction failure the warp
switch is rolled back so the pair is never left half-switched.

Example:
  lyx fabric checkout my-branch`,
		RunE: clihelp.WrapRunCtx(runCheckout),
	})

	// pairs
	cmd.AddCommand(&cobra.Command{
		Use:   "pairs",
		Args:  cobra.NoArgs,
		Short: "show full warp↔weft pair geometry with drift and junction-health fields",
		Long: `Show every warp↔weft pair's branch, in-sync verdict, junction health, and
warp-pollution scan.

junction_healthy and junction_reason cover BOTH warp junctions (_lyx and
.lyx): a pair is only healthy when every junction resolves to its own
weft directory, and junction_reason names the first unhealthy one by name
when it is not. The pollution scan likewise covers _lyx paths accidentally
tracked in the warp index; every match carries an automated git rm --cached
remedy.`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runPairs(ctx, out, args) }),
	})

	// reconcile
	cmd.AddCommand(&cobra.Command{
		Use:   "reconcile",
		Args:  cobra.NoArgs,
		Short: "repair a managed pair whose weft side drifted or broke",
		Long: `Reconcile walks every warp worktree and applies the minimal corrective
action needed to restore a valid paired topology: recreate a missing weft
worktree, re-point a broken junction, adopt a raw (non-lyx) warp worktree, or
report an unmanaged branch untouched.

Junction repair covers BOTH warp junctions (_lyx and .lyx): if either is
missing, not a link, or points elsewhere, this re-wires every junction for
that pair in one call — a pair with only one junction broken is repaired,
not reported already-healthy.

It also restores a pair's hub-level portal junction (_portals/<slug>) and
launcher directory (_launchers/<slug>) when either has gone missing, reporting
portal_restored rather than already_healthy. The hub's prime worktree is
skipped: it never had either, so there is nothing there to repair.`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runReconcile(ctx, out, args) }),
	})

	// prune [--apply]
	var pruneCmd *cobra.Command
	pruneCmd = &cobra.Command{
		Use:   "prune [--apply] [--force]",
		Args:  cobra.NoArgs,
		Short: "identify and optionally remove stale or orphaned warp↔weft pairs",
		Long: `Prune scans for on-disk pair debris in two passes: a registered pair whose
warp worktree directory is gone (stale), and a weft worktree with no warp
sibling at all (orphaned).

By default this is a dry run: every stale or orphaned pair is reported and
nothing is removed. With --apply, each entry's weft worktree is removed, the
dead slug's portal junction and launcher directory are torn down, and stale
worktree registrations are pruned on both repos. Branches are never deleted
here — orphaned weft branches are "lyx fabric cleanup"'s job.

The weft worktree is removed forcefully, so an entry whose weft worktree
still carries uncommitted tracked changes is reported "protected": true and
skipped. Use --force to remove it anyway, discarding those changes. Untracked
files are not a reason to protect an entry — they are the ordinary residue of
an abandoned pair — and they go with the worktree when it is removed.

The orphan pass enumerates by directory NAME alone, so an ordinary directory —
or a wholly unrelated git clone — parked in the hub under a name ending in the
weft suffix is reported too. Such an entry is flagged "unowned": true and is
never removed, in any mode: --force does not apply to it, because the question
it answers is not "is this work worth keeping" but "is this fabric's at all".
Only a path the hub's weft repo registers as a linked worktree is removable.

A dry run computes the same protected and unowned verdicts the matching --apply
run would act on, so "protected": false with no "unowned" in a dry run means
"--apply would remove this".

Example:
  lyx fabric prune
  lyx fabric prune --apply
  lyx fabric prune --apply --force`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			apply, _ := pruneCmd.Flags().GetBool("apply")
			force, _ := pruneCmd.Flags().GetBool("force")
			return runPruneWithFlags(ctx, out, apply, force)
		}),
	}
	pruneCmd.Flags().Bool("apply", false, "remove stale weft worktrees (default is dry-run/report)")
	pruneCmd.Flags().Bool("force", false, "also remove a weft worktree with uncommitted tracked changes")
	cmd.AddCommand(pruneCmd)

	var cleanupCmd *cobra.Command
	cleanupCmd = &cobra.Command{
		Use:   "cleanup [--apply] [--force]",
		Args:  cobra.NoArgs,
		Short: "delete weft branches whose warp sibling is gone",
		Long: `cleanup finds weft branches with no corresponding warp worktree sibling.

Flag matrix:
  (no flags)          dry-run: report orphaned weft branches only.
  --apply             delete every orphan weft branch (primary weft branch
                      and checked-out branches stay protected).
  --force (alone)     report only; --force does not imply --apply, and today
                      answers no cleanup gate.

A dry run reports the same protected verdict the matching --apply run would
act on, so "protected: false" in a dry run means "--apply would delete this".

A weft branch currently checked out at a worktree is always reported as
protected and never deleted, in every mode — git cannot delete a checked-out
branch, and its being checked out means the pair is still on disk.

The repo's primary weft branch (the weft pairing of the branch the hub's
_board worktree is on, e.g. "main-weft") is likewise always protected, in
every mode. It stays the durable weft line however the prime worktree happens
to be checked out, so a coordinated checkout onto another branch must not
promote it to a deletable orphan. If that primary cannot be determined —
a hub with no readable _board worktree — cleanup refuses to enumerate
orphans at all rather than sweep on a guess.

The weft repo may also hold weft branches without the fabric suffix (e.g.
inherited from history predating fabric's uniform naming scheme); those are
reported but never deleted here, since they are not fabric-managed.

Deletion is local to the hub's weft repo: a deleted branch's copy on the
weft remote, if it was ever pushed, is left untouched.`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			apply, _ := cleanupCmd.Flags().GetBool("apply")
			force, _ := cleanupCmd.Flags().GetBool("force")
			return runCleanupWithFlags(ctx, out, apply, force)
		}),
	}
	cleanupCmd.Flags().Bool("apply", false, "delete orphaned weft branches (default is dry-run/report)")
	cleanupCmd.Flags().Bool("force", false, "reserved; answers no cleanup gate today")
	cmd.AddCommand(cleanupCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "unwire",
		Args:  cobra.NoArgs,
		Short: "fully deactivate fabric wiring for this worktree",
		Long: `unwire is a full per-warp-worktree deactivation: it removes every warp
junction present (_lyx, .lyx) and their warp .git/info/exclude entries. It
leaves every weft-side directory intact — weft-side content is never deleted
by unwire.

This is distinct from "lyx fabric reconcile", which converges wiring toward
the repo-wide pathspec (adding or re-pointing junctions as needed); unwire
always tears wiring down. It leaves the repo-wide weft:main records intact
(.lyx-anchor, the .lyx-warp binding, and fabric.yaml), so a later
"lyx fabric reconcile" can re-wire this worktree.

Example:
  lyx fabric unwire`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runUnwire(ctx, out, args) }),
	})

	// Wire the weft-git content-sync verbs (status/commit/push/pull/sync), their
	// own --weft-path bypass flag, and their scoped PersistentPreRunE.
	addWeftVerbs(cmd)

	return cmd
}

// RunCLI is the public seam for the fabric module.
// It delegates to clihelp.Execute, allowing in-process tests to capture output.
// Returns the exit code (0 on success, 1 on error).
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// resolveWarpLocation resolves the seam cwd — the cwd RunCLIIn injected into ctx, or the process
// cwd otherwise — into the acting Location, refusing any cwd that resolves onto something other
// than a warp worktree.
//
// Every topology verb goes through it rather than calling lyxcwd.Resolve directly, because
// lyxcwd cannot make that distinction itself (see fabricengine.RequireWarpWorktree): a cwd inside a
// weft sibling, or inside the hub's own `_board` worktree, otherwise resolves cleanly and drives the
// verb against geometry that does not exist.
// It returns cwd alongside the Location for the verbs that pass cwd straight to a git invocation.
func resolveWarpLocation(ctx context.Context) (cwd string, l *lyxcwd.Location, err error) {
	cwd, err = lyxcwd.CwdFrom(ctx)
	if err != nil {
		return "", nil, err
	}

	l, err = lyxcwd.Resolve(cwd)
	if err != nil {
		// On a gate failure the generic error can actively misdirect: from a weft sibling's
		// NON-anchored directory it names the weft's own anchored directory as the place to stand,
		// where RequireWarpWorktree then refuses anyway.
		// Classify the worktree ungated and prefer the specific weft/board refusal when it applies.
		if worktreeLocation, worktreeErr := lyxcwd.ResolveWorktree(cwd); worktreeErr == nil {
			if refusal := fabricengine.RequireWarpWorktree(worktreeLocation); refusal != nil {
				return "", nil, refusal
			}
		}
		return "", nil, err
	}

	if err := fabricengine.RequireWarpWorktree(l); err != nil {
		return "", nil, err
	}

	return cwd, l, nil
}

// runAdd executes the fabric add subcommand. Under cobra, args[0] is the slug.
func runAdd(ctx context.Context, out io.Writer, args []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
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
		return errWithRecord(out, r.Mutated(), err)
	}
	return okWithRecord(out, r.Mutated(), map[string]any{
		"slug":   r.Slug,
		"branch": r.Branch,
		"path":   r.Path,
		"pushed": r.Pushed,
	})
}

// runList parses and executes the fabric list subcommand.
func runList(ctx context.Context, out io.Writer, _ []string) int {
	cwd, l, err := resolveWarpLocation(ctx)
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
// supplied, it resolves the current warp branch and performs an in-place
// re-checkout, re-pointing junctions and re-syncing weft.
func runCheckout(ctx context.Context, out io.Writer, args []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	var branch string
	if len(args) >= 1 {
		branch = args[0]
	} else {
		branchOut, runErr := gitexec.Run(
			[]string{"branch", "--show-current"},
			l.WorktreePath(),
		)
		if runErr != nil {
			// A *GitError means git ran and rejected the command: recover it as
			// "no current branch to infer", a distinct answer from an exec-level
			// failure, which keeps its own diagnostic below.
			var gitErr *gitexec.GitError
			if errors.As(runErr, &gitErr) {
				return output.Err(out, "usage: lyx fabric checkout <branch>")
			}
			return output.Err(out, runErr.Error())
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
		return errWithRecord(out, r.Mutated(), err)
	}
	return okWithRecord(out, r.Mutated(), map[string]any{
		"branch":        r.Branch,
		"weft_worktree": r.WeftWorktree,
	})
}

// runPairs executes the fabric pairs subcommand, enumerating all warp↔weft
// pairs with drift and pollution data.
func runPairs(ctx context.Context, out io.Writer, _ []string) int {
	_, l, err := resolveWarpLocation(ctx)
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

// runReconcile executes the fabric reconcile subcommand, walking and repairing all warp↔weft pairs.
// Beyond the per-pair repair Topology.Reconcile performs itself, this handler owns the commit-and-push
// half of the once-per-hub warp-URL binding backfill: on a fresh "recorded" outcome it commits the
// written record through Bolt, and on both "recorded" and "present" it attempts a push, so a
// previously committed-but-unpushed record is retried on every subsequent reconcile. Either step
// failing downgrades the reported outcome to WarpBindingOutcomeRecordFailed — a CLI-only value
// Topology.Reconcile itself never returns — but never the exit code: a failed backfill commit or push
// is non-fatal, mirroring the board-junction-wiring precedent that a convenience repair may never
// downgrade a reconcile verdict.
func runReconcile(ctx context.Context, out io.Writer, _ []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// The recorder is built here, as soon as l resolves, and not after top.Reconcile(l) returns:
	// configsync.ReconcileFabricAt below runs before top.Reconcile in this handler and may already
	// have written a file, so seeding from r.Mutated() first would misstate the array's order — array
	// order is the only thing carrying ordering in this vocabulary. This is the one handler where
	// "pre-flight" and "pre-mutation" come apart: none of the three output.Err sites below qualifies
	// for the ordinary pre-flight carve-out, since ReconcileFabricAt may already have mutated state by
	// the time any of them is reached.
	rec := fabricengine.NewMutations(l.HubPath)

	// Reconcile is the repair verb, so a missing repo-wide fabric config is healed here rather
	// than reported: without this, LoadConfig's "not initialized here; run \"lyx fabric
	// reconcile\"" remedy was circular when reconcile itself emitted it.
	// ReconcileFabricAt only adds absent keys and never rewrites a recorded pathspec.
	fabricResult, err := configsync.ReconcileFabricAt(fabricengine.BoardDir(l.HubPath), true)
	if err != nil {
		// ReconcileFabricAt can fail after a partial write, so this emits whatever rec holds rather
		// than a bare error.
		return errWithRecord(out, rec.Snapshot(), err)
	}
	if fabricResult.Applied {
		rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(fabricengine.BoardDir(l.HubPath), fabricResult.Module), "")
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Reconcile(l)
	if err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	rec.Extend(r.Mutated())

	binding := r.WarpBinding
	detail := r.WarpBindingDetail

	if binding == fabricengine.WarpBindingOutcomeRecorded || binding == fabricengine.WarpBindingOutcomePresent {
		b := fabricengine.NewBolt(fabricengine.BoardDir(l.HubPath))

		if binding == fabricengine.WarpBindingOutcomeRecorded {
			sha, committed, commitErr := b.Commit("fabric reconcile: record warp binding", fabricengine.SyncOptions{})
			if commitErr != nil {
				binding = fabricengine.WarpBindingOutcomeRecordFailed
				detail = commitErr.Error()
			} else if committed {
				rec.Append(fabricengine.KindCommitCreated, fabricengine.BoardDir(l.HubPath), sha)
			}
		}

		// Push on both "recorded" and "present": the "present" case is what retries a backfill that
		// committed locally but failed to push on a prior reconcile — without it, the next reconcile
		// would see the record already on disk, report "present" again, and a commit-only-on-
		// "recorded" handler would skip the push forever.
		//
		// Bolt.Push reaches gitrepo.PushCoalesced, which checks HasUnpushed (a purely local
		// rev-list) and returns nil without contacting the remote when HEAD is already in sync, so
		// this costs nothing when there is nothing to push. Caveat: HasUnpushed treats *no configured
		// upstream* as unpushed, so a board worktree with no upstream attempts a network push on
		// every reconcile. That is the adopt path's non-case — a board on an already-existing default
		// branch carries its upstream from the initial clone — but it IS the steady state for a hub
		// bootstrapped against a genuinely empty weft remote, whose board branch is orphan-created
		// with no upstream at all. The attempt is harmless: it either succeeds or yields
		// record_failed with the error in the detail.
		//
		// This push records nothing, and that is deliberate: a nil error from Bolt.Push means either
		// a push landed or nothing was unpushed to begin with, an unobservable-outcome distinction
		// that makes a KindBranchPushed entry here a lie of commission — the commit above is already
		// recorded, and branch_pushed is exempt from the truthfulness oracle's commission direction,
		// so omitting it costs the cross-check nothing.
		if binding == fabricengine.WarpBindingOutcomeRecorded || binding == fabricengine.WarpBindingOutcomePresent {
			if pushErr := b.Push(fabricengine.SyncOptions{}); pushErr != nil {
				wasPresent := binding == fabricengine.WarpBindingOutcomePresent
				binding = fabricengine.WarpBindingOutcomeRecordFailed
				if wasPresent {
					detail = fmt.Sprintf("a previously committed warp binding record could not be pushed: %v", pushErr)
				} else {
					detail = fmt.Sprintf("commit succeeded but push failed: %v", pushErr)
				}
			}
		}
	}

	envelope := map[string]any{
		"pairs":        r.Pairs,
		"warp_binding": string(binding),
	}
	if detail != "" {
		envelope["warp_binding_detail"] = detail
	}

	// A pair carrying an Error is a repair this verb was asked to perform and did not, so it must
	// not be reported through the success path. Every one of Topology.Reconcile's own pr.Error sites
	// is a genuine failure — a junction it could not re-point, a weft worktree it could not
	// recreate, a branch it could not read — never an advisory outcome, which is exactly why prune
	// and cleanup deliberately do NOT get this treatment: their per-entry Error doubles as the
	// explanation for a designed refusal ("commit them or re-run with --force"), and turning that
	// into a non-zero exit would report a documented outcome as a failure.
	// The envelope is carried through unchanged so a caller still learns WHICH pair failed; without
	// it, a caller would gain an exit code and lose the report it needs to act on.
	if pairErr := failedReconcilePairs(r.Pairs); pairErr != nil {
		return errWithRecordFields(out, rec.Snapshot(), pairErr, envelope)
	}

	return okWithRecord(out, rec.Snapshot(), envelope)
}

// failedReconcilePairs returns an error summarising every pair whose reconcile step failed, or nil
// when every pair reconciled cleanly.
//
// The summary names the count and the first failing pair's worktree and reason rather than
// concatenating all of them: the full per-pair detail already travels in the envelope's "pairs"
// array, so repeating it in the error string would duplicate the report an operator is about to
// read anyway.
func failedReconcilePairs(pairs []fabricengine.ReconcilePairResult) error {
	var failed []fabricengine.ReconcilePairResult
	for _, pair := range pairs {
		if pair.Error != "" {
			failed = append(failed, pair)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf(
		"reconcile could not repair %d of %d pair(s); first failure at %s: %s",
		len(failed), len(pairs), failed[0].WarpWorktree, failed[0].Error,
	)
}

// runPruneWithFlags executes the prune logic with the resolved apply and force flags.
func runPruneWithFlags(ctx context.Context, out io.Writer, apply, force bool) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		return output.Err(out, err.Error())
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Prune(l, apply, force)
	if err != nil {
		return errWithRecord(out, r.Mutated(), err)
	}
	return okWithRecord(out, r.Mutated(), map[string]any{
		"entries": r.Entries,
	})
}

// runCleanupWithFlags executes the cleanup logic with the resolved apply and
// force flags.
func runCleanupWithFlags(ctx context.Context, out io.Writer, apply, force bool) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
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
		return errWithRecord(out, r.Mutated(), err)
	}
	return okWithRecord(out, r.Mutated(), map[string]any{
		"entries": r.Entries,
	})
}

// runRemoveWithFlag executes the remove logic with the resolved force flag.
func runRemoveWithFlag(ctx context.Context, out io.Writer, args []string, force bool) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
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
		return errWithRecord(out, r.Mutated(), err)
	}
	return okWithRecord(out, r.Mutated(), map[string]any{
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
