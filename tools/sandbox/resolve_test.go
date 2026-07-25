// resolve_test.go contains unit tests for resolveLyx (dev/prod binary
// resolution) and prependPath (child-process PATH construction). All tests
// are Tier-1 pure: the devBinPath and lookPath seams are stubbed, and only
// os.Stat/os.WriteFile on t.TempDir() paths touch the filesystem.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveLyx_DevBinaryExists verifies that resolveLyx returns the derived
// dev-binary path and sourceDev when devBinPath resolves to a file that
// actually exists on disk.
func TestResolveLyx_DevBinaryExists(t *testing.T) {
	devPath := filepath.Join(t.TempDir(), "lyx")
	if err := os.WriteFile(devPath, []byte("fake dev lyx"), 0o755); err != nil {
		t.Fatalf("write fake dev binary: %v", err)
	}

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return devPath, nil }

	path, source, err := resolveLyx()
	if err != nil {
		t.Fatalf("resolveLyx() error = %v; want nil", err)
	}
	if source != sourceDev {
		t.Errorf("resolveLyx() source = %q; want %q", source, sourceDev)
	}
	if path != devPath {
		t.Errorf("resolveLyx() path = %q; want %q", path, devPath)
	}
}

// TestResolveLyx_DevBinaryMissingFallsBackToProd verifies that resolveLyx
// falls back to the lookPath seam and returns sourceProd when devBinPath
// resolves to a path that does not exist on disk.
func TestResolveLyx_DevBinaryMissingFallsBackToProd(t *testing.T) {
	missingDevPath := filepath.Join(t.TempDir(), "lyx")
	const fakeProdPath = "/fake/prod/lyx"

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return missingDevPath, nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) { return fakeProdPath, nil }

	path, source, err := resolveLyx()
	if err != nil {
		t.Fatalf("resolveLyx() error = %v; want nil", err)
	}
	if source != sourceProd {
		t.Errorf("resolveLyx() source = %q; want %q", source, sourceProd)
	}
	if path != fakeProdPath {
		t.Errorf("resolveLyx() path = %q; want %q", path, fakeProdPath)
	}
}

// TestResolveLyx_DevBinaryMissingAndLookPathFails verifies that resolveLyx
// propagates a lookPath error when both the dev binary is absent and the
// PATH fallback fails to find "lyx".
func TestResolveLyx_DevBinaryMissingAndLookPathFails(t *testing.T) {
	missingDevPath := filepath.Join(t.TempDir(), "lyx")
	wantErr := errors.New("exec: \"lyx\": executable file not found in $PATH")

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return missingDevPath, nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) { return "", wantErr }

	_, _, err := resolveLyx()
	if err == nil {
		t.Fatal("resolveLyx() error = nil; want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("resolveLyx() error = %v; want wrapping %v", err, wantErr)
	}
}

// TestPrependPath_PrependsToExistingPath verifies that prependPath makes dir
// the first PATH segment while preserving the prior segments after it.
func TestPrependPath_PrependsToExistingPath(t *testing.T) {
	environ := []string{"PATH=/usr/bin:/bin", "HOME=/x"}
	got := prependPath("/dev/bin", environ)

	want := fmt.Sprintf("PATH=/dev/bin%c/usr/bin:/bin", os.PathListSeparator)
	if got[0] != want {
		t.Errorf("prependPath() PATH entry = %q; want %q", got[0], want)
	}
}

// TestPrependPath_PreservesNonPathEntries verifies that a non-PATH env var is
// left untouched by prependPath.
func TestPrependPath_PreservesNonPathEntries(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/x"}
	got := prependPath("/dev/bin", environ)

	if got[1] != "HOME=/x" {
		t.Errorf("prependPath() non-PATH entry = %q; want %q", got[1], "HOME=/x")
	}
}

// TestPrependPath_EmptyDirReturnsEnvironUnchanged verifies that prependPath
// with an empty dir returns environ with its contents unchanged.
func TestPrependPath_EmptyDirReturnsEnvironUnchanged(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/x"}
	got := prependPath("", environ)

	if len(got) != len(environ) {
		t.Fatalf("prependPath(\"\", environ) length = %d; want %d", len(got), len(environ))
	}
	for i := range environ {
		if got[i] != environ[i] {
			t.Errorf("prependPath(\"\", environ)[%d] = %q; want %q", i, got[i], environ[i])
		}
	}
}

// TestPrependPath_WindowsPathKeyEditedInPlace verifies that a lowercase-cased
// "Path=..." entry (the Windows form) is edited in place rather than left
// alone with a duplicate "PATH=" entry appended.
func TestPrependPath_WindowsPathKeyEditedInPlace(t *testing.T) {
	environ := []string{"Path=C:\\Windows;C:\\Windows\\System32", "HOME=/x"}
	got := prependPath("C:\\dev-bin", environ)

	if len(got) != len(environ) {
		t.Fatalf("prependPath() length = %d; want %d (no entry should be appended)", len(got), len(environ))
	}
	want := fmt.Sprintf("Path=C:\\dev-bin%cC:\\Windows;C:\\Windows\\System32", os.PathListSeparator)
	if got[0] != want {
		t.Errorf("prependPath() Path entry = %q; want %q", got[0], want)
	}
}
