// takeoverkit.go implements WriteTakeover, the takeover entry's test writer.

package takeoverkit

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// WriteTakeover sets the takeover key of the status JSON at statusPath to entry and writes the file back.
// Every other key stays as it was; the caller commits and pushes the file.
func WriteTakeover(statusPath string, entry shedengine.Takeover) error {
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("read status %s: %w", statusPath, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode status %s: %w", statusPath, err)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode takeover entry: %w", err)
	}
	object["takeover"] = encoded
	out, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status %s: %w", statusPath, err)
	}
	if err := os.WriteFile(statusPath, out, 0o644); err != nil {
		return fmt.Errorf("write status %s: %w", statusPath, err)
	}
	return nil
}
