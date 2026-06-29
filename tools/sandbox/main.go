// main.go implements the sandbox tool entry point, flag parsing, and subcommand
// dispatch. It supports two subcommands: "build" (default, clones the Hub) and
// "suite" (runs the embedded test-scheme agent). The -parent and -reset flags
// live at the top level to preserve back-compat with existing callers.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	hostURL = "https://github.com/Knatte18/lyx-test"
	weftURL = "https://github.com/Knatte18/lyx-test-weft"
	hubName = "lyx-test-HUB"
)

// cloneRun is a testability seam for executing the clone command.
// In tests, this can be replaced to avoid network calls.
var cloneRun = func(parentDir string) error {
	cmd := exec.Command("lyx", "warp", "clone", hostURL, weftURL)
	cmd.Dir = parentDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, isExitError := err.(*exec.ExitError); isExitError {
			// Subprocess printed its own error; just propagate the exit code
			return err
		}
		// Startup error (lyx not found, etc.); add context
		return fmt.Errorf("lyx not found on PATH: %w", err)
	}
	return nil
}

// removeAll is a testability seam for os.RemoveAll, matching the pattern in internal/warpengine/clone.go.
var removeAll = os.RemoveAll

// decideClone determines whether to clone the Hub and performs the necessary actions.
// It returns a non-nil error if any operation fails.
func decideClone(hubPath string, reset bool) error {
	// Check if the Hub directory exists
	_, err := os.Stat(hubPath)
	if err == nil {
		// Hub exists
		if !reset {
			// No-op success
			fmt.Printf("Hub already exists at %s\n", hubPath)
			fmt.Println("Use -reset to rebuild it")
			return nil
		}
		// Reset: remove the Hub and proceed to clone
		if err := removeAll(hubPath); err != nil {
			return fmt.Errorf("remove hub: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error (permission denied, etc.)
		return fmt.Errorf("stat hub path: %w", err)
	}
	// Hub does not exist; proceed to clone

	// Run the clone command
	parentDir := filepath.Dir(hubPath)
	return cloneRun(parentDir)
}

// run is the testable entry point for the sandbox tool. It parses argv, resolves
// the -parent path, and dispatches to the appropriate subcommand. It returns 0
// on success and 1 on any error, writing a "sandbox: ..." message to stderr.
func run(argv []string) int {
	// Top-level flagset holds flags that apply to all subcommands. -reset is
	// build-only but stays here so existing callers (sandbox.cmd -reset) keep
	// working without a subcommand token.
	fs := flag.NewFlagSet("sandbox", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parentDir := fs.String("parent", "", "parent directory where the Hub will be created (required)")
	reset := fs.Bool("reset", false, "rebuild the Hub even if it already exists (build subcommand only)")

	if err := fs.Parse(argv); err != nil {
		// flag.ContinueOnError already wrote the usage message to stderr.
		return 1
	}

	if *parentDir == "" {
		fmt.Fprintln(os.Stderr, "sandbox: -parent is required")
		return 1
	}

	// Resolve -parent to an absolute path so every downstream path computation
	// is stable regardless of the caller's working directory.
	absParent, err := filepath.Abs(*parentDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: resolve parent path: %v\n", err)
		return 1
	}

	// Dispatch on the first remaining positional argument. An absent positional
	// defaults to "build" so the bare `sandbox.cmd` invocation is unchanged.
	subcommand := ""
	if args := fs.Args(); len(args) > 0 {
		subcommand = args[0]
	}

	switch subcommand {
	case "", "build":
		// Default subcommand: clone or reset the Hub.
		hubPath := filepath.Join(absParent, hubName)
		if err := decideClone(hubPath, *reset); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "suite":
		// Parse suite-specific flags from the remaining positionals after "suite".
		sf := flag.NewFlagSet("sandbox suite", flag.ContinueOnError)
		sf.SetOutput(os.Stderr)
		claudeFlag := sf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := sf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := sf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	default:
		fmt.Fprintf(os.Stderr, "sandbox: unknown subcommand %q\n", subcommand)
		return 1
	}

	return 0
}

// main delegates entirely to run so the dispatch logic can be tested without
// spawning a subprocess.
func main() {
	os.Exit(run(os.Args[1:]))
}
