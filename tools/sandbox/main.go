// main.go implements the sandbox tool entry point, flag parsing, and subcommand
// dispatch. It supports ten subcommands: "build" (default, clones the Hub),
// "suite" (runs the embedded SANDBOX-CORE-SUITE agent), "reed-suite" (runs the
// embedded SANDBOX-REED-SUITE agent), "shuttle-suite" (runs the embedded
// SANDBOX-SHUTTLE-SUITE agent), "burler-suite" (runs the embedded
// SANDBOX-BURLER-SUITE agent), "perch-suite" (runs the embedded
// SANDBOX-PERCH-SUITE agent), "builder-suite" (runs the embedded
// SANDBOX-BUILDER-SUITE agent), "webster-suite" (runs the embedded
// SANDBOX-WEBSTER-SUITE agent), "fabric-suite" (clones the dedicated fabric
// hub if absent, then runs the embedded SANDBOX-FABRIC-SUITE agent), and
// "fetch" (collects the agent-written report into .scratch). Only -parent and
// -loomyard live at the top level; -reset is a build-subcommand flag, parsed
// after the "build" token like suite/reed-suite/shuttle-suite/burler-suite/
// perch-suite/builder-suite/webster-suite/fabric-suite parse their
// -claude/-prompt flags.

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
	hostURL = "https://github.com/Knatte18/lyx-test"
	weftURL = "https://github.com/Knatte18/lyx-test-weft"
	hubName = "lyx-test-HUB"

	// fabric-suite runs against its own dedicated hub, never the shared
	// lyx-test-HUB above -- the dedicated hub hosts fabric's stricter
	// "main-weft"-suffixed branch-naming suite, which the shared hub's fixtures
	// do not exercise.
	fabricHostURL   = "https://github.com/Knatte18/lyx-fabric-test"
	fabricWeftURL   = "https://github.com/Knatte18/lyx-fabric-test-weft"
	fabricHubName   = "lyx-fabric-test-HUB"
	fabricHostDir   = "lyx-fabric-test"
	fabricSuiteFile = "SANDBOX-FABRIC-SUITE.md"
	fabricSuiteAsk  = "Read ./SANDBOX-FABRIC-SUITE.md and follow the instructions in it exactly."
)

//go:embed SANDBOX-FABRIC-SUITE.md
var fabricSandboxSuiteMD string

// cloneRun is a testability seam for executing the clone command.
var cloneRun = func(parentDir, lyxPath string) error {
	cmd := exec.Command(lyxPath, "fabric", "clone", hostURL, weftURL)
	cmd.Dir = parentDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, isExitError := err.(*exec.ExitError); isExitError {
			// Subprocess printed its own error; just propagate the exit code
			return err
		}
		// Startup error (resolved binary vanished, permission denied, etc.); add
		// context pointing at deploy-dev as an alternative to the resolved path.
		return fmt.Errorf("failed to start resolved lyx binary %s (deploy it, or run deploy-dev): %w", lyxPath, err)
	}
	return nil
}

// fabricCloneRun is a testability seam for executing `lyx fabric clone`
// against the dedicated fabric sandbox repos.
var fabricCloneRun = func(parentDir, lyxPath string) error {
	cmd := exec.Command(lyxPath, "fabric", "clone", fabricHostURL, fabricWeftURL)
	cmd.Dir = parentDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, isExitError := err.(*exec.ExitError); isExitError {
			// Subprocess printed its own error; just propagate the exit code
			return err
		}
		// Startup error (resolved binary vanished, permission denied, etc.); add
		// context pointing at deploy-dev as an alternative to the resolved path.
		return fmt.Errorf("failed to start resolved lyx binary %s (deploy it, or run deploy-dev): %w", lyxPath, err)
	}
	return nil
}

// removeAll is a testability seam for os.RemoveAll, matching the pattern in internal/fabricengine/clone.go.
var removeAll = os.RemoveAll

// decideClone determines whether to clone the Hub. Returns error on failure.
func decideClone(hubPath string, reset bool) error {
	_, err := os.Stat(hubPath)
	if err == nil {
		if !reset {
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

	lyxPath, _, err := resolveLyx()
	if err != nil {
		return err
	}

	// Run the clone command
	parentDir := filepath.Dir(hubPath)
	return cloneRun(parentDir, lyxPath)
}

// decideFabricClone materializes the dedicated fabric sandbox hub if it does
// not already exist. No -reset flag: reuses existing hub state.
func decideFabricClone(hubPath string) error {
	if _, err := os.Stat(hubPath); err == nil {
		fmt.Printf("Fabric hub already exists at %s\n", hubPath)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat fabric hub path: %w", err)
	}

	lyxPath, _, err := resolveLyx()
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(hubPath)
	return fabricCloneRun(parentDir, lyxPath)
}

// runFabricSuite fingerprints lyx, writes SANDBOX-FABRIC-SUITE.md into the
// fabric hub, and launches an interactive Claude session.
func runFabricSuite(parentDir, claudeOverride, promptOverride string) error {
	hostRepoDir := filepath.Join(parentDir, fabricHubName, fabricHostDir)

	if _, err := os.Stat(hostRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("fabric hub host repo not found at %s -- run sandbox/fabric-suite.cmd, which clones it first", hostRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat fabric host repo %s: %w", hostRepoDir, err)
	}

	lyxPath, source, err := resolveLyx()
	if err != nil {
		return err
	}

	info, err := binaryFingerprint(lyxPath, source)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	suitePath := filepath.Join(hostRepoDir, fabricSuiteFile)
	if err := os.WriteFile(suitePath, []byte(renderScheme(info, fabricSandboxSuiteMD)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fabricSuiteFile, err)
	}

	if err := ensureGitExclude(hostRepoDir, fabricSuiteFile); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	reportPath := filepath.Join(hostRepoDir, reportFileName)
	if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", reportFileName, err)
	}
	if err := ensureGitExclude(hostRepoDir, reportFileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	claudePath := claudeOverride
	if claudePath == "" {
		claudePath, err = lookPath("claude")
		if err != nil {
			return fmt.Errorf("claude not found on PATH: %w", err)
		}
	}

	instruction := promptOverride
	if instruction == "" {
		instruction = fabricSuiteAsk
	}

	binDir := ""
	if source == sourceDev {
		binDir = filepath.Dir(lyxPath)
	}

	code := launchAgent(hostRepoDir, claudePath, instruction, binDir)
	fmt.Fprintf(os.Stderr,
		"sandbox: agent session ended (exit code %d). Run sandbox/fetch.cmd to collect findings into .scratch.\n",
		code)

	return nil
}

// run is the testable entry point. Parses argv, resolves -parent, and
// dispatches to the appropriate subcommand. Returns 0 on success, 1 on error.
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

	case "perch-suite":
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

	case "builder-suite":
		bdf := flag.NewFlagSet("sandbox builder-suite", flag.ContinueOnError)
		bdf.SetOutput(os.Stderr)
		claudeFlag := bdf.String("claude", "", "path to the claude binary (default: resolve from PATH)")
		promptFlag := bdf.String("prompt", "", "instruction string passed to the agent (default: built-in)")

		remaining := fs.Args()[1:]
		if err := bdf.Parse(remaining); err != nil {
			return 1
		}

		if err := runSuite(absParent, *claudeFlag, *promptFlag, builderSuite); err != nil {
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

		fabricHubPath := filepath.Join(absParent, fabricHubName)
		if err := decideFabricClone(fabricHubPath); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
			return 1
		}

		if err := runFabricSuite(absParent, *claudeFlag, *promptFlag); err != nil {
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
