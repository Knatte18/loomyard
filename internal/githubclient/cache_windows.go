//go:build windows

// cache_windows.go implements the Windows half of the token cache's file
// hardening hook: stripping inherited ACLs and installing an explicit
// owner-only DACL on the temp file before it is renamed into place. Go's
// os.WriteFile / os.Chmod permission bits are effectively ignored by Windows,
// so mode bits alone would prove nothing about who can read the cached
// token — the DACL is what actually restricts access.

package githubclient

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// hardenCacheFile sets 0600 mode and installs an owner-only DACL on path via
// SetNamedSecurityInfo, breaking inheritance.
func hardenCacheFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}

	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("GetTokenUser: %w", err)
	}

	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(tokenUser.User.Sid),
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("build owner-only DACL: %w", err)
	}

	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, // owner: leave unchanged
		nil, // group: leave unchanged
		dacl,
		nil, // SACL: leave unchanged
	)
	if err != nil {
		return fmt.Errorf("SetNamedSecurityInfo(%s): %w", path, err)
	}
	return nil
}
