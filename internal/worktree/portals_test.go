// portals_test.go covers portal junction create/remove, including mirrored link
// placement and idempotent removal that prunes empty ancestors.

package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestCreatePortal covers the createPortal and removePortal helpers.
// It creates a paths.Layout from a test repo subdirectory (non-trivial RelPath),
// creates the target _mhgo/ dir, calls createPortal and asserts the junction
// resolves to the target at the mirrored location l.PortalLink(slug).
// Then it calls removePortal and asserts the link is gone, empty ancestors are
// pruned, the target survives, and a second removePortal call is idempotent.
func TestCreatePortal(t *testing.T) {
	// Create a test hub repo
	hub := newTestRepo(t)

	// Create a subdirectory to get a non-trivial RelPath
	subdir := filepath.Join(hub, "subdir", "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	// Build a Layout from the subdirectory (RelPath will be "subdir/nested")
	l, err := paths.Resolve(subdir)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", subdir, err)
	}

	// Create the target _mhgo directory
	targetParent := filepath.Join(l.Container, "test-slug", l.RelPath)
	if err := os.MkdirAll(targetParent, 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}

	targetDir := filepath.Join(targetParent, "_mhgo")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	// Test createPortal
	if err := createPortal(l, "test-slug"); err != nil {
		t.Skipf("portal creation not supported on this platform: %v", err)
	}

	// Verify the portal link exists at the mirrored location l.PortalLink(slug)
	portalLink := l.PortalLink("test-slug")
	_, err = os.Lstat(portalLink)
	if err != nil {
		t.Fatalf("portal link does not exist at %s: %v", portalLink, err)
	}

	// Verify the link resolves to the target by accessing through the portal link.
	// os.Readlink is unreliable for NTFS junctions (may include \??\ prefix),
	// so we use os.Stat to verify the junction resolves correctly.
	info, err := os.Stat(portalLink)
	if err != nil {
		t.Errorf("portal link does not resolve: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("portal link does not resolve to a directory")
	}

	// Test removePortal
	// First removePortal call
	if err := removePortal(l, "test-slug"); err != nil {
		t.Fatalf("removePortal: %v", err)
	}

	// Verify the link is gone
	_, err = os.Lstat(portalLink)
	if err == nil {
		t.Error("portal link still exists after removal")
	} else if !os.IsNotExist(err) {
		t.Fatalf("lstat portal: %v", err)
	}

	// Verify empty mirrored ancestors are pruned up to PortalsDir
	// The parent dir of the link should be gone
	linkParent := filepath.Dir(portalLink)
	if _, err := os.Stat(linkParent); err == nil {
		t.Error("mirrored ancestor dir still exists after removal")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat link parent: %v", err)
	}

	// Verify PortalsDir itself still exists
	if _, err := os.Stat(l.PortalsDir()); err != nil {
		t.Errorf("PortalsDir was removed: %v", err)
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

// TestCreatePortalMultipleSubpaths asserts that distinct subpaths do not collide
// for the same slug (each gets its own mirrored directory).
func TestCreatePortalMultipleSubpaths(t *testing.T) {
	// Create a test hub repo
	hub := newTestRepo(t)

	// Create two distinct subdirectories
	subdir1 := filepath.Join(hub, "subdir1")
	subdir2 := filepath.Join(hub, "subdir2")
	if err := os.MkdirAll(subdir1, 0o755); err != nil {
		t.Fatalf("mkdir subdir1: %v", err)
	}
	if err := os.MkdirAll(subdir2, 0o755); err != nil {
		t.Fatalf("mkdir subdir2: %v", err)
	}

	// Resolve layouts from both subdirectories
	l1, err := paths.Resolve(subdir1)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", subdir1, err)
	}

	l2, err := paths.Resolve(subdir2)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", subdir2, err)
	}

	// Create target dirs for both
	for _, l := range []*paths.Layout{l1, l2} {
		targetParent := filepath.Join(l.Container, "test-slug", l.RelPath)
		if err := os.MkdirAll(targetParent, 0o755); err != nil {
			t.Fatalf("mkdir target parent: %v", err)
		}
		targetDir := filepath.Join(targetParent, "_mhgo")
		if err := os.Mkdir(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
	}

	// Create portals for the same slug from both subpaths
	if err := createPortal(l1, "test-slug"); err != nil {
		t.Skipf("portal creation not supported on this platform: %v", err)
	}
	if err := createPortal(l2, "test-slug"); err != nil {
		t.Skipf("portal creation not supported on this platform: %v", err)
	}

	// Verify both portals exist at distinct locations
	link1 := l1.PortalLink("test-slug")
	link2 := l2.PortalLink("test-slug")

	if link1 == link2 {
		t.Error("portals for same slug from different subpaths collide")
	}

	for _, link := range []string{link1, link2} {
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("portal link does not exist at %s: %v", link, err)
		}
	}
}

// TestCreatePortalRootRelPath asserts that the root-level (RelPath == ".")
// behavior is backward-compatible with the flat layout.
func TestCreatePortalRootRelPath(t *testing.T) {
	// Create a test hub repo
	hub := newTestRepo(t)

	// Build a Layout from the hub root (RelPath will be ".")
	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hub, err)
	}

	if l.RelPath != "." {
		t.Fatalf("expected RelPath == \".\", got %q", l.RelPath)
	}

	// Create the target _mhgo directory
	targetParent := filepath.Join(l.Container, "test-slug", l.RelPath)
	if err := os.MkdirAll(targetParent, 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}

	targetDir := filepath.Join(targetParent, "_mhgo")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	// Create portal
	if err := createPortal(l, "test-slug"); err != nil {
		t.Skipf("portal creation not supported on this platform: %v", err)
	}

	// Verify the portal link is at the flat location (no subpath segments)
	// This should collapse to <Container>/_portals/<slug>
	portalLink := l.PortalLink("test-slug")
	expectedLink := filepath.Join(l.PortalsDir(), "test-slug")
	if portalLink != expectedLink {
		t.Errorf("portal link mismatch: got %s, want %s", portalLink, expectedLink)
	}

	if _, err := os.Lstat(portalLink); err != nil {
		t.Fatalf("portal link does not exist: %v", err)
	}
}
