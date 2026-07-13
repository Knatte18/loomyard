// fingerprint.go implements Fingerprint, the plan-identity hash builder's
// run state uses to detect a stale on-disk plan across a crash/resume
// boundary: state.json records the fingerprint at first init, and every
// later run/spawn-batch entry recomputes and compares it, per the
// discussion's plan-fingerprint decision.

package builderengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint computes a SHA-256 digest over every "*.md" file's name and
// contents in planDir. Non-.md entries (including subdirectories) are
// ignored, since only markdown plan files carry plan-format v2 content.
// Filenames are sorted lexically before hashing so the result does not
// depend on directory read order; each entry contributes
// name + "\x00" + contents + "\x00" to the hash, so a rename (the name
// changes) and a content edit (the name stays, the bytes change) each
// change the result independently. Adding or removing a batch file changes
// the set of names hashed, and so also changes the result. Returns the
// digest as lowercase hex.
func Fingerprint(planDir string) (string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return "", fmt.Errorf("builder: fingerprint %s: %w", planDir, err)
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
			return "", fmt.Errorf("builder: fingerprint %s: read %s: %w", planDir, name, err)
		}
		// The trailing NUL after each of name and contents prevents two
		// different (name, contents) pairs from concatenating to the same
		// byte stream (e.g. name "ab" + contents "c" vs. name "a" +
		// contents "bc").
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
