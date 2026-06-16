// cli.go — muxpoc CLI entry point and configuration.
//
// RunCLI parses command-line flags and dispatches to subcommands. Config holds
// all paths and dimensions. Subcommands: up, review, attach, status, down, daemon.

package muxpoc

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// Config holds paths and dimensions for muxpoc operations.
type Config struct {
	PsmuxPath    string
	PwshPath     string
	ClaudePath   string
	LaunchTpl    string
	ResumeTpl    string
	Width        int
	Height       int
	Interval     time.Duration
	WorktreeRoot string
}

// RunCLI parses command-line flags and dispatches to the appropriate subcommand.
// Returns process exit code (0 on success, 1 on error).
//
// Usage:
//
//	mhgo muxpoc <subcommand> [args...]
//
// Subcommands:
//
//	up        cold-start or cold-recover the muxpoc session
//	review    add a reviewer pane to the active session
//	attach    pop the session into a maximized terminal
//	status    show session and pane status
//	down      stop the session and delete state
//	daemon    foreground poller; recovers a crashed session (crash-loop-guarded)
func RunCLI(out io.Writer, args []string) int {
	fs := flag.NewFlagSet("muxpoc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	psmuxPath := fs.String("psmux", `C:\Code\tools\bin\psmux.exe`, "path to psmux executable")
	pwshPath := fs.String("pwsh", `C:\Code\tools\powershell7\pwsh.exe`, "path to powershell executable")
	claudePath := fs.String("claude", "", "path to claude executable (empty: find on PATH)")
	launchTpl := fs.String("launch", "%CLAUDE% --session-id %SID% %TASK%", "template for new claude launch")
	resumeTpl := fs.String("resume", "%CLAUDE% --resume %SID%", "template for claude resume")
	width := fs.Int("width", 220, "psmux window width")
	height := fs.Int("height", 50, "psmux window height")
	interval := fs.Duration("interval", 2*time.Second, "poll interval for session checks")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Resolve the worktree root via paths
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get current working directory: %v", err))
	}

	layout, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("not a git repository: %v", err))
	}

	cfg := Config{
		PsmuxPath:    *psmuxPath,
		PwshPath:     *pwshPath,
		ClaudePath:   *claudePath,
		LaunchTpl:    *launchTpl,
		ResumeTpl:    *resumeTpl,
		Width:        *width,
		Height:       *height,
		Interval:     *interval,
		WorktreeRoot: layout.WorktreeRoot,
	}

	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lyx muxpoc <subcommand> [args...]")
		return 1
	}

	subcommand := rest[0]

	switch subcommand {
	case "up":
		return cmdUp(out, cfg)
	case "review":
		return cmdReview(out, cfg)
	case "attach":
		return cmdAttach(out, cfg)
	case "status":
		return cmdStatus(out, cfg)
	case "down":
		return cmdDown(out, cfg)
	case "daemon":
		return cmdDaemon(out, cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		return 1
	}
}
