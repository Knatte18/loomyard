// stamp.go computes the build key and reads and writes the build stamp file.

package hubreconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// stamp is the build stamp file: the key of the build that last reconciled the hub, with its identity in readable form for the log.
type stamp struct {
	BuildKey string `json:"build_key"`
	buildvcs.Identity
}

// BuildKey returns the hex SHA-256 of the VCS revision, the modified flag and the registry fingerprint, each length-prefixed.
func BuildKey(id buildvcs.Identity, fingerprint string) string {
	hash := sha256.New()
	for _, field := range []string{id.Revision, fmt.Sprint(id.Modified), fingerprint} {
		fmt.Fprintf(hash, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// runningStamp returns the stamp the running binary would write.
func runningStamp() stamp {
	id := buildvcs.Running()
	return stamp{BuildKey: BuildKey(id, configreg.Fingerprint()), Identity: id}
}

// readStamp reads the stamp at path; an absent file reports not found with no error.
func readStamp(path string) (stamp, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return stamp{}, false, nil
	}
	if err != nil {
		return stamp{}, false, fmt.Errorf("read build stamp: %w", err)
	}
	var s stamp
	if err := json.Unmarshal(data, &s); err != nil {
		return stamp{}, false, fmt.Errorf("parse build stamp %s: %w", path, err)
	}
	return s, true, nil
}

// writeStamp writes the stamp atomically, creating its dir.
func writeStamp(path string, s stamp) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode build stamp: %w", err)
	}
	return fsx.AtomicWriteBytes(path, append(data, '\n'))
}
