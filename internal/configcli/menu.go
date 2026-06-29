// menu.go — interactive config module picker.
//
// Implements the interactive numbered menu for bare `lyx config` (no module arg).

package configcli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/paths"
)

// menu presents an interactive picker of available config modules.
//
// It prints a numbered list of moduleNames(), each marked "(configured)" if its
// YAML file exists at filepath.Join(baseDir, "_lyx", "config", name+".yaml"),
// else "(default)".
//
// Reads one line from in with bufio.NewReader.ReadString('\n').
// Handles 'q' to quit (return 0).
// Parses selection as 1-indexed number, validates range, routes to editOne on valid choice.
// Returns the exit code from editOne or an error code (1) on invalid input.
func menu(l *paths.Layout, baseDir string, in io.Reader, out io.Writer, edit configengine.EditorFunc, sync syncFunc) int {
	// Get the list of available modules.
	names := configreg.Names()

	// Print numbered picker with configured/default status.
	for i, name := range names {
		// Check if config file exists.
		configPath := paths.ConfigFile(baseDir, name)
		_, err := os.Stat(configPath)
		status := "(default)"
		if err == nil {
			status = "(configured)"
		}

		fmt.Fprintf(out, "%d) %s %s\n", i+1, name, status)
	}

	// Read user selection from input.
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(out, "read input: %v\n", err)
		return 1
	}

	line = strings.TrimSpace(line)

	// Handle 'q' to quit.
	if line == "q" {
		return 0
	}

	// Parse the selection as a number.
	num, err := strconv.Atoi(line)
	if err != nil {
		fmt.Fprintf(out, "invalid input: must be a number or 'q'\n")
		return 1
	}

	// Validate range (1-indexed).
	if num < 1 || num > len(names) {
		fmt.Fprintf(out, "invalid selection: %d (must be 1-%d or 'q')\n", num, len(names))
		return 1
	}

	// Route to editOne with the chosen module name.
	chosenModule := names[num-1]
	return editOne(baseDir, out, chosenModule, edit, sync)
}
