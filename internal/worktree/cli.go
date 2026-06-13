// cli.go — the worktree module's command router.
//
// RunCLI parses <subcommand> [args], resolves the worktree configuration
// from the current working directory (cwd-authoritative model), dispatches to
// one Worktree method, and writes the JSON result to the given writer. Owns the
// worktree CLI surface so main stays a thin module dispatcher.

package worktree

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/mhgo/internal/output"
)

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
	cwd, err := os.Getwd()
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
		fmt.Fprintln(os.Stderr, "usage: mhgo worktree <subcommand> [args]")
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
		r, err := w.Add(cwd, slug)
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

		r, err := w.Remove(cwd, slug, *force)
		if err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{
			"slug":           r.Slug,
			"path":           r.Path,
			"links_removed":  r.LinksRemoved,
		})

	default:
		return output.Err(out, fmt.Sprintf("unknown subcommand: %s", subcommand))
	}
}
