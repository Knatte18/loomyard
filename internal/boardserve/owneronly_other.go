//go:build !windows

// owneronly_other.go restricts a file of the board server to its owner where the file mode does that.

package boardserve

import (
	"fmt"
	"os"
)

// hardenFile makes path readable and writable by its owner alone.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
