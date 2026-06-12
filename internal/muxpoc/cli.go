// cli.go — the muxpoc module command router (parallel to internal/board's RunCLI).
//
// muxpoc is the single-column, in-place proof-of-concept for the mux daemon/resume design.
// Usage: mhgo muxpoc <subcommand> [flags]
//
//	up        create the column (or cold-recover from state); launch claude --session-id
//	review    stack a reviewer pane below and launch a claude in it
//	attach    pop a maximized Windows Terminal attached to the session (visible)
//	status    JSON: state, live panes, server up, and which env vars are stripped
//	down      kill the psmux server and clear state
//	daemon    foreground watchdog: poll the server and rebuild+resume on crash (blocks)
//
// All non-daemon subcommands emit a JSON envelope (ok=true/…); daemon streams log lines.
package muxpoc

import (
	"flag"
	"io"
	"os"
	"time"

	"github.com/Knatte18/mhgo/internal/output"
)

// RunCLI parses and executes a "muxpoc" subcommand, returning the process exit code.
func RunCLI(out io.Writer, args []string) int {
	if len(args) < 1 {
		return output.Err(out, "usage: mhgo muxpoc <up|review|attach|status|down|daemon> [flags]")
	}
	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("muxpoc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	def := DefaultConfig()
	psmuxBin := fs.String("psmux", def.Psmux, "psmux binary path")
	pwshBin := fs.String("pwsh", def.Pwsh, "PowerShell 7 binary path")
	claudeBin := fs.String("claude", def.Claude, "claude binary path")
	launchTmpl := fs.String("launch", def.LaunchTemplate, "fresh-session launch template (%CLAUDE%, %SID%)")
	resumeTmpl := fs.String("resume", def.ResumeTemplate, "resume template (%CLAUDE%, %SID%)")
	width := fs.Int("width", 200, "session width in cells")
	height := fs.Int("height", 50, "session height in cells")
	interval := fs.Duration("interval", 2*time.Second, "daemon poll interval")
	if err := fs.Parse(rest); err != nil {
		return 1
	}

	cfg := Config{
		Psmux: *psmuxBin, Pwsh: *pwshBin, Claude: *claudeBin,
		LaunchTemplate: *launchTmpl, ResumeTemplate: *resumeTmpl,
	}
	cwd, err := os.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	switch sub {
	case "up":
		res, err := opUp(cfg, cwd, *width, *height)
		return emit(out, res, err)
	case "review":
		res, err := opReview(cfg, cwd)
		return emit(out, res, err)
	case "attach":
		res, err := opAttach(cfg, cwd)
		return emit(out, res, err)
	case "status":
		res, err := opStatus(cfg, cwd)
		return emit(out, res, err)
	case "down":
		res, err := opDown(cfg, cwd)
		return emit(out, res, err)
	case "daemon":
		// Foreground, blocking, streams log lines until interrupted.
		if err := opDaemon(cfg, cwd, *interval, out); err != nil {
			return output.Err(out, err.Error())
		}
		return 0
	default:
		return output.Err(out, "unknown muxpoc subcommand: "+sub)
	}
}

// emit renders an (result, error) pair as the standard JSON envelope.
func emit(out io.Writer, res map[string]any, err error) int {
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, res)
}
