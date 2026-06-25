// warp.go implements the RunCLI entry point for the warp command.
//
// warp.go is a thin subcommand dispatcher: it routes the first subcommand argument
// to the matching warp verb and delegates all further parsing and execution to that
// verb. Each verb owns its own flags, arguments, and output format.

package warp

import (
	"io"

	"github.com/Knatte18/loomyard/internal/output"
)

// RunCLI parses and executes warp subcommands, writing JSON results to out.
//
// It accepts a subcommand as the first argument (currently only "clone" is supported)
// and routes to the matching verb handler. Unknown or missing subcommands return a
// usage error.
//
// Returns exit code 0 on success or 1 on error. Output is JSON on out.
func RunCLI(out io.Writer, args []string) int {
	if len(args) < 1 {
		return output.Err(out, "usage: lyx warp <clone|...>")
	}

	subcommand, subArgs := args[0], args[1:]

	switch subcommand {
	case "clone":
		return runClone(out, subArgs)
	default:
		return output.Err(out, "usage: lyx warp <clone|...>")
	}
}
