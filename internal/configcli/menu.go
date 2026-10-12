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

// layersHolding returns the names of the layers that hold a file for module, joined by ", ", or "template" when none does.
func (d configDirs) layersHolding(module configreg.Module) string {
	var held []string
	for _, layer := range layerNames {
		path, applies := configengine.LayerFile(d.engineDirs(), module.Spec(), layer)
		if !applies {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			held = append(held, string(layer))
		}
	}
	if len(held) == 0 {
		return string(configengine.LayerTemplate)
	}
	return strings.Join(held, ", ")
}

// menu presents an interactive picker of available config modules.
// It prints a numbered list from configreg.Names().
// Each module is marked with the layers that hold a file, as "(default, local)", or "(template)" when none does.
// It reads one line from in and quits with 0 on 'q'.
// Otherwise it parses the line as a 1-indexed number and routes a valid choice to editOne,
// returning 1 on invalid input.
func menu(dirs configDirs, in io.Reader, out io.Writer, edit configengine.EditorFunc, sync syncFunc, commit hubCommitFunc) int {
	modules := configreg.Modules()

	for i, mod := range modules {
		fmt.Fprintf(out, "%d) %s (%s)\n", i+1, mod.Name, dirs.layersHolding(mod))
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
	if num < 1 || num > len(modules) {
		fmt.Fprintf(out, "invalid selection: %d (must be 1-%d or 'q')\n", num, len(modules))
		return 1
	}

	return editOne(dirs, out, modules[num-1].Name, edit, sync, commit)
}
