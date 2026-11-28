//go:build !windows

// owneronly_other.go restricts a file to its owner by its permission bits.

package boardserve

import (
	"fmt"
	"os"
)

// hardenFile sets mode 0600 on path.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
