// cli.go — the weft module's command router.
//
// RunCLI parses weft subcommands (status, commit, push, pull, sync) and dispatches
// to the corresponding weft operations. When --weft-path is set (internal flag for
// detached push child), it uses that path directly; otherwise it resolves from the
// current working directory via paths.Layout geometry.

package weft

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// envSyncOptions reads WEFT_SKIP_* environment variables and returns a SyncOptions.
func envSyncOptions() SyncOptions {
	return SyncOptions{
		SkipGit:  os.Getenv("WEFT_SKIP_GIT") == "1",
		SkipPush: os.Getenv("WEFT_SKIP_PUSH") == "1",
	}
}

// RunCLI parses and executes a "weft" subcommand, writing JSON results to out.
// It returns the process exit code (0 on success, 1 on error).
//
// Usage:
//
//	weft <subcommand> [args]
//	weft --weft-path <path> push
//
// When --weft-path is set (internal flag for detached push child):
//   - Only the "push" subcommand is valid
//   - Any other subcommand returns an error
//
// When --weft-path is not set (cwd-authoritative):
//   - Resolves the layout from cwd
//   - Loads weft config
//   - Builds the scoped pathspec
//   - Dispatches subcommands: status, commit, push, pull, sync
//
// All output is JSON on out.
// Success: {"ok":true, ...}
// Error:   {"ok":false,"error":"..."} with exit code 1.
func RunCLI(out io.Writer, args []string) int {
	fs := flag.NewFlagSet("weft", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	weftPathFlag := fs.String("weft-path", "", "internal: injected absolute weft worktree path for the detached push child")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	// fs.Args() returns the arguments remaining after flags are parsed
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lyx weft <subcommand> [args]")
		return 1
	}

	subcommand := rest[0]

	// If --weft-path is set, use it directly (internal use for detached push child)
	if *weftPathFlag != "" {
		// Only "push" is valid in detached mode
		if subcommand != "push" {
			return output.Err(out, "subcommand requires a worktree context")
		}

		// Pass zero-value SyncOptions; the detached child pushes unconditionally
		// since spawnPush has already decided via its env check that pushing should proceed.
		if err := Push(*weftPathFlag, SyncOptions{}); err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{})
	}

	// Resolve cwd and layout
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Determine weft paths
	weftWorktree := l.WeftWorktree()
	weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)

	// Load config
	cfg, err := LoadConfig(weftBaseDir)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Build scoped pathspec
	pathspec := scopedPathspec(l.RelPath, cfg.Dirs())

	// Dispatch subcommands
	switch subcommand {
	case "status":
		statusMap, err := Status(weftWorktree, l.HostLyxLinkHere(), l.WeftLyxDir(), pathspec)
		if err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, statusMap)

	case "commit":
		committed, err := Commit(weftWorktree, pathspec, envSyncOptions())
		if err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{"committed": committed})

	case "push":
		opts := envSyncOptions()
		_, err := Commit(weftWorktree, pathspec, opts)
		if err != nil {
			return output.Err(out, err.Error())
		}
		if err := Push(weftWorktree, opts); err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{})

	case "pull":
		if err := Pull(weftWorktree, envSyncOptions()); err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{})

	case "sync":
		_, err := Commit(weftWorktree, pathspec, envSyncOptions())
		if err != nil {
			return output.Err(out, err.Error())
		}
		if err := spawnPush(weftWorktree); err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{})

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		return 1
	}
}
