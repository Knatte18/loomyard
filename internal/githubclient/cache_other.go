//go:build !windows

// cache_other.go implements the non-Windows half of the token cache's file
// hardening hook. On these platforms os.Chmod's mode bits are honored by the
// filesystem, so the 0600 applied here is sufficient and no ACL work is
// needed — unlike cache_windows.go, which must additionally strip inherited
// ACLs because mode bits alone are ignored there.

package githubclient

import (
	"fmt"
	"os"
)

// hardenCacheFile applies the 0600 mode to path, restricting it to the
// owning user. No further hardening is required on this platform.
func hardenCacheFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
