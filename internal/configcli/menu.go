// menu.go — interactive config module picker.
//
// Implements the numbered picker behind `lyx config menu`.

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
)

// menu presents an interactive picker of available config modules.
// It prints a numbered list from configreg.Names(), each marked "(configured)" or "(default)"
// by whether its config file exists.
// It reads one line from in and quits with 0 on 'q'.
// Otherwise it parses the line as a 1-indexed number and routes a valid choice to editOne,
// returning 1 on invalid input.
func menu(baseDir string, in io.Reader, out io.Writer, edit configengine.EditorFunc, sync syncFunc) int {
	names := configreg.Names()

	for i, name := range names {
		status := "(default)"
		if _, err := os.Stat(configengine.ConfigFile(baseDir, name)); err == nil {
			status = "(configured)"
		}
		fmt.Fprintf(out, "%d) %s %s\n", i+1, name, status)
	}

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(out, "read input: %v\n", err)
		return 1
	}
	line = strings.TrimSpace(line)

	if line == "q" {
		return 0
	}

	num, err := strconv.Atoi(line)
	if err != nil {
		fmt.Fprintf(out, "invalid input: must be a number or 'q'\n")
		return 1
	}
	if num < 1 || num > len(names) {
		fmt.Fprintf(out, "invalid selection: %d (must be 1-%d or 'q')\n", num, len(names))
		return 1
	}

	return editOne(baseDir, out, names[num-1], edit, sync)
}
