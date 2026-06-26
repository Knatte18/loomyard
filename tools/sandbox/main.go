// main.go implements the sandbox tool for building the lyx-test dogfood Hub.
// It drives the on-PATH lyx binary to clone a Hub by invoking lyx warp clone.

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

// removeAll is a testability seam for os.RemoveAll, matching the pattern in internal/warp/clone.go.
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

func main() {
	parentDir := flag.String("parent", "", "parent directory where the Hub will be created (required)")
	reset := flag.Bool("reset", false, "rebuild the Hub even if it already exists")
	flag.Parse()

	if *parentDir == "" {
		fmt.Fprintln(os.Stderr, "sandbox: -parent is required (error if empty)")
		os.Exit(1)
	}

	// Resolve -parent to an absolute path
	absParent, err := filepath.Abs(*parentDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: resolve parent path: %v\n", err)
		os.Exit(1)
	}

	// Compute hub path
	hubPath := filepath.Join(absParent, hubName)

	// Decide and execute
	if err := decideClone(hubPath, *reset); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		os.Exit(1)
	}
}
