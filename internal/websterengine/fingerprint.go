// fingerprint.go implements fingerprint, webster's own plan-identity hash builder: a verbatim webster-local copy of builderengine.Fingerprint (builderengine/fingerprint.go), since builder's own Fingerprint has an in-tree builder caller (frozen, per the Shared Decision builder-is-frozen-copy-not-move) and cannot be moved.
// webster's state.json records the fingerprint at first init,
// and every later run/begin-batch entry recomputes and compares it against the on-disk plan, so a --fresh crash/resume guard (wired into state and runlevel in batch 7) can detect a stale plan across a crash/resume boundary.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fingerprint computes a SHA-256 digest over every "*.md" file's name and
// contents in planDir (sorted lexically). Non-.md entries are ignored.
func fingerprint(planDir string) (string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return "", fmt.Errorf("websterengine: fingerprint %s: %w", planDir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return "", fmt.Errorf("websterengine: fingerprint %s: read %s: %w", planDir, name, err)
		}
		// Trailing NUL separates name and contents to prevent concatenation collisions.
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
