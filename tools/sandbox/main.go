// main.go implements the sandbox tool entry point, flag parsing, and subcommand
// dispatch. It supports seven subcommands: "build" (default, clones the Hub),
// "suite" (runs the embedded SANDBOX-CORE-SUITE agent), "mux-suite" (runs the
// embedded SANDBOX-MUX-SUITE agent), "shuttle-suite" (runs the embedded
// SANDBOX-SHUTTLE-SUITE agent), "burler-suite" (runs the embedded
// SANDBOX-BURLER-SUITE agent), "perch-suite" (runs the embedded
// SANDBOX-PERCH-SUITE agent), and "fetch" (collects the agent-written
// report into .scratch). Only -parent and -loomyard live at the top level;
// -reset is a build-subcommand flag, parsed after the "build" token like
// suite/mux-suite/shuttle-suite/burler-suite/perch-suite parse their
// -claude/-prompt flags.

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
	// Top-level flagset holds flags that apply across subcommands. -reset is
	// build-only, so it is parsed by the build subcommand below rather than here.
	fs := flag.NewFlagSet("sandbox", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parentDir := fs.String("parent", "", "parent directory where the Hub will be created (required)")
	loomyard := fs.String("loomyard", "", "loomyard repo root for fetching the sandbox report (required for the fetch subcommand)")

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
	// defaults to "build" so the bare `sandbox-build.cmd` invocation still builds.
	subcommand := ""
	if args := fs.Args(); len(args) > 0 {
		subcommand = args[0]
	}

	switch subcommand {
	case "", "build":
		// Default subcommand: clone or reset the Hub. Parse the build-only -reset
		// flag from the positionals after the "build" token (absent when the bare
		// sandbox-build.cmd is used, so reset defaults to false).
		bf := flag.NewFlagSet("sandbox build", flag.ContinueOnError)
		bf.SetOutput(os.Stderr)
		reset := bf.Bool("reset", false, "rebuild the Hub even if it already exists")

		rest := fs.Args()
		if len(rest) > 0 && rest[0] == "build" {
			rest = rest[1:]
		}
		if err := bf.Parse(rest); err != nil {
			return 1
		}

		hubPath := filepath.Join(absParent, hubName)
		if err := decideClone(hubPath, *reset); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "suite":
		// The suite subcommand only launches the agent; fetching the report is a
		// separate step (fetch), so -loomyard is not required here.

		// Parse suite-specific flags from the remaining positionals after "suite".
		sf := flag.NewFlagSet("sandbox suite", flag.ContinueOnError)
		sf.SetOutput(os.Stderr)
		claudeFlag := sf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := sf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := sf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, mainSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "mux-suite":
		// The mux-suite subcommand mirrors "suite" exactly, but runs the
		// dedicated SANDBOX-MUX-SUITE scheme via the muxSuite spec; fetching the
		// report is the same shared fetch subcommand, so -loomyard is not
		// required here either.

		// Parse mux-suite-specific flags from the remaining positionals after
		// "mux-suite".
		mf := flag.NewFlagSet("sandbox mux-suite", flag.ContinueOnError)
		mf.SetOutput(os.Stderr)
		claudeFlag := mf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := mf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := mf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, muxSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "shuttle-suite":
		// The shuttle-suite subcommand mirrors "suite"/"mux-suite" exactly, but
		// runs the dedicated SANDBOX-SHUTTLE-SUITE scheme via the shuttleSuite
		// spec; fetching the report is the same shared fetch subcommand, so
		// -loomyard is not required here either.

		// Parse shuttle-suite-specific flags from the remaining positionals
		// after "shuttle-suite".
		ssf := flag.NewFlagSet("sandbox shuttle-suite", flag.ContinueOnError)
		ssf.SetOutput(os.Stderr)
		claudeFlag := ssf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := ssf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := ssf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, shuttleSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "burler-suite":
		// The burler-suite subcommand mirrors "suite"/"mux-suite"/
		// "shuttle-suite" exactly, but runs the dedicated
		// SANDBOX-BURLER-SUITE scheme via the burlerSuite spec; fetching the
		// report is the same shared fetch subcommand, so -loomyard is not
		// required here either.

		// Parse burler-suite-specific flags from the remaining positionals
		// after "burler-suite".
		bsf := flag.NewFlagSet("sandbox burler-suite", flag.ContinueOnError)
		bsf.SetOutput(os.Stderr)
		claudeFlag := bsf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := bsf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := bsf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, burlerSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "perch-suite":
		// The perch-suite subcommand mirrors "suite"/"mux-suite"/
		// "shuttle-suite"/"burler-suite" exactly, but runs the dedicated
		// SANDBOX-PERCH-SUITE scheme via the perchSuite spec; fetching the
		// report is the same shared fetch subcommand, so -loomyard is not
		// required here either.

		// Parse perch-suite-specific flags from the remaining positionals
		// after "perch-suite".
		psf := flag.NewFlagSet("sandbox perch-suite", flag.ContinueOnError)
		psf.SetOutput(os.Stderr)
		claudeFlag := psf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := psf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := psf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, perchSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "fetch":
		// fetch collects the agent-written report into the loomyard repo,
		// so it cannot run without knowing that repo's root.
		if *loomyard == "" {
			fmt.Fprintln(os.Stderr, "sandbox: -loomyard is required for the fetch subcommand")
			return 1
		}
		// filepath.Clean strips the trailing "."/separator that sandbox-fetch.cmd
		// passes via "%~dp0." before resolving to an absolute path.
		absLoomyard, err := filepath.Abs(filepath.Clean(*loomyard))
		if err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: resolve loomyard path: %v\n", err)
			return 1
		}

		if err := runFetch(absParent, absLoomyard); err != nil {
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
