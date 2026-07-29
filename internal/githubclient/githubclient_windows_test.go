//go:build windows

// githubclient_windows_test.go carries the one case in this package's test
// suite that is genuinely platform-specific: asserting the token cache
// file's Windows security descriptor directly, rather than inferring the
// hardening from os.Stat's mode bits (which, on Windows, reflect only the
// read-only attribute and would report the same value whether or not
// hardenCacheFile's DACL step ran at all). Every other case in this package
// -- the resolution chain, the cache schema and concurrent-writer coverage,
// and the authenticating RoundTripper's 401-replay behaviour -- is
// platform-agnostic and lives in the untagged githubclient_test.go, which
// this file shares its setCacheDir/seedCacheFile/credentialFileName helpers
// with. Splitting the Windows-only piece into its own build-tagged file
// mirrors this package's cache.go/cache_windows.go/cache_other.go split, and
// is what lets `go test -race -count=1 ./internal/githubclient/...` actually
// exercise the platform-agnostic suite on any machine, per the Test Tier
// Purity Invariant.

package githubclient

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestWriteCachedToken_CreatesFileWithRestrictivePermissions asserts the
// hardening this file requires directly against the Windows security
// descriptor, rather than inferring it from os.Stat's reported mode bits --
// which, on Windows, reflect only the read-only attribute and would report
// the same value regardless of whether hardenCacheFile's os.Chmod call or
// its DACL step ran at all.
func TestWriteCachedToken_CreatesFileWithRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)

	writeCachedToken("a-token")

	resolvedDir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false")
	}
	path := filepath.Join(resolvedDir, credentialFileName)

	// os.Stat's synthesized Mode() on Windows reflects only the read-only
	// attribute bit, not the 0600 os.Chmod call hardenCacheFile makes --
	// Windows has no Unix permission bits to report, which is exactly why
	// the DACL assertion below, not this stat, is the real check.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	assertOwnerOnlyDACL(t, path)
}

// assertOwnerOnlyDACL reads path's security descriptor via
// golang.org/x/sys/windows and asserts its DACL grants access to exactly one
// trustee: the file's own owner. This is the direct ACL assertion card 26
// requires, as distinct from an inference drawn from mode bits.
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
	// The ACE's SID is stored inline immediately after its fixed fields;
	// this is the same address-of-trailing-field pattern golang.org/x/sys's
	// own tests use to read it back out.
	aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !owner.Equals(aceSid) {
		t.Errorf("cache file DACL's sole ACE grants access to a SID other than the file's owner")
	}
}
