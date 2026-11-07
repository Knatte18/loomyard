// main.go implements the sandbox tool entry point, flag parsing, and subcommand dispatch.
// The build subcommand is the default: it clones the Hub, and with -reset it removes an existing Hub first.
// Each suite subcommand runs one embedded agent suite against the shared Hub.
// Every suite uses the same Hub, and none has a hub or clone step of its own.
// The fetch subcommand collects the agent-written report into .scratch.
// Only -parent and -loomyard are top-level flags, parsed before the subcommand.
// -reset is a build flag, and each suite subcommand parses its own -claude and -prompt flags.
// Those flags follow the subcommand token.

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	warpURL = "https://github.com/Knatte18/lym-test"
	weftURL = "https://github.com/Knatte18/lym-test-weft"
	hubName = "lym-test-LYMHUB"
	// sandboxShortname is the shortname the sandbox hub is cloned with.
	sandboxShortname = "lyt"
)

// cloneRun is a testability seam for executing the clone command.
var cloneRun = func(parentDir, lymPath string) error {
	cmd := exec.Command(lymPath, "fabric", "clone", "--shortname", sandboxShortname, weftURL, warpURL)
	cmd.Dir = parentDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, isExitError := err.(*exec.ExitError); isExitError {
			// The subprocess printed its own error; propagate its exit code.
			return err
		}
		// Startup error (resolved binary vanished, permission denied, etc.).
		// Add context that points at deploy-dev as an alternative to the resolved path.
		return fmt.Errorf("failed to start resolved lym binary %s (deploy it, or run deploy-dev): %w", lymPath, err)
	}
	return nil
}

// removeAll is a testability seam for os.RemoveAll.
var removeAll = os.RemoveAll

// decideClone clones the Hub at hubPath when none exists or reset is set; with reset, it removes the existing Hub first.
// It returns an error if the removal or the clone fails.
func decideClone(hubPath string, reset bool) error {
	_, err := os.Stat(hubPath)
	if err == nil {
		if !reset {
			fmt.Printf("Hub already exists at %s\n", hubPath)
			fmt.Println("Use -reset to rebuild it")
			return nil
		}
		if err := removeAll(hubPath); err != nil {
			return fmt.Errorf("remove hub: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error (permission denied, etc.)
		return fmt.Errorf("stat hub path: %w", err)
	}

	lymPath, _, err := resolveLym()
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(hubPath)
	return cloneRun(parentDir, lymPath)
}

// run is the testable entry point: it parses argv, resolves -parent, and dispatches to the subcommand.
// It returns 0 on success and 1 on error.
func run(argv []string) int {
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

	absParent, err := filepath.Abs(*parentDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: resolve parent path: %v\n", err)
		return 1
	}

	subcommand := ""
	if args := fs.Args(); len(args) > 0 {
		subcommand = args[0]
	}

	switch subcommand {
	case "", "build":
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

	case "reed-suite":
		rf := flag.NewFlagSet("sandbox reed-suite", flag.ContinueOnError)
		rf.SetOutput(os.Stderr)
		claudeFlag := rf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := rf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := rf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, reedSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "reed-watch-suite":
		rwf := flag.NewFlagSet("sandbox reed-watch-suite", flag.ContinueOnError)
		rwf.SetOutput(os.Stderr)
		claudeFlag := rwf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := rwf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := rwf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, reedWatchSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "shuttle-suite":
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

	case "creel-suite":
		csf := flag.NewFlagSet("sandbox creel-suite", flag.ContinueOnError)
		csf.SetOutput(os.Stderr)
		claudeFlag := csf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := csf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := csf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, creelSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "burler-suite":
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

	case "webster-suite":
		wsf := flag.NewFlagSet("sandbox webster-suite", flag.ContinueOnError)
		wsf.SetOutput(os.Stderr)
		claudeFlag := wsf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := wsf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := wsf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, websterSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "fabric-suite":
		ffsf := flag.NewFlagSet("sandbox fabric-suite", flag.ContinueOnError)
		ffsf.SetOutput(os.Stderr)
		claudeFlag := ffsf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := ffsf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := ffsf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, fabricSuite); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

	case "fetch":
		if *loomyard == "" {
			fmt.Fprintln(os.Stderr, "sandbox: -loomyard is required for the fetch subcommand")
			return 1
		}
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

// main delegates to run so the dispatch logic can be tested.
func main() {
	os.Exit(run(os.Args[1:]))
}
