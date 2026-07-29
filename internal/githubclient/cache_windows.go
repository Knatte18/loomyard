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

// hardenCacheFile sets the 0600 mode (for parity with the non-Windows hook,
// though that mode alone is not what restricts access on this platform) and
// then installs an explicit owner-only DACL on path via SetNamedSecurityInfo
// with PROTECTED_DACL_SECURITY_INFORMATION. That flag breaks inheritance
// rather than merely supplementing it — an inherited ACL from the parent
// directory would otherwise continue to grant access regardless of the DACL
// this function installs.
func hardenCacheFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}

	// GetCurrentProcessToken returns a pseudo-token that needs no closing;
	// its user SID is the sole grantee of the DACL below.
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("GetTokenUser: %w", err)
	}

	// Build a DACL granting full control to the current owner and nothing
	// else — an empty DACL together with PROTECTED_DACL_SECURITY_INFORMATION
	// would deny everyone, including the owner.
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
