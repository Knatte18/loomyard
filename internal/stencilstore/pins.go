// pins.go hashes the installed board copy of each registry stencil, so a caller can pin what a run may render.
// It also reports which pinned stencils changed since.

package stencilstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
)

// Pins returns the BodyHash of each registry stencil's board copy under baseDir, keyed by stencil name.
// A stencil with no board copy is left out, since nothing can render it.
// A board copy that exists but cannot be read is an error naming the stencil.
func Pins(baseDir string, registry Registry) (map[string]string, error) {
	pins := make(map[string]string)
	for _, name := range registry.Names() {
		content, found, err := readBoardCopy(baseDir, name)
		if err != nil {
			return nil, err
		}
		if found {
			pins[name] = BodyHash(content)
		}
	}
	return pins, nil
}

// ChangedPins returns the names of the stencils in pins that changed, in name order.
// A stencil changed when its board copy under baseDir no longer has its pinned BodyHash.
// A pinned stencil whose board copy is gone counts as changed.
// A stencil installed after the pins were taken is not in pins, so it is never reported.
// A board copy that exists but cannot be read is an error naming the stencil.
func ChangedPins(baseDir string, pins map[string]string) ([]string, error) {
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)

	var changed []string
	for _, name := range names {
		content, found, err := readBoardCopy(baseDir, name)
		if err != nil {
			return nil, err
		}
		if !found || BodyHash(content) != pins[name] {
			changed = append(changed, name)
		}
	}
	return changed, nil
}

// readBoardCopy reads name's board copy under baseDir, reporting found false when the file does not exist.
func readBoardCopy(baseDir, name string) ([]byte, bool, error) {
	content, err := os.ReadFile(Path(baseDir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("stencilstore: read the board copy of %q: %w", name, err)
	}
	return content, true, nil
}
