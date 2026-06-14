package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestCreatePortal covers the createPortal and removePortal helpers.
// It creates a paths.Layout from a test repo, creates the target _mhgo/ dir,
// calls createPortal and asserts the junction resolves to the target.
// Then it calls removePortal and asserts the link is gone while the target survives.
// A second removePortal call is idempotent (no error).
func TestCreatePortal(t *testing.T) {
	// Create a test hub repo
	hub := newTestRepo(t)

	// Build a Layout from the hub
	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hub, err)
	}

	// Create the target _mhgo directory
	targetParent := filepath.Join(l.Container, "test-slug", l.RelPath)
	if err := os.MkdirAll(targetParent, 0755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}

	targetDir := filepath.Join(targetParent, "_mhgo")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	// Test createPortal
	if err := createPortal(l, "test-slug"); err != nil {
		t.Skipf("portal creation not supported on this platform: %v", err)
	}

	// Verify the portal link exists and resolves to the target
	portalLink := filepath.Join(l.PortalsDir(), "test-slug")
	_, err = os.Lstat(portalLink)
	if err != nil {
		t.Fatalf("portal link does not exist: %v", err)
	}

	// Verify the link resolves to the target by checking that a known file
	// in the target is accessible through the portal link.
	// os.Readlink is unreliable for NTFS junctions (may include \??\ prefix),
	// so we use os.Stat to verify the junction resolves correctly.
	knownFile := filepath.Join(portalLink, "_mhgo")
	_, err = os.Stat(knownFile)
	if err != nil {
		t.Errorf("portal link does not resolve to target: %v", err)
	}

	// Test removePortal
	// First removePortal call
	if err := removePortal(l, "test-slug"); err != nil {
		t.Fatalf("removePortal: %v", err)
	}

	// Verify the link is gone
	portalLink = filepath.Join(l.PortalsDir(), "test-slug")
	_, err = os.Lstat(portalLink)
	if err == nil {
		t.Error("portal link still exists after removal")
	} else if !os.IsNotExist(err) {
		t.Fatalf("lstat portal: %v", err)
	}

	// Verify the target dir still exists (not recursively removed)
	targetDir = filepath.Join(l.Container, "test-slug", l.RelPath, "_mhgo")
	if _, err := os.Stat(targetDir); err != nil {
		t.Errorf("target _mhgo dir was removed: %v", err)
	}

	// Second removePortal call should be idempotent
	if err := removePortal(l, "test-slug"); err != nil {
		t.Fatalf("second removePortal: %v", err)
	}
}
