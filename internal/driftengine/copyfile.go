// copyfile.go — the single-file copy the judge snapshot uses.

package driftengine

import (
	"fmt"
	"os"
	"path/filepath"
)

// copyFile copies the file at from to to, creating to's directory.
func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("drift: read %s: %w", from, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("drift: create %s: %w", filepath.Dir(to), err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		return fmt.Errorf("drift: write %s: %w", to, err)
	}
	return nil
}
