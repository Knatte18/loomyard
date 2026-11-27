// fixtries.go reads the fix-tries file of one subject directory.

package fixtries

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// FileName is the fix-tries file's name in a subject's scratch directory.
const FileName = "fix-tries.json"

// triesFile is the content of the fix-tries file.
type triesFile struct {
	Count int `json:"count"`
}

// Read returns the fix-try count the tries file in dir records, 0 when the file is absent.
// It creates nothing.
func Read(dir string) (int, error) {
	path := filepath.Join(dir, FileName)
	file, _, err := state.ReadJSON[triesFile](path, path+".lock")
	if err != nil {
		return 0, fmt.Errorf("read the fix tries of %s: %w", dir, err)
	}
	return file.Count, nil
}

