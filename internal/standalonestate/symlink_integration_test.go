//go:build integration

// symlink_integration_test.go pins derive's symlink normalization against a real filesystem: a
// symlink path and its real target must hash identically, the property that stops two spellings
// of one directory getting two different state directories.
// It needs a real filesystem to create a symlink, so it is tagged integration to keep tier 1
// clean, even though a symlink is a filesystem operation rather than a git spawn.
// This package spawns no git and builds no hubforge/gitkit fixture, so it needs no TestMain.

package standalonestate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDerive_SymlinkNormalization pins that a symlink and its real target produce the same
// hash8, skipping when os.Symlink fails with a permission error so a Windows host without
// Developer Mode does not fail the suite.
func TestDerive_SymlinkNormalization(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q) error = %v", realDir, err)
	}

	linkPath := filepath.Join(base, "link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		if os.IsPermission(err) {
			t.Skip("os.Symlink not permitted on this host; skipping")
		}
		t.Fatalf("os.Symlink(%q, %q) error = %v", realDir, linkPath, err)
	}

	_, realHash8, err := derive("linux", "", "", "/home/user", realDir)
	if err != nil {
		t.Fatalf("derive(realDir) error = %v; want nil", err)
	}
	_, linkHash8, err := derive("linux", "", "", "/home/user", linkPath)
	if err != nil {
		t.Fatalf("derive(linkPath) error = %v; want nil", err)
	}

	if realHash8 != linkHash8 {
		t.Errorf("derive() hash8 real = %q, symlink = %q; want equal", realHash8, linkHash8)
	}
}
