// main.go implements the codeintel-poc harness entry point: flag parsing and
// mode dispatch. The harness measures structured Go reference/call-graph
// lookup via go/packages+go/types (in-process), gopls (subprocess CLI and
// LSP), and go/callgraph (CHA/RTA/VTA), comparing their steady-state latency
// and precision against hand-verified ground truth. It is throwaway
// instrumentation per Shared Decision no-production-module-conventions: no
// *_test.go, no CLI/Cobra registration, run manually via
// `go run ./tools/codeintel-poc ...`.

package main

import (
	"flag"
	"fmt"
	"os"
)

// usage is the -help text. It documents every mode this harness supports and
// the symbol spec format every mode shares, so an operator running the
// binary cold does not need to read the source to construct a query.
const usage = `codeintel-poc: structured Go reference/call-graph lookup spike harness

Usage:
  go run ./tools/codeintel-poc -mode=<mode> -symbol=<spec> [flags]

Modes:
  refs           in-process go/packages + go/types reference finder
  callers        in-process direct-caller (call-hierarchy) finder
  gopls-refs     gopls subprocess, driven over stdin/stdout LSP JSON-RPC
  gopls-cli-refs gopls "references" CLI subcommand
  callgraph      go/callgraph transitive caller finder (CHA/RTA/VTA)

Symbol spec format:
  <import-path>.<Name>              a package-level func, type, or var
  <import-path>.<Type>.<Method>     a method on a named type

  Examples:
    github.com/Knatte18/loomyard/internal/state.WriteJSON
    github.com/Knatte18/loomyard/internal/hubgeometry.Layout.LyxDir

Flags:
`

// Flag values shared by every mode handler. main() populates these from argv
// before calling dispatch; handlers registered in dispatch read them
// directly rather than each re-parsing flag.CommandLine, since dispatch's
// signature (per this batch's plan) takes only the mode string.
var (
	symbolFlag string
	dirFlag    string
	nFlag      int
	algoFlag   string
	jsonFlag   bool
)

// dispatch runs the handler registered for mode, returning an error for any
// mode not yet implemented by the current batch. Later cards extend this
// switch as each mode's handler lands; the default branch keeps the binary
// compiling and runnable before every mode exists (unknown modes error at
// runtime, not compile time).
func dispatch(mode string) error {
	cfg := config{
		mode:   mode,
		symbol: symbolFlag,
		dir:    dirFlag,
		n:      nFlag,
		algo:   algoFlag,
		json:   jsonFlag,
	}

	switch mode {
	case "refs":
		return runRefs(cfg)
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

// config carries every flag value a mode handler needs, snapshotted from the
// package-level flag vars at dispatch time so handlers take one plain value
// rather than reaching into flag.CommandLine themselves.
type config struct {
	mode   string
	symbol string
	dir    string
	n      int
	algo   string
	json   bool
}

// main parses flags and dispatches to the requested mode's handler, printing
// any resulting error to stderr and exiting non-zero.
func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}

	mode := flag.String("mode", "", "measurement mode: refs, callers, gopls-refs, gopls-cli-refs, callgraph")
	flag.StringVar(&symbolFlag, "symbol", "", "symbol spec: <import-path>.<Name> or <import-path>.<Type>.<Method>")
	flag.StringVar(&dirFlag, "dir", ".", "module root to analyze")
	flag.IntVar(&nFlag, "n", 5, "number of steady-state query repeats")
	flag.StringVar(&algoFlag, "algo", "cha", "call-graph algorithm for callgraph mode (cha, rta, vta)")
	flag.BoolVar(&jsonFlag, "json", false, "emit JSON instead of text")
	flag.Parse()

	if *mode == "" || symbolFlag == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := dispatch(*mode); err != nil {
		fmt.Fprintf(os.Stderr, "codeintel-poc: %v\n", err)
		os.Exit(1)
	}
}
