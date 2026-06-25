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
// warp.go is a thin subcommand dispatcher: it routes the first subcommand argument
// to the matching warp verb and delegates all further parsing and execution to that
// verb. Each verb owns its own flags, arguments, and output format.
package warp

import (
	"flag"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunCLI parses and executes warp subcommands, writing JSON results to out.
//
// It accepts a subcommand as the first argument (clone, add, list, remove, checkout,
// status, reconcile, prune, cleanup) and routes to the matching verb handler. Unknown
// or missing subcommands return a usage error.
//
// Returns exit code 0 on success or 1 on error. Output is JSON on out.
func RunCLI(out io.Writer, args []string) int {
	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp <clone|add|list|remove|checkout|status|reconcile|prune|cleanup>")
	}

	subcommand, subArgs := args[0], args[1:]

	switch subcommand {
	case "clone":
		return runClone(out, subArgs)
	case "add":
		return runAdd(out, subArgs)
	case "list":
		return runList(out, subArgs)
	case "remove":
		return runRemove(out, subArgs)
	case "checkout":
		return runCheckout(out, subArgs)
	case "status":
		return runStatus(out, subArgs)
	case "reconcile":
		return runReconcile(out, subArgs)
	case "prune":
		return runPrune(out, subArgs)
	case "cleanup":
		return runCleanup(out, subArgs)
	default:
		return output.Err(out, "usage: lyx warp <clone|add|list|remove|checkout|status|reconcile|prune|cleanup>")
	}
}

// runAdd parses and executes the warp add subcommand.
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
		return output.Err(out, "usage: warp add <slug>")
	}

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
func runList(out io.Writer, args []string) int {
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
// then calls Checkout with the supplied branch argument. On success it emits
// a JSON object with branch and weft_worktree fields.
func runCheckout(out io.Writer, args []string) int {
	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp checkout <branch>")
	}

	branch := args[0]

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

// runPrune parses and executes the warp prune subcommand.
//
// Resolves the layout and warp config from the current working directory,
// calls Prune to identify stale or orphaned host↔weft pairs, and emits the
// result via output.Ok. The --apply flag switches from dry-run/report to
// actually removing stale weft worktrees.
func runPrune(out io.Writer, args []string) int {
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

	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	apply := fs.Bool("apply", false, "remove stale weft worktrees (default is dry-run/report)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	w := New(cfg)

	r, err := w.Prune(l, *apply)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"entries": r.Entries,
	})
}

// runCleanup parses and executes the warp cleanup subcommand.
//
// Resolves the layout and warp config from the current working directory,
// calls Cleanup to find orphaned weft branches, and emits the result via
// output.Ok. The flag matrix governs deletion:
//
//   - (no flags)          dry-run: report orphaned weft branches only.
//   - --apply             delete non-gate-protected orphan branches.
//   - --apply --force     also delete gate-protected task branches.
//   - --force (alone)     report only; --force does not imply --apply.
func runCleanup(out io.Writer, args []string) int {
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

	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	apply := fs.Bool("apply", false, "delete non-gate-protected orphaned weft branches")
	force := fs.Bool("force", false, "also delete gate-protected task branches (requires --apply)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	w := New(cfg)

	r, err := w.Cleanup(l, *apply, *force)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"entries": r.Entries,
	})
}

// runRemove parses and executes the warp remove subcommand.
func runRemove(out io.Writer, args []string) int {
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

	// Parse flags
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "forcefully remove worktree with uncommitted changes")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	slug := fs.Arg(0)
	if slug == "" {
		return output.Err(out, "usage: warp remove [--force] <slug>")
	}

	r, err := w.Remove(l, slug, *force)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"slug":          r.Slug,
		"path":          r.Path,
		"links_removed": r.LinksRemoved,
	})
}
