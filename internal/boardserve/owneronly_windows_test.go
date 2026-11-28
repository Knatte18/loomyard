// owneronly_windows_test.go pins the access list of a hardened file.

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

	path := filepath.Join(t.TempDir(), "secret")
	must(t, os.WriteFile(path, []byte("secret"), 0o600))
	must(t, hardenFile(path))

	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	must(t, err)
	dacl, _, err := descriptor.DACL()
	must(t, err)
	if dacl.AceCount != 1 {
		t.Fatalf("the access list holds %d entries; want 1", dacl.AceCount)
	}
	var entry *windows.ACCESS_ALLOWED_ACE
	must(t, windows.GetAce(dacl, 0, &entry))
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	must(t, err)
	if sid := (*windows.SID)(unsafe.Pointer(&entry.SidStart)); !sid.Equals(user.User.Sid) {
		t.Errorf("the access list grants %s; want the current user %s", sid, user.User.Sid)
	}
}
