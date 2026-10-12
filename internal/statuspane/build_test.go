// build_test.go pins the build identity and the sidecar the bootstrap keeps or replaces a status strand by.

package statuspane

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testSidecarFile = "status-strand.json"

func TestIdentityOf(t *testing.T) {
	t.Parallel()

	t.Run("changes with size and mtime", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "lyx")
		if err := os.WriteFile(path, []byte("one"), 0o755); err != nil {
			t.Fatal(err)
		}
		base, err := IdentityOf(path)
		if err != nil {
			t.Fatal(err)
		}
		again, err := IdentityOf(path)
		if err != nil {
			t.Fatal(err)
		}
		if base != again {
			t.Fatalf("identity of an untouched file changed: %+v vs %+v", base, again)
		}

		later := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
		touched, err := IdentityOf(path)
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
		resized, err := IdentityOf(path)
		if err != nil {
			t.Fatal(err)
		}
		if resized.Size == touched.Size || resized == touched {
			t.Errorf("identity did not change with size: %+v vs %+v", resized, touched)
		}
	})

	t.Run("resolves a symlink", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		target := filepath.Join(dir, "real")
		if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		viaLink, err := IdentityOf(link)
		if err != nil {
			t.Fatal(err)
		}
		direct, err := IdentityOf(target)
		if err != nil {
			t.Fatal(err)
		}
		if viaLink != direct {
			t.Errorf("symlink identity %+v != direct %+v", viaLink, direct)
		}
	})
}

func TestSidecar_RoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", testSidecarFile)
	build := BuildIdentity{Path: "/bin/lyx", Size: 5, ModTime: 9}
	if got := ReadSidecar(path); got != nil {
		t.Fatalf("missing sidecar read = %+v; want nil", got)
	}
	if err := WriteSidecar(path, "g0", build); err != nil {
		t.Fatal(err)
	}
	got := ReadSidecar(path)
	if got == nil || got.GUID != "g0" || got.Build != build {
		t.Errorf("round trip = %+v; want guid g0 build %+v", got, build)
	}
	if err := WriteSidecar(path, "", build); err == nil {
		t.Error("WriteSidecar with an empty guid = nil error; want a refusal")
	}
}

func TestReadSidecar_UnreadableIsNil(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), testSidecarFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadSidecar(path); got != nil {
		t.Errorf("corrupt sidecar read = %+v; want nil", got)
	}
}
