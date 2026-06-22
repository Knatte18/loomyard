// Package worktree provides creation, tracking, and teardown of git worktrees.
// The module is cwd-authoritative: all operations resolve paths relative to the
// current working directory.
//
// Worktrees are stateless — there is no worktree registry or local-state.json.
// Worktree state is read entirely from git worktree list --porcelain (see list.go).
//
// Windows teardown hazard: Worktree removal must be junction-aware and ordered.
// Junctions and portals must be removed
// before calling git worktree remove, or Windows holds the directory lock and
// removal fails. See remove.go and junction_windows.go for details.

package worktree

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// addOptionsFromEnv constructs an AddOptions from the WEFT_SKIP_GIT and
// WEFT_SKIP_PUSH environment variables. This mapping lives at the CLI edge so
// that in-process callers (tests, library users) can pass options directly
// without touching the environment.
func addOptionsFromEnv() AddOptions {
	return AddOptions{
		SkipGit:  os.Getenv("WEFT_SKIP_GIT") == "1",
		SkipPush: os.Getenv("WEFT_SKIP_PUSH") == "1",
	}
}

// RunCLI parses and executes a "worktree" subcommand, writing JSON results to out.
// It returns the process exit code (0 on success, 1 on error).
//
// Usage:
//
//	worktree <subcommand> [args]
//
// Configuration resolution (cwd-authoritative):
// RunCLI delegates to LoadConfig, which resolves the worktree config from the
// current working directory via internal/config. The worktree module never reads
// config files itself — file layout and overrides are entirely internal/config's
// concern.
//
// Subcommands:
//
//	add <slug>                  Create a new git worktree with the given slug.
//	list                        List all git worktrees in the repository.
//	remove [--force] <slug>     Remove a git worktree (--force skips dirty check).
//
// All output is JSON on out.
// Success: {"ok":true, ...}
// Error:   {"ok":false,"error":"..."} with exit code 1.
func RunCLI(out io.Writer, args []string) int {
	// Resolve cwd
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Resolve Layout
	// Note: paths.Resolve checks for git-repo membership (via a git rev-parse query)
	// and fails with ErrNotAGitRepo if the cwd is not within a git repository.
	// This failure precedes the LoadConfig call intentionally: geometry errors
	// (not a git repo) are fatal and take priority over initialization errors
	// (missing _lyx/ config). This ensures consistent error reporting for callers
	// outside a git repository.
	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Load config
	cfg, err := LoadConfig(cwd, "worktree")
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Create worktree facade
	w := New(cfg)

	// Require subcommand
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lyx worktree <subcommand> [args]")
		return 1
	}

	subcommand := args[0]

	switch subcommand {
	case "add":
		// add <slug>
		if len(args) < 2 {
			return output.Err(out, "usage: worktree add <slug>")
		}
		slug := args[1]
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

	case "list":
		// list (no args)
		entries, err := w.List(cwd)
		if err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{
			"worktrees": entries,
		})

	case "remove":
		// remove [--force] <slug>
		fs := flag.NewFlagSet("remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		force := fs.Bool("force", false, "forcefully remove worktree with uncommitted changes")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}

		slug := fs.Arg(0)
		if slug == "" {
			return output.Err(out, "usage: worktree remove [--force] <slug>")
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

	default:
		return output.Err(out, fmt.Sprintf("unknown subcommand: %s", subcommand))
	}
}
