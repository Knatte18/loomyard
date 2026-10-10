// fabric.go is the cobra Command() entry point and the RunCLI seam for the fabric module.
// It builds the "fabric" parent command and its hub-scoped topology verbs (add, list, remove,
// checkout, pairs, reconcile, prune, cleanup), each driving fabricengine.Topology for the warp↔weft
// worktree pairing.
// The weft-git content-sync verbs (status, commit, push, pull, sync, diff) are wired in by
// weft_verbs.go, which also extends this file's Command() build with the --weft-path bypass flag
// and its PersistentPreRunE.

package fabriccli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
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
		Short: "clone a hub, or add, sync, merge and remove its warp+weft worktree pairs",
		Long: `fabric keeps a hub's code (warp) and records (weft) git repositories paired:
every warp worktree has a weft worktree beside it, reached through junctions,
and fabric creates, switches, syncs, merges and tears down both sides together.

Every weft branch is named after its warp branch plus a fixed suffix (warp
branch "wt-foo" pairs with weft branch "wt-foo` + weftname.Suffix + `"), the
clone-time primary included.

Example:
  lyx fabric clone https://github.com/user/repo-weft
  lyx fabric add my-task
  lyx fabric remove my-task`,
		RunE: clihelp.GroupRunE,
	}

	// clone <weft-url> [<warp-url>]
	var cloneCmd *cobra.Command
	cloneCmd = &cobra.Command{
		Use:         "clone <weft-url> [<warp-url>]",
		Short:       "bootstrap a new hub, wiring the entire topology in one shot",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `clone creates a new hub directory, <warp-name>-LYXHUB, holding the warp
prime (<warp-name>), the weft prime (<warp-name>` + weftname.Suffix + `) and the _board worktree,
and wires the junctions and configs; no follow-up step is needed.

Reach for it to start working on a repository pair on this machine, or with
--reset to rebuild an existing hub from its remotes.

<weft-url> is the records repository. <warp-url> is the code repository: pass
it, with --shortname, the first time a weft is bound; afterwards it is read
from the binding recorded on the weft and may be left out.

--shortname is 2-6 characters matching [a-z][a-z0-9]{1,5}; a weft that
already records one supplies it, and "lyx fabric shortname" records one later.
--subpath anchors lyx at a subdirectory of the warp repo, such as backend in a
monorepo; a re-clone adopts the recorded subpath. --into names the directory
the hub is created in. --force-bootstrap admits a brand-new weft remote that
is neither empty nor lyx-anchored, such as one created with a README.

Example:
  lyx fabric clone --shortname mono --subpath backend https://github.com/user/mono-weft https://github.com/user/mono
  lyx fabric clone https://github.com/user/repo-weft
  lyx fabric clone --into ~/repos https://github.com/user/repo-weft`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			reset, _ := cloneCmd.Flags().GetBool("reset")
			subpath, _ := cloneCmd.Flags().GetString("subpath")
			forceBootstrap, _ := cloneCmd.Flags().GetBool("force-bootstrap")
			into, _ := cloneCmd.Flags().GetString("into")
			shortname, _ := cloneCmd.Flags().GetString("shortname")
			return runCloneWithReset(ctx, out, args, reset, subpath, forceBootstrap, into, shortname)
		}),
	}
	cloneCmd.Flags().String("shortname", "", "the repo's shortname, 2-6 characters matching [a-z][a-z0-9]{1,5}; required when the weft is bound for the first time, recorded as "+fabricengine.ShortnameFileName)
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
		Use:         "add <slug>",
		Args:        cobra.MaximumNArgs(1),
		Short:       "create a dual warp+weft worktree pair",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Create a new paired warp and weft git worktree for the given slug.

The new weft branch is forked from the HEAD of the worktree you run
"lyx fabric add" from — that worktree's current checked-out branch, not main
and not prime's branch. This makes the new pair an exact continuation of
the context you were working in. The weft branch name is always the warp
branch's name with fabric's uniform suffix appended.

A pair whose weft branch already exists, locally or on origin, is adopted
rather than forked.

The command errors if the worktree is on a detached HEAD or an unborn branch,
because a fork point cannot be determined in either case.

Example:
  lyx fabric add my-task`,
		RunE: clihelp.WrapRunCtx(runAdd),
	})

	// list
	cmd.AddCommand(&cobra.Command{
		Use:         "list",
		Args:        cobra.NoArgs,
		Short:       "list warp worktrees (use 'lyx fabric pairs' for full pair geometry)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `List all warp worktrees registered in the current hub.

This command outputs warp worktree paths only. For the full warp↔weft pair
geometry view — including weft pairing, branch drift, and junction health —
use "lyx fabric pairs".`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runList(ctx, out, args) }),
	})

	// remove <slug>
	var removeCmd *cobra.Command
	removeCmd = &cobra.Command{
		Use:         "remove <slug>",
		Args:        cobra.MaximumNArgs(1),
		Short:       "destroy a dual warp+weft worktree pair",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `remove tears down a warp+weft worktree pair: both worktrees, the warp
junctions (_lyx, .lyx), the pair's portal junction and launchers, its reed
session and its weft branch. The local task branch is deleted too once its
work is pushed or landed on the parent branch recorded at "lyx fabric add";
otherwise it is kept, with the reason in code_branch_kept_reason, and the
command still exits 0.

Reach for it when a task is landed or abandoned. It waits up to two minutes for
the pair's loom driver to go quiet, and refuses while the driver stays busy.

<slug> names the pair, as given to "lyx fabric add".

A pair with uncommitted changes on either side is refused; --force removes it
anyway, and never overrides the task branch check. --remote also deletes the
weft branch on the weft remote, and the landed task branch on the warp origin:
irreversible, and visible to every other clone.

Example:
  lyx fabric remove my-task
  lyx fabric remove --force my-task
  lyx fabric remove --remote my-task`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			// The --force and --remote flags are read from the cobra flag set via closure over
			// removeCmd.
			force, _ := removeCmd.Flags().GetBool("force")
			remote, _ := removeCmd.Flags().GetBool("remote")
			return runRemoveWithFlag(ctx, out, args, force, remote)
		}),
	}
	removeCmd.Flags().Bool("force", false, "forcefully remove worktree with uncommitted changes")
	removeCmd.Flags().Bool("remote", false, "irreversibly delete the pair's weft branch on the weft remote too, visible to every other clone")
	cmd.AddCommand(removeCmd)

	cmd.AddCommand(&cobra.Command{
		Use:         "checkout [<branch>]",
		Args:        cobra.MaximumNArgs(1),
		Short:       "coordinated branch switch across warp+weft with junction re-point",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
		Use:         "pairs",
		Args:        cobra.NoArgs,
		Short:       "show full warp↔weft pair geometry with drift and junction-health fields",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
		Use:         "reconcile",
		Args:        cobra.NoArgs,
		Short:       "repair a managed pair whose weft side drifted or broke",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
skipped: it never had either, so there is nothing there to repair.

It also heals every hub-wide config file, such as fabric.yaml or gate.yaml, at
the hub's board dir: an absent board.yaml is seeded from the prime worktree's copy
when it has one, and the written files are committed in _board and pushed. The
board is pulled first; when it cannot be brought up to date nothing is written
and the reason is reported under hub_config_detail without failing the verb.
The envelope reports each module under hub_config, a commit or push failure of
the hub-wide config or the warp-binding record under hub_config_detail or
warp_binding_detail and fails the verb with the envelope kept, and a board.yaml
started from the template, which carries no custom types or labels, under
warnings.`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int { return runReconcile(ctx, out, args) }),
	})

	// prune
	var pruneCmd *cobra.Command
	pruneCmd = &cobra.Command{
		Use:         "prune",
		Args:        cobra.NoArgs,
		Short:       "identify and optionally remove stale or orphaned warp↔weft pairs",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
		Use:         "cleanup",
		Args:        cobra.NoArgs,
		Short:       "delete weft branches whose warp sibling is gone",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `cleanup finds weft branches whose warp worktree sibling is gone, and with
--apply deletes them from the hub's weft repo.

Reach for it after pairs were removed by hand, or to clear weft branches left
behind on the remote. Without --apply it is a dry run whose "protected"
verdicts match what --apply would do, so "protected": false means "--apply
would delete this".

A weft branch checked out at a worktree, and the hub's primary weft branch
(e.g. "main` + weftname.Suffix + `"), are always protected. A weft branch without the
fabric suffix is reported and never deleted.

--remote, with --apply, also deletes each deleted branch's copy on the weft
remote, irreversibly and visibly to every other clone, and sweeps leftover
task branches on the warp origin: a fabric-managed branch that is not the
default branch, not checked out in the hub, has no open pull request and
carries no work the default branch lacks. Without --apply, --remote reports
what it would delete. --force answers no cleanup gate.

Example:
  lyx fabric cleanup
  lyx fabric cleanup --apply
  lyx fabric cleanup --apply --remote`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			apply, _ := cleanupCmd.Flags().GetBool("apply")
			force, _ := cleanupCmd.Flags().GetBool("force")
			remote, _ := cleanupCmd.Flags().GetBool("remote")
			return runCleanupWithFlags(ctx, out, apply, force, remote)
		}),
	}
	cleanupCmd.Flags().Bool("apply", false, "delete orphaned weft branches (default is dry-run/report)")
	cleanupCmd.Flags().Bool("force", false, "reserved; answers no cleanup gate today")
	cleanupCmd.Flags().Bool("remote", false, "irreversibly delete each deleted branch's copy on the weft remote too, visible to every other clone; requires --apply")
	cmd.AddCommand(cleanupCmd)

	cmd.AddCommand(&cobra.Command{
		Use:         "unwire",
		Args:        cobra.NoArgs,
		Short:       "fully deactivate fabric wiring for this worktree",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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

	cmd.AddCommand(&cobra.Command{
		Use:         "shortname [<shortname>]",
		Args:        cobra.MaximumNArgs(1),
		Short:       "print or record the hub's shortname",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Print the hub's shortname, or record one.

With no argument it prints the recorded shortname. With one argument it records the
shortname in ` + fabricengine.ShortnameFileName + ` on weft:main when the hub has none, committing and
pushing it; the shortname is 2-6 characters matching [a-z][a-z0-9]{1,5}.

Recording the shortname a hub already has is a no-op. A different shortname is refused:
changing it would orphan every agent name already in use.

Example:
  lyx fabric shortname
  lyx fabric shortname ly`,
		RunE: clihelp.WrapRunCtx(runShortname),
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
		// A leftover refusal fires before Add's first mutation, so it is a pre-flight failure: a bare output.Err.
		var leftover *fabricengine.ErrRemoteLeftover
		if errors.As(err, &leftover) {
			return output.Err(out, err.Error())
		}
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
		"branch":           r.Branch,
		"records_worktree": r.RecordsWorktree,
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
// Topology.Reconcile itself never returns — and fails the verb with the envelope kept, as does a commit or push failure of the hub-wide config.
// A heal skipped because the board could not be brought up to date is reported under hub_config_detail and is no failure.
// A pair failure takes precedence over these as the error the verb returns.
func runReconcile(ctx context.Context, out io.Writer, _ []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// The recorder is built here, as soon as l resolves, and not after top.Reconcile(l) returns:
	// configsync.ReconcileHubWideAt below runs before top.Reconcile in this handler and may already
	// have written a file, so seeding from r.Mutated() first would misstate the array's order — array
	// order is the only thing carrying ordering in this vocabulary. This is the one handler where
	// "pre-flight" and "pre-mutation" come apart: none of the three output.Err sites below qualifies
	// for the ordinary pre-flight carve-out, since ReconcileHubWideAt may already have mutated state by
	// the time any of them is reached.
	rec := fabricengine.NewMutations(l.HubPath)

	// Reconcile is the repair verb, so a missing hub-wide config is healed here rather than reported:
	// without this, the "run \"lyx fabric reconcile\"" remedy of a strict config loader was circular when reconcile itself emitted it.
	// ReconcileHubWideAt only adds absent keys and never rewrites a recorded pathspec.
	// A PrimeName failure means no prime is resolvable, not an error: the seed falls back to the template.
	boardDir := fabricengine.BoardDir(l.HubPath)
	primeBaseDir := ""
	if primeName, primeErr := fabricengine.PrimeName(l); primeErr == nil {
		primeBaseDir = filepath.Join(l.HubPath, primeName, l.AnchorRel)
	}

	// The board is pulled first, and the files are written and committed in _board under the board write lock.
	// A skipped heal writes nothing and is reported under hub_config_detail without failing the verb.
	// A commit or push failure leaves the file for the next board sync and fails the verb once the envelope is built.
	var hubWideResults []configsync.Result
	var reconcileErr error
	var firstFailure error
	hubConfigBolt := fabricengine.NewBolt(boardDir)
	healed, commitErr := hubConfigBolt.PullThenCommitWritten([]fabricengine.BoltWrite{{
		Message: "fabric reconcile: hub-wide config",
		Write: func() ([]string, error) {
			results, err := configsync.ReconcileHubWideAt(boardDir, primeBaseDir, true)
			if err != nil {
				reconcileErr = err
				return nil, err
			}
			hubWideResults = results
			var written []string
			for _, result := range results {
				if !result.Applied {
					continue
				}
				written = append(written, configengine.ConfigFileRel(result.Module))
				for _, legacy := range result.MigratedFrom {
					written = append(written, configengine.ConfigFileRel(legacy))
				}
			}
			return written, nil
		},
	}}, rec)
	if reconcileErr != nil {
		// ReconcileHubWideAt can fail after a partial write, so this emits whatever rec holds rather
		// than a bare error.
		return errWithRecord(out, rec.Snapshot(), reconcileErr)
	}
	hubConfigDetail := ""
	switch {
	case commitErr != nil && hubWideResults == nil:
		// The write step never ran: the board write lock could not be taken, or the pull failed outright.
		return errWithRecord(out, rec.Snapshot(), commitErr)
	case commitErr != nil:
		hubConfigDetail = commitErr.Error()
		firstFailure = hubConfigFailure(hubConfigDetail, boardDir)
	case healed.Skipped != "":
		hubConfigDetail = healed.SkipDetail
	default:
		if pushErr := hubConfigBolt.PushRecorded(fabricengine.SyncOptions{}, rec); pushErr != nil {
			hubConfigDetail = fmt.Sprintf("hub-wide config committed but push failed: %v", pushErr)
			firstFailure = hubConfigFailure(hubConfigDetail, boardDir)
		}
	}

	hubConfig := make([]map[string]any, 0, len(hubWideResults))
	var warnings []string
	for _, result := range hubWideResults {
		hubConfig = append(hubConfig, map[string]any{"module": result.Module, "seed": result.Seed, "applied": result.Applied})
		module, _ := configreg.Lookup(result.Module)
		if len(module.OpenMaps) > 0 && result.Seed == configsync.SeedTemplate {
			warnings = append(warnings, fmt.Sprintf(
				"%s: the prime held no copy, so custom types and labels were not carried; add them by editing that file",
				configengine.ConfigFile(boardDir, result.Module),
			))
		}
	}

	// A failure past the heal still reports why the heal did not run, since a strict config load fails on exactly the file a skipped heal left absent.
	var healFields map[string]any
	if hubConfigDetail != "" {
		healFields = map[string]any{"hub_config_detail": hubConfigDetail}
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		return errWithRecordFields(out, rec.Snapshot(), err, healFields)
	}

	top := fabricengine.NewTopology(cfg)

	r, err := top.Reconcile(l)
	if err != nil {
		return errWithRecordFields(out, rec.Snapshot(), err, healFields)
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
				if firstFailure == nil {
					firstFailure = warpBindingFailure(detail, fabricengine.BoardDir(l.HubPath))
				}
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
		// This push records no branch_pushed entry, and that is deliberate:
		// a nil error from Bolt.PushRecorded means either a push landed or nothing was unpushed to begin with, an unobservable-outcome distinction that makes a KindBranchPushed entry here a lie of commission.
		// The commit above is already recorded, and branch_pushed is exempt from the truthfulness oracle's commission direction, so omitting it costs the cross-check nothing.
		// A seed-commit drop behind the push is recorded in rec as commits_dropped.
		if binding == fabricengine.WarpBindingOutcomeRecorded || binding == fabricengine.WarpBindingOutcomePresent {
			if pushErr := b.PushRecorded(fabricengine.SyncOptions{}, rec); pushErr != nil {
				wasPresent := binding == fabricengine.WarpBindingOutcomePresent
				binding = fabricengine.WarpBindingOutcomeRecordFailed
				if wasPresent {
					detail = fmt.Sprintf("a previously committed warp binding record could not be pushed: %v", pushErr)
				} else {
					detail = fmt.Sprintf("commit succeeded but push failed: %v", pushErr)
				}
				if firstFailure == nil {
					firstFailure = warpBindingFailure(detail, fabricengine.BoardDir(l.HubPath))
				}
			}
		}
	}

	envelope := map[string]any{
		"pairs":        r.Pairs,
		"warp_binding": string(binding),
		"hub_config":   hubConfig,
	}
	if detail != "" {
		envelope["warp_binding_detail"] = detail
	}
	if hubConfigDetail != "" {
		envelope["hub_config_detail"] = hubConfigDetail
	}
	if len(warnings) > 0 {
		envelope["warnings"] = warnings
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
	if firstFailure != nil {
		return errWithRecordFields(out, rec.Snapshot(), firstFailure, envelope)
	}

	return okWithRecord(out, rec.Snapshot(), envelope)
}

// reconcileFailureWayForward words the way forward of a board commit or push failure in boardDir.
func reconcileFailureWayForward(boardDir string) string {
	return fmt.Sprintf("re-run \"lyx fabric reconcile\" once the remote is reachable, or for a rejected push run `git pull --rebase` in %s and then `lyx board sync`", boardDir)
}

// hubConfigFailure is the error that fails the verb when the hub-wide config's commit or push failed.
func hubConfigFailure(detail, boardDir string) error {
	return fmt.Errorf("fabric reconcile: %s; %s", detail, reconcileFailureWayForward(boardDir))
}

// warpBindingFailure is the error that fails the verb when the warp-binding record's commit or push failed.
func warpBindingFailure(detail, boardDir string) error {
	return fmt.Errorf("fabric reconcile: warp binding record failed: %s; %s", detail, reconcileFailureWayForward(boardDir))
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
		len(failed), len(pairs), failed[0].CodeWorktree, failed[0].Error,
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

// runCleanupWithFlags executes the cleanup logic with the resolved apply,
// force, and remote flags.
func runCleanupWithFlags(ctx context.Context, out io.Writer, apply, force, remote bool) int {
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

	r, err := top.Cleanup(l, apply, force, remote)
	if err != nil {
		return errWithRecord(out, r.Mutated(), err)
	}

	// fields, not CleanupResult, is what actually marshals — remote_skipped_reason reaches the
	// envelope because the map names it, regardless of the struct's own omitempty tag.
	fields := map[string]any{
		"entries":               r.Entries,
		"remote_skipped_reason": r.RemoteSkippedReason,
	}

	// The origin task-branch sweep runs after the weft sweep and folds its record into the one envelope record.
	// Cleanup's signature is untouched and fabricengine imports no GitHub client, so the open-PR set is fetched here.
	rec := r.Mutations
	var warpFailedBranches []string
	var warpAttempted int
	if remote {
		warp, warpErr := sweepRemoteTaskBranches(ctx, top, l, apply)
		rec.Extend(warp.Mutations)
		if warpErr != nil {
			return errWithRecordFields(out, rec, warpErr, fields)
		}
		fields["warp_entries"] = warp.Entries
		fields["warp_skipped_reason"] = warp.SkippedReason
		for _, entry := range warp.Entries {
			if entry.Error != "" {
				warpFailedBranches = append(warpFailedBranches, entry.Branch)
			}
			if entry.Error != "" || entry.Deleted {
				warpAttempted++
			}
		}
	}

	// Error is always a genuine failure here, never a designed refusal — Cleanup sets no Error on a
	// protected or unmanaged entry — unlike prune's Error, which stays in doc.go's carve-out.
	var localFailedBranches, remoteFailedBranches []string
	var attemptedLocal, attempted int
	for _, entry := range r.Entries {
		if entry.Error != "" {
			localFailedBranches = append(localFailedBranches, entry.Branch)
		}
		if entry.Error != "" || entry.Deleted {
			attemptedLocal++
		}
		if entry.RemoteError != "" {
			remoteFailedBranches = append(remoteFailedBranches, entry.Branch)
		}
		if entry.Deleted {
			attempted++
		}
	}

	var synthesised error
	switch {
	case len(localFailedBranches) > 0 && len(remoteFailedBranches) > 0:
		localErr := fmt.Errorf(
			"branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error",
			len(localFailedBranches), attemptedLocal, strings.Join(localFailedBranches, ", "))
		remoteErr := fmt.Errorf(
			"remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error",
			len(remoteFailedBranches), attempted, strings.Join(remoteFailedBranches, ", "))
		synthesised = fmt.Errorf("%v; additionally, %v", localErr, remoteErr)
	case len(localFailedBranches) > 0:
		synthesised = fmt.Errorf(
			"branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].error",
			len(localFailedBranches), attemptedLocal, strings.Join(localFailedBranches, ", "))
	case len(remoteFailedBranches) > 0:
		synthesised = fmt.Errorf(
			"remote branch deletion failed for %d of %d orphan branches (%s); each branch's reason is in entries[].remote_error",
			len(remoteFailedBranches), attempted, strings.Join(remoteFailedBranches, ", "))
	}

	if len(warpFailedBranches) > 0 {
		warpErr := fmt.Errorf(
			"task branch deletion on origin failed for %d of %d task branches (%s); each branch's reason is in warp_entries[].error",
			len(warpFailedBranches), warpAttempted, strings.Join(warpFailedBranches, ", "))
		if synthesised != nil {
			synthesised = fmt.Errorf("%v; additionally, %v", synthesised, warpErr)
		} else {
			synthesised = warpErr
		}
	}

	if synthesised != nil {
		return errWithRecordFields(out, rec, synthesised, fields)
	}
	return okWithRecord(out, rec, fields)
}

// sweepRemoteTaskBranches runs the origin task-branch sweep with the open pull request set fetched from GitHub.
// When the set cannot be established it skips the sweep entirely and names the cause in SkippedReason, failing toward keeping every task branch;
// the returned entries are then an empty array, never nil.
func sweepRemoteTaskBranches(ctx context.Context, top *fabricengine.Topology, l *lyxcwd.Location, apply bool) (fabricengine.RemoteWarpCleanupResult, error) {
	skipped := func(reason string) fabricengine.RemoteWarpCleanupResult {
		return fabricengine.RemoteWarpCleanupResult{
			Entries:       []fabricengine.RemoteWarpBranchEntry{},
			SkippedReason: reason,
		}
	}

	remoteURL, err := gitrepo.New(l.WorktreePath()).RemoteURL("origin")
	if err != nil {
		return skipped(fmt.Sprintf("task branches on origin were kept: the warp repo has no origin remote: %v", err)), nil
	}
	heads, err := listOpenPRHeads(ctx, remoteURL)
	if err != nil {
		return skipped(fmt.Sprintf("task branches on origin were kept: the open pull requests could not be listed: %v", err)), nil
	}
	return top.CleanupRemoteWarp(l, apply, heads)
}

// runRemoveWithFlag executes the remove logic with the resolved force and remote flags.
func runRemoveWithFlag(ctx context.Context, out io.Writer, args []string, force, remote bool) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		return output.Err(out, err.Error())
	}

	// args[0] is the slug; cobra has already consumed "remove" from the argument list.
	if len(args) < 1 {
		return output.Err(out, "usage: lyx fabric remove [--force] [--remote] <slug>")
	}
	slug := args[0]

	td, err := pairteardown.New(l)
	if err != nil {
		return output.Err(out, err.Error())
	}
	res, err := td.Run(ctx, pairteardown.Request{
		Slug:           slug,
		Force:          force,
		Remote:         remote,
		QuietWait:      pairteardown.RemoveQuietWait,
		RefuseWhenBusy: true,
	})
	r := res.Removal
	if err != nil {
		return errWithRecord(out, r.Mutated(), err)
	}

	fields := removeFields(r, res.Session)

	// Keyed on RemoteBranchError alone — never on RemoteSkippedReason — so that a missing origin
	// produces exit 0 here exactly as it does from cleanup, and the identical configuration state
	// never yields two different verdicts across the two verbs.
	if r.RemoteBranchError != "" {
		weftBranch := fabricengine.RecordsBranchName(cfg.BranchPrefix + slug)
		// "origin" is hardcoded here rather than referencing fabricengine's own unexported
		// originRemoteName, which stays unexported: exporting it just to spell this one error string
		// would widen the engine's API for no caller that needs it.
		synthesised := fmt.Errorf(
			"weft branch %q was deleted locally, but its copy on %q was not: %s",
			weftBranch, "origin", r.RemoteBranchError)
		return errWithRecordFields(out, r.Mutated(), synthesised, fields)
	}
	return okWithRecord(out, r.Mutated(), fields)
}

// removeFields builds the envelope fields for a remove result.
// New RemoveResult fields must be added here explicitly — the map is hand-built, not reflected.
// code_branch_kept_reason, remote_code_branch_kept_reason, stray_path and abandoned_session appear only when set.
func removeFields(r fabricengine.RemoveResult, session pairteardown.SessionResult) map[string]any {
	steps := r.Steps
	if steps == nil {
		steps = []string{}
	}
	fields := map[string]any{
		"slug":                       r.Slug,
		"path":                       r.Path,
		"links_removed":              r.LinksRemoved,
		"remote_branch_deleted":      r.RemoteBranchDeleted,
		"remote_branch_error":        r.RemoteBranchError,
		"remote_skipped_reason":      r.RemoteSkippedReason,
		"code_branch_deleted":        r.CodeBranchDeleted,
		"steps":                      steps,
		"finished":                   r.Finished,
		"remote_code_branch_deleted": r.RemoteCodeBranchDeleted,
		"session_ended":              session.Ended,
	}
	if r.CodeBranchKeptReason != "" {
		fields["code_branch_kept_reason"] = r.CodeBranchKeptReason
	}
	if r.RemoteCodeBranchKeptReason != "" {
		fields["remote_code_branch_kept_reason"] = r.RemoteCodeBranchKeptReason
	}
	if r.StrayPath != "" {
		fields["stray_path"] = r.StrayPath
	}
	if session.AbandonedSession != "" {
		fields["abandoned_session"] = session.AbandonedSession
	}
	return fields
}

// addOptionsFromEnv returns the AddOptions for a CLI-driven `lyx fabric add`,
// always returning the zero value. The add subcommand always pushes both sides;
// bypass gates do not apply.
func addOptionsFromEnv() fabricengine.AddOptions {
	return fabricengine.AddOptions{}
}
