// warp.go implements the RunCLI entry point for the warp command.
//
// warp.go is a thin subcommand dispatcher: it routes the first subcommand argument
// to the matching warp verb and delegates all further parsing and execution to that
// verb. Each verb owns its own flags, arguments, and output format.

package warp

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunCLI parses and executes warp subcommands, writing JSON results to out.
//
// It accepts a subcommand as the first argument (clone, add, list, remove)
// and routes to the matching verb handler. Unknown or missing subcommands return a
// usage error.
//
// Returns exit code 0 on success or 1 on error. Output is JSON on out.
func RunCLI(out io.Writer, args []string) int {
	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp <clone|add|list|remove>")
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
	default:
		return output.Err(out, "usage: lyx warp <clone|add|list|remove>")
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

	l, err := paths.Resolve(cwd)
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
