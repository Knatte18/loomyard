// holds.go reads and writes the per-subject fix-tries file.

package adoptcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// triesFileName records, in a subject's scratch directory, how many fix tasks the subject's check has filed.
const triesFileName = "fix-tries.json"

// triesFile is the content of fix-tries.json.
type triesFile struct {
	Count int `json:"count"`
}

// readTries returns the fix-try count the tries file in dir records, 0 when the file is absent.
func readTries(dir string) (int, error) {
	path := filepath.Join(dir, triesFileName)
	file, _, err := state.ReadJSON[triesFile](path, path+".lock")
	if err != nil {
		return 0, fmt.Errorf("read the fix tries of %s: %w", dir, err)
	}
	return file.Count, nil
}

// writeTries records count as the fix-try count in the tries file in dir.
func writeTries(dir string, count int) error {
	path := filepath.Join(dir, triesFileName)
	err := state.UpdateJSON(path, path+".lock", func(current triesFile, _ bool) (triesFile, error) {
		current.Count = count
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("record the fix tries of %s: %w", dir, err)
	}
	return nil
}
