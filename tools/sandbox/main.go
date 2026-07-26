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
	// lyx-test-HUB above -- warp/weft's sandbox state must stay untouched by
	// fabric's parallel-build testing and vice versa.
	fabricHostURL   = "https://github.com/Knatte18/lyx-fabric-test"
	fabricWeftURL   = "https://github.com/Knatte18/lyx-fabric-test-weft"
	fabricHubName   = "lyx-fabric-test-HUB"
	fabricHostDir   = "lyx-fabric-test"
	fabricSuiteFile = "SANDBOX-FABRIC-SUITE.md"
	fabricSuiteAsk  = "Read ./SANDBOX-FABRIC-SUITE.md and follow the instructions in it exactly."
)

//go:embed SANDBOX-FABRIC-SUITE.md
var fabricSandboxSuiteMD string

// cloneRun is a testability seam for executing the clone command. lyxPath is
// the already-resolved lyx binary (see decideClone) so cloneRun itself
// performs no PATH lookup; in tests, this seam can be replaced to avoid
// network calls.
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

// fabricCloneRun is a testability seam for executing `lyx fabric clone` against
// the dedicated fabric sandbox repos. It mirrors cloneRun's shape exactly --
// same subprocess-error-vs-startup-error handling, same resolved-binary
// parameter instead of a bare-PATH lookup -- but shells "fabric clone" (never
// "warp clone") and targets fabric's own dedicated hub instead of the shared
// lyx-test-HUB.
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

	// Resolve the binary lazily, here at the clone step, rather than in run()'s
	// build case or as a decideClone parameter: this keeps the no-op path above
	// (Hub exists, no -reset) succeeding even when neither a dev binary nor a
	// PATH lyx is resolvable, matching today's behaviour for that path.
	lyxPath, _, err := resolveLyx()
	if err != nil {
		return err
	}

	// Run the clone command
	parentDir := filepath.Dir(hubPath)
	return cloneRun(parentDir, lyxPath)
}

// decideFabricClone materializes the dedicated fabric sandbox hub at hubPath if
// it does not already exist. Unlike decideClone, this has no -reset flag: the
// "fabric-suite" subcommand is not the "build" subcommand, so it always reuses
// whatever state a prior session left the hub in rather than tearing it down.
func decideFabricClone(hubPath string) error {
	if _, err := os.Stat(hubPath); err == nil {
		// Hub already exists; reuse it as-is.
		fmt.Printf("Fabric hub already exists at %s\n", hubPath)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat fabric hub path: %w", err)
	}

	// Resolve the binary lazily, here at the clone step, mirroring decideClone's
	// rationale exactly (the "hub already exists" no-op path above must keep
	// succeeding even when no lyx is resolvable).
	lyxPath, _, err := resolveLyx()
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(hubPath)
	return fabricCloneRun(parentDir, lyxPath)
}

// runFabricSuite fingerprints the deployed lyx binary, writes the rendered
// SANDBOX-FABRIC-SUITE.md into the dedicated fabric hub's host repo, registers
// it (and the report file) in .git/info/exclude, and launches an interactive
// Claude session against it.
//
// This duplicates the fingerprint/write/exclude/launch mechanics of suite.go's
// runSuite (binaryFingerprint, renderScheme, ensureGitExclude, lookPath,
// launchAgent, reportFileName -- all same-package, so no import is needed)
// rather than calling runSuite directly: runSuite's hostRepoDir is hardcoded to
// the shared hubName/hostDirName constants, and fabric needs its own dedicated
// hub instead. fabric-suite also has no live-substrate teardown step (unlike
// the muxTeardown specs in suite.go) because none of its scenarios boot mux.
func runFabricSuite(parentDir, claudeOverride, promptOverride string) error {
	hostRepoDir := filepath.Join(parentDir, fabricHubName, fabricHostDir)

	// Guard against a missing Hub so the operator gets a clear, actionable message
	// rather than a confusing downstream file-write failure. Under normal use this
	// cannot happen -- the "fabric-suite" case runs decideFabricClone first -- but
	// a direct call (e.g. from a test) should still fail loudly.
	if _, err := os.Stat(hostRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("fabric hub host repo not found at %s -- run sandbox-fabric-suite.cmd, which clones it first", hostRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat fabric host repo %s: %w", hostRepoDir, err)
	}

	// Resolve lyx via resolveLyx (derived .dev-bin first, PATH fallback) so the
	// fingerprint captures the exact binary the session will run, and so the
	// dev/prod distinction can be stamped and threaded to the agent's PATH below.
	lyxPath, source, err := resolveLyx()
	if err != nil {
		return err
	}

	info, err := binaryFingerprint(lyxPath, source)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	// Write the rendered scheme (fingerprint header + body) into the host repo,
	// overwriting any copy left from a previous run so every session starts fresh.
	suitePath := filepath.Join(hostRepoDir, fabricSuiteFile)
	if err := os.WriteFile(suitePath, []byte(renderScheme(info, fabricSandboxSuiteMD)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fabricSuiteFile, err)
	}

	// Exclude the scheme file from git tracking in the host repo so it does not
	// show up as an untracked change when the agent runs git status or similar.
	if err := ensureGitExclude(hostRepoDir, fabricSuiteFile); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	// Remove any report left over from a previous session so a fetch run after
	// this session cannot pick up stale findings under a fresh fingerprint; if
	// the agent writes nothing, fetch then correctly surfaces the missing-report
	// error instead.
	reportPath := filepath.Join(hostRepoDir, reportFileName)
	if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", reportFileName, err)
	}
	if err := ensureGitExclude(hostRepoDir, reportFileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	// Resolve the claude binary: honour an explicit override flag, otherwise
	// search PATH -- the agent must be installed like any other tool.
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

	// Only a resolved dev binary gets its directory prepended to the agent's
	// PATH (see the agent-path-prepend-launchagent-only Shared Decision); a
	// prod resolution leaves binDir empty so launchAgent inherits the
	// environment unchanged, exactly as before.
	binDir := ""
	if source == sourceDev {
		binDir = filepath.Dir(lyxPath)
	}

	// Launch the interactive agent session. An interactive claude session never
	// self-terminates, so its manual exit is expected and its non-zero exit code
	// is NORMAL -- it must not be treated as a failure. Fetching the report is a
	// separate step, so print guidance and return nil regardless of the code.
	code := launchAgent(hostRepoDir, claudePath, instruction, binDir)
	fmt.Fprintf(os.Stderr,
		"sandbox: agent session ended (exit code %d). Run sandbox-fetch.cmd to collect findings into .scratch.\n",
		code)

	return nil
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

	case "reed-suite":
		// The reed-suite subcommand mirrors "suite" exactly, but runs the
		// dedicated SANDBOX-REED-SUITE scheme via the reedSuite spec; fetching the
		// report is the same shared fetch subcommand, so -loomyard is not
		// required here either.

		// Parse reed-suite-specific flags from the remaining positionals after
		// "reed-suite".
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
		// The shuttle-suite subcommand mirrors "suite"/"reed-suite" exactly, but
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
		// The burler-suite subcommand mirrors "suite"/"reed-suite"/
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
		// The perch-suite subcommand mirrors "suite"/"reed-suite"/
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

	case "builder-suite":
		// The builder-suite subcommand mirrors "suite"/"reed-suite"/
		// "shuttle-suite"/"burler-suite"/"perch-suite" exactly, but runs the
		// dedicated SANDBOX-BUILDER-SUITE scheme via the builderSuite spec;
		// fetching the report is the same shared fetch subcommand, so
		// -loomyard is not required here either.

		// Parse builder-suite-specific flags from the remaining positionals
		// after "builder-suite".
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
		// The webster-suite subcommand mirrors "suite"/"reed-suite"/
		// "shuttle-suite"/"burler-suite"/"perch-suite"/"builder-suite"
		// exactly, but runs the dedicated SANDBOX-WEBSTER-SUITE scheme via
		// the websterSuite spec; fetching the report is the same shared
		// fetch subcommand, so -loomyard is not required here either.

		// Parse webster-suite-specific flags from the remaining positionals
		// after "webster-suite".
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
		// Unlike every other *-suite subcommand above, fabric-suite runs against
		// its own dedicated hub rather than the shared lyx-test-HUB, and it has
		// no separate "build" step: it clones that hub itself (idempotently) via
		// decideFabricClone before launching the agent.

		// Parse fabric-suite-specific flags from the remaining positionals
		// after "fabric-suite".
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
