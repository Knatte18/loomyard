//go:build windows

// owneronly_windows.go restricts a server file to its owner by an owner-only DACL that breaks inheritance.
// Windows ignores Go's permission bits, so mode bits alone restrict nothing.

package boardserve

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// hardenFile sets mode 0600 on path and replaces its DACL with one that grants the current user alone.
func hardenFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("GetTokenUser: %w", err)
	}
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(tokenUser.User.Sid),
		},
	}}, nil)
	if err != nil {
		return fmt.Errorf("build owner-only DACL: %w", err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("SetNamedSecurityInfo(%s): %w", path, err)
	}
	return nil
}
