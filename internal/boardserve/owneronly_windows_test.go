//go:build windows

// owneronly_windows_test.go reads the security descriptor of a hardened file, which mode bits do not show on Windows.

package boardserve

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHardenFileGrantsOnlyTheOwner(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := hardenFile(path); err != nil {
		t.Fatalf("hardenFile: %v", err)
	}

	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL: %v", err)
	}
	if dacl.AceCount != 1 {
		t.Fatalf("DACL holds %d entries; want 1", dacl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	if sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)); !sid.Equals(user.User.Sid) {
		t.Errorf("DACL entry is for %s; want the current user %s", sid, user.User.Sid)
	}
}
