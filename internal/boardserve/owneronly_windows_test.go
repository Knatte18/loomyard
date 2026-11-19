//go:build windows

// owneronly_windows_test.go checks hardenFile's security descriptor, which only Windows has.

package boardserve

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHardenFileGrantsOnlyTheOwner asserts the hardened file's DACL holds one entry, for the current user.
func TestHardenFileGrantsOnlyTheOwner(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("token"), 0o644); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	if err := hardenFile(path); err != nil {
		t.Fatalf("hardenFile: %v", err)
	}

	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("descriptor.DACL(): %v", err)
	}
	if dacl.AceCount != 1 {
		t.Fatalf("DACL has %d entries; want 1", dacl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce(dacl, 0): %v", err)
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	// The entry's SID is stored inline right after its fixed fields.
	entrySID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !tokenUser.User.Sid.Equals(entrySID) {
		t.Errorf("the DACL's sole entry grants a SID other than the current user's")
	}
}
