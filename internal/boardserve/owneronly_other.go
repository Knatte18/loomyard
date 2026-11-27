//go:build !windows

// owneronly_other.go restricts a server file to its owner where the filesystem honors mode bits.

package boardserve

import (
	"fmt"
	"os"
)

// hardenFile sets mode 0600 on path, restricting it to its owner.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
