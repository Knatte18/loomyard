//go:build !windows

// owneronly_other.go is the non-Windows half of hardenFile.
// These platforms honor the permission bits, so 0600 is enough; owneronly_windows.go also installs an ACL.

package boardserve

import (
	"fmt"
	"os"
)

// hardenFile restricts the file at path to its owner.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
