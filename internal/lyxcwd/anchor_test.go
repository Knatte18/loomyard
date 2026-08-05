//go:build integration

// anchor_test.go covers recorded-anchor AnchorRel resolution: Resolve's
// record-wins + cwd at-or-below gate, the marker-absent "." fallback, and
// ResolveWorktree's gate-free counterpart used by internal callers that
// resolve geometry from a worktree root rather than an acting cwd.

package lyxcwd_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// writeAnchor writes the recorded .fabric-anchor marker into hub's board
// directory, creating the board directory if needed. hub here is the
// lyxcwd.Location.HubPath value (the container directory), not a worktree root.
func writeAnchor(t *testing.T, hub, anchor string) {
	t.Helper()

	boardDir := lyxcwd.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lyxcwd.FabricAnchorName)
	if err := os.WriteFile(anchorPath, []byte(anchor), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// TestResolve_RootAnchor verifies that a root ("." ) recorded anchor resolves
// RelPath="." from both the worktree root and a subdirectory, and never
// triggers the cwd gate (every cwd under the root is at or below ".").
func TestResolve_RootAnchor(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	base, err := lyxcwd.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve(root) error = %v; want nil", err)
	}
	writeAnchor(t, base.HubPath, ".")

	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"at root", root},
		{"at subdirectory", subDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := lyxcwd.Resolve(tt.cwd)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v; want nil", tt.cwd, err)
			}
			if layout.AnchorRel != "." {
				t.Errorf("Resolve(%q).AnchorRel = %q; want %q", tt.cwd, layout.AnchorRel, ".")
			}
		})
	}
}

// TestResolve_SubpathAnchor verifies that a recorded subpath anchor
// ("backend") resolves RelPath="backend" both at the anchored directory itself
// and at a descendant of it.
func TestResolve_SubpathAnchor(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	base, err := lyxcwd.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve(root) error = %v; want nil", err)
	}
	writeAnchor(t, base.HubPath, "backend")

	backendDir := filepath.Join(root, "backend")
	deeperDir := filepath.Join(backendDir, "deeper")
	if err := os.MkdirAll(deeperDir, 0o755); err != nil {
		t.Fatalf("mkdir backend/deeper: %v", err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"at anchored directory", backendDir},
		{"at descendant of anchored directory", deeperDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := lyxcwd.Resolve(tt.cwd)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v; want nil", tt.cwd, err)
			}
			if layout.AnchorRel != "backend" {
				t.Errorf("Resolve(%q).AnchorRel = %q; want %q", tt.cwd, layout.AnchorRel, "backend")
			}
		})
	}
}

// TestResolve_CwdOutsideAnchor verifies that a cwd outside the recorded
// anchor's subtree is a hard error wrapping ErrCwdOutsideAnchor: a sibling
// directory of the anchor, and the repo root itself (which sits above a
// subpath anchor).
func TestResolve_CwdOutsideAnchor(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	base, err := lyxcwd.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve(root) error = %v; want nil", err)
	}
	writeAnchor(t, base.HubPath, "backend")

	backendDir := filepath.Join(root, "backend")
	frontendDir := filepath.Join(root, "frontend")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.MkdirAll(frontendDir, 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"sibling directory of the anchor", frontendDir},
		{"repo root above a subpath anchor", root},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := lyxcwd.Resolve(tt.cwd)
			if layout != nil {
				t.Errorf("Resolve(%q) returned non-nil layout; want nil", tt.cwd)
			}
			if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
				t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", tt.cwd, err)
			}
		})
	}
}

// TestResolve_AnchorAbsentFallsBackToDot verifies that when no
// .fabric-anchor marker is recorded, Resolve's AnchorRel falls back to "."
// with no error — never to a cwd-derived relative path, which would make the
// Location name a lie — the mid-clone / lyxtest synthetic hub / non-fabric
// repo case.
func TestResolve_AnchorAbsentFallsBackToDot(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	subDir := filepath.Join(root, "sub", "nested")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	layout, err := lyxcwd.Resolve(subDir)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v; want nil", subDir, err)
	}
	if layout.AnchorRel != "." {
		t.Errorf("Resolve(%q).AnchorRel = %q; want %q (no-anchor fallback)", subDir, layout.AnchorRel, ".")
	}
}

// TestResolveWorktree_SubpathAnchorNoGate verifies the exact geometry
// fabricengine's hostLayoutFor fallback hits: calling the gate-free resolver
// with a worktree root that sits ABOVE a recorded subpath anchor must return
// RelPath="backend" and must NOT return ErrCwdOutsideAnchor — this gate-free
// behavior is what distinguishes ResolveWorktree from Resolve.
func TestResolveWorktree_SubpathAnchorNoGate(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	base, err := lyxcwd.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve(root) error = %v; want nil", err)
	}
	writeAnchor(t, base.HubPath, "backend")

	layout, err := lyxcwd.ResolveWorktree(root)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) error = %v; want nil", root, err)
	}
	if errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
		t.Errorf("ResolveWorktree(%q) error wraps ErrCwdOutsideAnchor; want no gate applied", root)
	}
	if layout.AnchorRel != "backend" {
		t.Errorf("ResolveWorktree(%q).AnchorRel = %q; want %q", root, layout.AnchorRel, "backend")
	}
}
