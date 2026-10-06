// resolve_test.go contains unit tests for resolveLyx (dev/prod binary resolution) and prependPath
// (child-process PATH construction).
// All tests are Tier-1 pure: the devBinPath and lookPath seams are stubbed,
// and only os.Stat/os.WriteFile on t.TempDir() paths touch the filesystem.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestResolveLyx_DevBinaryExists verifies that resolveLyx returns the dev-binary path and sourceDev
// when devBinPath resolves to an existing file.
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

// TestResolveLyx_DevBinaryMissingFallsBackToProd verifies that resolveLyx falls back to lookPath
// and returns sourceProd when the dev binary is absent.
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

// TestResolveLyx_DevBinaryMissingAndLookPathFails verifies that resolveLyx propagates a lookPath
// error when both the dev binary is absent and PATH lookup fails.
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

// TestPrependPath verifies that prependPath makes dir the first PATH segment, leaves non-PATH env vars untouched, returns environ unchanged when dir is empty, and edits a Windows-form "Path=..."
// entry in place without appending a duplicate "PATH=" entry.
func TestPrependPath(t *testing.T) {
	t.Parallel()

	sep := string(os.PathListSeparator)
	tests := []struct {
		name    string
		dir     string
		environ []string
		want    []string
	}{
		{
			name:    "prepends to the existing PATH and keeps other entries",
			dir:     "/dev/bin",
			environ: []string{"PATH=/usr/bin:/bin", "HOME=/x"},
			want:    []string{"PATH=/dev/bin" + sep + "/usr/bin:/bin", "HOME=/x"},
		},
		{
			name:    "empty dir returns environ unchanged",
			dir:     "",
			environ: []string{"PATH=/usr/bin", "HOME=/x"},
			want:    []string{"PATH=/usr/bin", "HOME=/x"},
		},
		{
			name:    "Windows Path key edited in place",
			dir:     "C:\\dev-bin",
			environ: []string{"Path=C:\\Windows;C:\\Windows\\System32", "HOME=/x"},
			want:    []string{"Path=C:\\dev-bin" + sep + "C:\\Windows;C:\\Windows\\System32", "HOME=/x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := prependPath(tt.dir, tt.environ)
			if !slices.Equal(got, tt.want) {
				t.Errorf("prependPath(%q, %q) = %q; want %q", tt.dir, tt.environ, got, tt.want)
			}
		})
	}
}
