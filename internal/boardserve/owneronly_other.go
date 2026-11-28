//go:build !windows

// owneronly_other.go restricts a file to its user where the file system honours mode bits.

package boardserve

import (
	"fmt"
	"os"
)

// hardenFile makes the file at path readable and writable by its user alone.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict %s to its user: %w", path, err)
	}
	return nil
}
