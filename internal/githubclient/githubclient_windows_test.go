//go:build windows

// githubclient_windows_test.go holds the one platform-specific test of this package.
// It checks the cache file's Windows security descriptor directly.
// Mode bits cannot show the hardening on Windows, since os.Stat reports only the read-only attribute there.
// The package's other tests are platform-agnostic and live in githubclient_test.go.
// This file shares that file's setCacheDir, seedCacheFile and credentialFileName helpers.
// Keeping this file under a build tag lets the platform-agnostic suite run on any machine.
// For example, `go test -race -count=1 ./internal/githubclient/...` runs that suite everywhere.

package githubclient

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWriteCachedToken_CreatesFileWithRestrictivePermissions checks the cache file's DACL.
// Mode bits cannot show the hardening on Windows, so the check reads the security descriptor directly.
func TestWriteCachedToken_CreatesFileWithRestrictivePermissions(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	setCacheDir(t, dir)

	writeCachedToken("a-token")

	resolvedDir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false")
	}
	path := filepath.Join(resolvedDir, credentialFileName)

	// On Windows, os.Stat's Mode reflects only the read-only attribute bit.
	// It does not reflect the 0600 os.Chmod call that hardenCacheFile makes.
	// The DACL assertion below is therefore the real check; this stat only confirms the file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	assertOwnerOnlyDACL(t, path)
}

// assertOwnerOnlyDACL fails the test unless path's DACL grants access to exactly one trustee, the file's owner.
// It reads the security descriptor through golang.org/x/sys/windows, since mode bits cannot show the ACL.
func assertOwnerOnlyDACL(t *testing.T, path string) {
	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}

	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("descriptor.DACL(): %v", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatalf("descriptor.Owner(): %v", err)
	}

	if dacl.AceCount != 1 {
		t.Fatalf("cache file DACL has %d ACEs; want exactly 1 (owner-only, breaking inheritance)", dacl.AceCount)
	}

	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce(dacl, 0): %v", err)
	}
	// The ACE's SID is stored inline immediately after its fixed fields.
	// Reading the address of SidStart therefore yields the SID.
	aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !owner.Equals(aceSid) {
		t.Errorf("cache file DACL's sole ACE grants access to a SID other than the file's owner")
	}
}
