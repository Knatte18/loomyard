// Command mhgo is the CLI for the mhgo task tracker.
//
// main.go is a thin module dispatcher: it routes the first argument to the
// matching module and delegates all further parsing and execution to that
// module. Each module owns its own flags, subcommands, and output format.
//
// Usage:
//
//	mhgo <module> [module-args...]
//
// Modules:
//
//	wiki   task-tracker wiki — see internal/wiki.RunCLI for subcommands
//
// All output is JSON on stdout. Exit code 1 on error.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/mhgo/internal/wiki"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

// run dispatches args to the matching module, writing module output to out.
// It returns the process exit code (0 on success, 1 on error). Usage and
// unknown-module messages go to stderr; module output goes to out.
func run(args []string, out io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: mhgo <module> [args...]")
		return 1
	}

	module, moduleArgs := args[0], args[1:]

	switch module {
	case "wiki":
		return wiki.RunCLI(out, moduleArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown module: %s\n", module)
		return 1
	}
}
