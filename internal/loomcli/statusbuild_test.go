package loomcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdentityOf_ChangesWithSizeAndMtime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyx")
	if err := os.WriteFile(path, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	base, err := identityOf(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := identityOf(path)
	if err != nil {
		t.Fatal(err)
	}
	if base != again {
		t.Fatalf("identity of an untouched file changed: %+v vs %+v", base, again)
	}

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	touched, err := identityOf(path)
	if err != nil {
		t.Fatal(err)
	}
	if touched == base {
		t.Errorf("identity did not change with mtime: %+v", touched)
	}

	if err := os.WriteFile(path, []byte("longer content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	resized, err := identityOf(path)
	if err != nil {
		t.Fatal(err)
	}
	if resized.Size == touched.Size || resized == touched {
		t.Errorf("identity did not change with size: %+v vs %+v", resized, touched)
	}
}

func TestIdentityOf_ResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	viaLink, err := identityOf(link)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := identityOf(target)
	if err != nil {
		t.Fatal(err)
	}
	if viaLink != direct {
		t.Errorf("symlink identity %+v != direct %+v", viaLink, direct)
	}
}

func TestStatusSidecar_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", statusSidecarFile)
	build := buildIdentity{Path: "/bin/lyx", Size: 5, ModTime: 9}
	if got := readStatusSidecar(path); got != nil {
		t.Fatalf("missing sidecar read = %+v; want nil", got)
	}
	if err := writeStatusSidecar(path, "g0", build); err != nil {
		t.Fatal(err)
	}
	got := readStatusSidecar(path)
	if got == nil || got.GUID != "g0" || got.Build != build {
		t.Errorf("round trip = %+v; want guid g0 build %+v", got, build)
	}
}

func TestReadStatusSidecar_UnreadableIsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), statusSidecarFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readStatusSidecar(path); got != nil {
		t.Errorf("corrupt sidecar read = %+v; want nil", got)
	}
}
