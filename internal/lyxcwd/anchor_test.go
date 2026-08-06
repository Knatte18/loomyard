//go:build integration

// anchor_test.go covers recorded-anchor AnchorRel resolution: Resolve's
// record-wins + strict cwd-equals-anchor gate, the marker-absent "."
// fallback, and ResolveWorktree's gate-free counterpart used by internal
// callers that resolve geometry from a worktree root rather than an acting cwd.

package lyxcwd_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// writeAnchor writes the recorded .lyx-anchor marker into hub's board
// directory, creating the board directory if needed. hub here is the
// lyxcwd.Location.HubPath value (the container directory), not a worktree root.
func writeAnchor(t *testing.T, hub, anchor string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(anchor), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// TestResolve_RootAnchor verifies that a root ("." ) recorded anchor resolves AnchorRel="."
// from exactly the worktree root,
// and that the strict gate now rejects a subdirectory of that root — the user-visible behaviour
// change documented in this batch's card 6 commit.
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

	t.Run("at root resolves", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(root)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v; want nil", root, err)
		}
		if layout.AnchorRel != "." {
			t.Errorf("Resolve(%q).AnchorRel = %q; want %q", root, layout.AnchorRel, ".")
		}
	})

	t.Run("at subdirectory errors under the strict gate", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(subDir)
		if layout != nil {
			t.Errorf("Resolve(%q) returned non-nil layout; want nil", subDir)
		}
		if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
			t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", subDir, err)
		}
	})
}

// TestResolve_SubpathAnchor verifies that a recorded subpath anchor ("backend") resolves
// AnchorRel="backend" exactly at the anchored directory,
// and that the strict gate now rejects a descendant of it.
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

	t.Run("at anchored directory resolves", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(backendDir)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v; want nil", backendDir, err)
		}
		if layout.AnchorRel != "backend" {
			t.Errorf("Resolve(%q).AnchorRel = %q; want %q", backendDir, layout.AnchorRel, "backend")
		}
	})

	t.Run("at descendant of anchored directory errors under the strict gate", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(deeperDir)
		if layout != nil {
			t.Errorf("Resolve(%q) returned non-nil layout; want nil", deeperDir)
		}
		if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
			t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", deeperDir, err)
		}
	})
}

// TestResolve_CwdOutsideAnchor verifies that a cwd outside the recorded anchor's subtree is a hard
// error wrapping ErrCwdOutsideAnchor: a sibling directory of the anchor,
// and the repo root itself (which sits above a subpath anchor).
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

// TestResolve_AnchorAbsentFallsBackToDot verifies that when no .lyx-anchor marker is recorded,
// Resolve's AnchorRel falls back to "."
// with no error at the worktree root — never to a cwd-derived relative path, which would make the
// Location name a lie — the mid-clone / lyxtest synthetic hub / non-fabric repo case.
// The strict gate is hoisted to apply unconditionally (card 6), so with no anchor recorded lyx is
// accepted only at the worktree root, never in a subdirectory: a subdirectory now errors.
func TestResolve_AnchorAbsentFallsBackToDot(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	root := fix.Hub

	subDir := filepath.Join(root, "sub", "nested")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	t.Run("at root resolves with AnchorRel dot", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(root)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v; want nil", root, err)
		}
		if layout.AnchorRel != "." {
			t.Errorf("Resolve(%q).AnchorRel = %q; want %q (no-anchor fallback)", root, layout.AnchorRel, ".")
		}
	})

	t.Run("at subdirectory errors under the strict gate", func(t *testing.T) {
		layout, err := lyxcwd.Resolve(subDir)
		if layout != nil {
			t.Errorf("Resolve(%q) returned non-nil layout; want nil", subDir)
		}
		if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
			t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", subDir, err)
		}
	})
}

// TestResolveWorktree_SubpathAnchorNoGate verifies the exact geometry fabricengine's hostLayoutFor
// fallback hits: calling the gate-free resolver with a worktree root that sits ABOVE a recorded
// subpath anchor must return RelPath="backend" and must NOT return ErrCwdOutsideAnchor — this
// gate-free behavior is what distinguishes ResolveWorktree from Resolve.
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
